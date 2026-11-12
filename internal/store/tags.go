package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

// Tag failures. Application services map these to the stable wire error
// codes; the store stays transport-agnostic.
var (
	ErrInvalidTag  = errors.New("store: invalid tag")
	ErrTagNotFound = errors.New("store: tag assignment not found")
)

// MaxTagNameRunes bounds a normalized tag name.
const MaxTagNameRunes = 64

// NormalizeTagName computes the deterministic unique identity of a tag
// name: Unicode NFKC, full Unicode case fold, trim and collapse of Unicode
// whitespace, like the title search key. It reports a distinct invalid-tag
// error when the normalized name is empty or exceeds the bound.
func NormalizeTagName(raw string) (string, error) {
	name := domain.SearchKey(raw)
	if name == "" {
		return "", fmt.Errorf("%w: name is empty after normalization", ErrInvalidTag)
	}
	if utf8.RuneCountInString(name) > MaxTagNameRunes {
		return "", fmt.Errorf("%w: name exceeds %d characters after normalization", ErrInvalidTag, MaxTagNameRunes)
	}
	return name, nil
}

// TagActor is the acting identity recorded on a tag attach or detach. It
// is stored as canonical JSON in the actor columns.
type TagActor struct {
	Principal string `json:"principal"`
	Origin    string `json:"origin,omitempty"`
	Identity  string `json:"identity,omitempty"`
}

// TagAssignment is one stored attach/detach audit row. A row is active
// while DetachedAt is nil; a detach keeps the row as history.
type TagAssignment struct {
	ID          string
	TagID       string
	Name        string
	WorkID      string
	EditionID   string
	AttachedAt  time.Time
	Actor       TagActor
	DetachedAt  *time.Time
	DetachActor *TagActor
}

// activeTagNames lists the normalized names of every active assignment on
// one target, in name order.
func (d *Database) activeTagNames(ctx context.Context, q queryer, column, targetID string) ([]string, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT tg.name FROM tag_assignments ta
		 JOIN tags tg ON tg.id = ta.tag_id
		 WHERE ta.`+column+` = ? AND ta.detached_at IS NULL
		 ORDER BY tg.name`, targetID)
	if err != nil {
		return nil, fmt.Errorf("store: query active tags: %w", err)
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("store: scan active tag: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return names, nil
}

// currentWorkTags lists the normalized names of every active work tag.
func (d *Database) currentWorkTags(ctx context.Context, workID string) ([]string, error) {
	return d.activeTagNames(ctx, d.db, "work_id", workID)
}

// currentEditionTags lists the normalized names of every active edition tag.
func (d *Database) currentEditionTags(ctx context.Context, editionID string) ([]string, error) {
	return d.activeTagNames(ctx, d.db, "edition_id", editionID)
}

// ensureTagTx resolves the tag row by normalized name, inserting it on
// first use, inside the caller's transaction.
func (d *Database) ensureTagTx(ctx context.Context, tx *sql.Tx, name string, nowS string) (string, error) {
	var tagID string
	err := tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE name = ?`, name).Scan(&tagID)
	if errors.Is(err, sql.ErrNoRows) {
		tagID, err = domain.NewUUID()
		if err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tags (id, name, created_at) VALUES (?, ?, ?)`,
			tagID, name, nowS); err != nil {
			return "", fmt.Errorf("store: insert tag: %w", err)
		}
		return tagID, nil
	}
	if err != nil {
		return "", fmt.Errorf("store: resolve tag: %w", err)
	}
	return tagID, nil
}

// activeAssignmentTx finds the active assignment of one tag to one target
// inside the caller's transaction; a missing row is not an error.
func activeAssignmentTx(ctx context.Context, tx *sql.Tx, tagID, column, targetID string) (TagAssignment, error) {
	var (
		a          TagAssignment
		attachedAt string
		actor      string
		target     sql.NullString
	)
	err := tx.QueryRowContext(ctx,
		`SELECT ta.id, ta.tag_id, tg.name, ta.`+column+`, ta.attached_at, ta.actor
		 FROM tag_assignments ta
		 JOIN tags tg ON tg.id = ta.tag_id
		 WHERE ta.tag_id = ? AND ta.`+column+` = ? AND ta.detached_at IS NULL`,
		tagID, targetID).
		Scan(&a.ID, &a.TagID, &a.Name, &target, &attachedAt, &actor)
	if errors.Is(err, sql.ErrNoRows) {
		return TagAssignment{}, nil
	}
	if err != nil {
		return TagAssignment{}, fmt.Errorf("store: query active tag assignment: %w", err)
	}
	if column == "work_id" {
		a.WorkID = target.String
	} else {
		a.EditionID = target.String
	}
	var act TagActor
	if err := json.Unmarshal([]byte(actor), &act); err != nil {
		return TagAssignment{}, fmt.Errorf("store: parse tag actor: %w", err)
	}
	a.Actor = act
	a.AttachedAt, err = parseInstant(attachedAt)
	if err != nil {
		return TagAssignment{}, fmt.Errorf("store: parse tag attached_at: %w", err)
	}
	return a, nil
}

// setTagTx applies one idempotent attach or detach of a normalized tag to
// one target inside the caller's transaction and returns the effective
// target revision after the operation. A no-op leaves the revision as is.
func (d *Database) setTagTx(ctx context.Context, tx *sql.Tx, column, targetID string, expectedRevision int64, name string, detach bool, actor TagActor, now time.Time) (int64, error) {
	revision, err := d.requireTargetRevisionTx(tx, ctx, column, targetID, expectedRevision)
	if err != nil {
		return 0, err
	}
	nowS := instantUTC(now)
	tagID, err := d.ensureTagTx(ctx, tx, name, nowS)
	if err != nil {
		return 0, err
	}
	active, err := activeAssignmentTx(ctx, tx, tagID, column, targetID)
	if err != nil {
		return 0, err
	}
	// A repeated attach or detach is a no-op: the assignment state is
	// already what the request asks for.
	if (detach && active.ID == "") || (!detach && active.ID != "") {
		return revision, nil
	}
	if detach {
		detachJSON, err := json.Marshal(actor)
		if err != nil {
			return 0, fmt.Errorf("store: encode tag actor: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE tag_assignments SET detached_at = ?, detach_actor = ? WHERE id = ?`,
			nowS, string(detachJSON), active.ID); err != nil {
			return 0, fmt.Errorf("store: detach tag: %w", err)
		}
	} else {
		id, err := domain.NewUUID()
		if err != nil {
			return 0, err
		}
		actorJSON, err := json.Marshal(actor)
		if err != nil {
			return 0, fmt.Errorf("store: encode tag actor: %w", err)
		}
		// The non-target column stays SQL NULL so the exactly-one-target
		// check holds.
		var workID, editionID any
		if column == "work_id" {
			workID = targetID
		} else {
			editionID = targetID
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tag_assignments (id, tag_id, work_id, edition_id, attached_at, actor)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			id, tagID, workID, editionID, nowS, string(actorJSON)); err != nil {
			return 0, fmt.Errorf("store: attach tag: %w", err)
		}
	}
	if err := d.bumpTargetRevisionTx(tx, ctx, column, targetID, revision, now); err != nil {
		return 0, err
	}
	return revision + 1, nil
}

// requireTargetRevisionTx reads the expected revision of the target entity
// inside the write transaction.
func (d *Database) requireTargetRevisionTx(tx *sql.Tx, ctx context.Context, column, targetID string, expectedRevision int64) (int64, error) {
	if column == "work_id" {
		return requireRevision(tx, ctx, targetID, expectedRevision)
	}
	return requireEditionRevision(tx, ctx, targetID, expectedRevision)
}

// bumpTargetRevisionTx advances the revision and updated_at of the target
// entity.
func (d *Database) bumpTargetRevisionTx(tx *sql.Tx, ctx context.Context, column, targetID string, revision int64, now time.Time) error {
	if column == "work_id" {
		return bumpWorkRevision(tx, ctx, targetID, revision, now)
	}
	return d.bumpEditionRevision(tx, ctx, targetID, revision, now)
}

// SetWorkTag attaches or detaches one normalized tag on a work. A repeated
// attach or detach is a no-op without a revision bump but still settles the
// journal so exact retries replay. A detach keeps the assignment row as
// audit history. The work revision guards against lost updates.
func (d *Database) SetWorkTag(ctx context.Context, now time.Time, workID string, expectedRevision int64, name string, detach bool, actor TagActor, journal *JournalInput) (Work, error) {
	normalized, err := NormalizeTagName(name)
	if err != nil {
		return Work{}, err
	}
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Work{}, err
	}
	defer tx.Rollback()
	revision, err := d.setTagTx(ctx, tx, "work_id", workID, expectedRevision, normalized, detach, actor, now)
	if err != nil {
		return Work{}, err
	}
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("work_id", workID, revision)); err != nil {
		return Work{}, err
	}
	if err := tx.Commit(); err != nil {
		return Work{}, fmt.Errorf("store: commit work tag: %w", err)
	}
	return d.GetWork(ctx, workID)
}

// SetEditionTag attaches or detaches one normalized tag on an edition with
// the same no-op, audit and revision rules as SetWorkTag.
func (d *Database) SetEditionTag(ctx context.Context, now time.Time, editionID string, expectedRevision int64, name string, detach bool, actor TagActor, journal *JournalInput) (Edition, error) {
	normalized, err := NormalizeTagName(name)
	if err != nil {
		return Edition{}, err
	}
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Edition{}, err
	}
	defer tx.Rollback()
	revision, err := d.setTagTx(ctx, tx, "edition_id", editionID, expectedRevision, normalized, detach, actor, now)
	if err != nil {
		return Edition{}, err
	}
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("edition_id", editionID, revision)); err != nil {
		return Edition{}, err
	}
	if err := tx.Commit(); err != nil {
		return Edition{}, fmt.Errorf("store: commit edition tag: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}
