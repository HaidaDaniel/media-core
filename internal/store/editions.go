package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

// Edition and publication failures. Application services map these to the
// stable wire error codes; the store stays transport-agnostic.
var (
	ErrEditionNotFound      = errors.New("store: edition not found")
	ErrEditionTitleNotFound = errors.New("store: edition title not found")
	ErrInvalidEdition       = errors.New("store: invalid edition")
	ErrInvalidPublication   = errors.New("store: invalid publication")
	ErrSupersedeConflict    = errors.New("store: observation supersede conflict")
)

// EditionInput is one requested edition line of a work.
type EditionInput struct {
	Kind       domain.EditionKind
	Language   *string
	OtherLabel *string
	Titles     []TitleInput
}

// Edition is a persisted edition line with its active titles and current
// publication observation (nil when none was recorded; presentation of an
// explicit unknown is an application concern).
type Edition struct {
	ID          string
	WorkID      string
	Kind        domain.EditionKind
	Language    *string
	OtherLabel  *string
	Revision    int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArchivedAt  *time.Time
	Titles      []EditionTitle
	Publication *PublicationObservation
}

// EditionTitle is one edition title row, active or retired.
type EditionTitle struct {
	ID         string
	EditionID  string
	Text       string
	Language   *string
	Kind       domain.TitleKind
	Preferred  bool
	Provenance string
	CreatedAt  time.Time
	RetiredAt  *time.Time
	SearchKey  string
}

// PublicationInput is one requested publication observation.
type PublicationInput struct {
	Value        domain.Publication
	Source       *string
	ObservedAt   time.Time
	Provenance   string
	SupersedesID *string
}

// PublicationObservation is one stored publication history row.
type PublicationObservation struct {
	ID           string
	EditionID    string
	Value        domain.Publication
	Source       *string
	ObservedAt   time.Time
	RecordedAt   time.Time
	Provenance   string
	SupersedesID *string
}

// AddEdition appends one edition line to a work in a write transaction.
// The work revision must match (a work's child mutation) and is advanced;
// the edition starts at revision 0. Edition titles follow the work-title
// rules, and an edition without titles falls back to its work title for
// display.
func (d *Database) AddEdition(ctx context.Context, now time.Time, workID string, expectedWorkRevision int64, in EditionInput) (Edition, error) {
	if err := domain.ValidateEditionAttrs(in.Kind, in.Language, in.OtherLabel); err != nil {
		return Edition{}, fmt.Errorf("%w: %v", ErrInvalidEdition, err)
	}
	preferredCount := 0
	for _, t := range in.Titles {
		if err := validateTitleInput(t); err != nil {
			return Edition{}, fmt.Errorf("%w: %v", ErrInvalidEdition, err)
		}
		if t.Preferred {
			preferredCount++
		}
	}
	if preferredCount > 1 {
		return Edition{}, fmt.Errorf("%w: at most one preferred title", ErrInvalidEdition)
	}
	editionID, err := domain.NewUUID()
	if err != nil {
		return Edition{}, err
	}
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Edition{}, err
	}
	defer tx.Rollback()
	workRevision, err := requireRevision(tx, ctx, workID, expectedWorkRevision)
	if err != nil {
		return Edition{}, err
	}
	createdAt := instantUTC(now)
	if _, err := tx.ExecContext(ctx, `INSERT INTO editions (id, work_id, kind, language, other_label, revision, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
		editionID, workID, string(in.Kind), in.Language, in.OtherLabel, createdAt, createdAt); err != nil {
		return Edition{}, fmt.Errorf("store: insert edition: %w", err)
	}
	for i, t := range in.Titles {
		id, err := domain.NewUUID()
		if err != nil {
			return Edition{}, err
		}
		preferred := t.Preferred
		if preferredCount == 0 && i == 0 {
			preferred = true
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO edition_titles (id, edition_id, seq, text, search_key, language, kind, preferred, provenance, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, editionID, i+1, t.Text, domain.SearchKey(t.Text), t.Language, string(t.Kind), boolInt(preferred), t.Provenance, createdAt); err != nil {
			return Edition{}, fmt.Errorf("store: insert edition title: %w", err)
		}
	}
	if err := bumpWorkRevision(tx, ctx, workID, workRevision, now); err != nil {
		return Edition{}, err
	}
	if err := tx.Commit(); err != nil {
		return Edition{}, fmt.Errorf("store: commit edition: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}

// GetEdition returns one edition with its active titles and current
// publication observation.
func (d *Database) GetEdition(ctx context.Context, editionID string) (Edition, error) {
	var (
		e          Edition
		createdAt  string
		updatedAt  string
		archivedAt sql.NullString
		language   sql.NullString
		otherLabel sql.NullString
		kind       string
	)
	err := d.db.QueryRowContext(ctx,
		`SELECT id, work_id, kind, language, other_label, revision, created_at, updated_at, archived_at
		 FROM editions WHERE id = ?`, editionID).
		Scan(&e.ID, &e.WorkID, &kind, &language, &otherLabel, &e.Revision, &createdAt, &updatedAt, &archivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Edition{}, fmt.Errorf("%w: %s", ErrEditionNotFound, editionID)
	}
	if err != nil {
		return Edition{}, fmt.Errorf("store: query edition: %w", err)
	}
	e.Kind = domain.EditionKind(kind)
	if language.Valid {
		e.Language = &language.String
	}
	if otherLabel.Valid {
		e.OtherLabel = &otherLabel.String
	}
	e.CreatedAt, err = parseInstant(createdAt)
	if err != nil {
		return Edition{}, fmt.Errorf("store: parse edition created_at: %w", err)
	}
	e.UpdatedAt, err = parseInstant(updatedAt)
	if err != nil {
		return Edition{}, fmt.Errorf("store: parse edition updated_at: %w", err)
	}
	if archivedAt.Valid {
		t, err := parseInstant(archivedAt.String)
		if err != nil {
			return Edition{}, fmt.Errorf("store: parse edition archived_at: %w", err)
		}
		e.ArchivedAt = &t
	}
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, text, search_key, language, kind, preferred, provenance, created_at
		 FROM edition_titles WHERE edition_id = ? AND retired_at IS NULL ORDER BY seq`, editionID)
	if err != nil {
		return Edition{}, fmt.Errorf("store: query edition titles: %w", err)
	}
	defer rows.Close()
	e.Titles = []EditionTitle{}
	for rows.Next() {
		var (
			t          EditionTitle
			tLang      sql.NullString
			tKind      string
			preferred  int
			tCreatedAt string
		)
		if err := rows.Scan(&t.ID, &t.Text, &t.SearchKey, &tLang, &tKind, &preferred, &t.Provenance, &tCreatedAt); err != nil {
			return Edition{}, fmt.Errorf("store: scan edition title: %w", err)
		}
		t.EditionID = editionID
		t.Kind = domain.TitleKind(tKind)
		t.Preferred = preferred == 1
		if tLang.Valid {
			t.Language = &tLang.String
		}
		t.CreatedAt, err = parseInstant(tCreatedAt)
		if err != nil {
			return Edition{}, fmt.Errorf("store: parse edition title created_at: %w", err)
		}
		e.Titles = append(e.Titles, t)
	}
	if err := rows.Err(); err != nil {
		return Edition{}, err
	}
	e.Publication, err = d.currentPublication(ctx, editionID)
	return e, err
}

// ListEditions returns the editions of one work in creation order for
// work-view summaries.
func (d *Database) ListEditions(ctx context.Context, workID string) ([]Edition, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id FROM editions WHERE work_id = ? ORDER BY created_at, id`, workID)
	if err != nil {
		return nil, fmt.Errorf("store: list editions: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan edition list: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	editions := make([]Edition, 0, len(ids))
	for _, id := range ids {
		e, err := d.GetEdition(ctx, id)
		if err != nil {
			return nil, err
		}
		editions = append(editions, e)
	}
	return editions, nil
}

// currentPublication selects the last non-retracted observation by
// (observed_at, recorded_at, id). A row is retracted only when another row
// supersedes it; the superseding correction row itself is live. A
// past-dated observation never overtakes a newer one.
func (d *Database) currentPublication(ctx context.Context, editionID string) (*PublicationObservation, error) {
	var (
		p          PublicationObservation
		observedAt string
		recordedAt string
		source     sql.NullString
		supersedes sql.NullString
		value      string
	)
	err := d.db.QueryRowContext(ctx, `
		SELECT o.id, o.value, o.observed_at, o.recorded_at, o.source, o.provenance, o.supersedes_id
		FROM publication_observations o
		WHERE o.edition_id = ?
		  AND NOT EXISTS (SELECT 1 FROM publication_observations s WHERE s.supersedes_id = o.id)
		ORDER BY o.observed_at DESC, o.recorded_at DESC, o.id DESC
		LIMIT 1`, editionID).
		Scan(&p.ID, &value, &observedAt, &recordedAt, &source, &p.Provenance, &supersedes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: query current publication: %w", err)
	}
	p.EditionID = editionID
	p.Value = domain.Publication(value)
	if source.Valid {
		p.Source = &source.String
	}
	if supersedes.Valid {
		p.SupersedesID = &supersedes.String
	}
	p.ObservedAt, err = parseInstant(observedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse publication observed_at: %w", err)
	}
	p.RecordedAt, err = parseInstant(recordedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse publication recorded_at: %w", err)
	}
	return &p, nil
}

// AddEditionTitle appends a title to an edition; a preferred new title
// demotes the current preferred one. The edition revision guards against
// lost updates.
func (d *Database) AddEditionTitle(ctx context.Context, now time.Time, editionID string, expectedRevision int64, in TitleInput) (Edition, error) {
	if err := validateTitleInput(in); err != nil {
		return Edition{}, fmt.Errorf("%w: %v", ErrInvalidEdition, err)
	}
	id, err := domain.NewUUID()
	if err != nil {
		return Edition{}, err
	}
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Edition{}, err
	}
	defer tx.Rollback()
	revision, err := requireEditionRevision(tx, ctx, editionID, expectedRevision)
	if err != nil {
		return Edition{}, err
	}
	var (
		seq    int
		active int
	)
	// seq advances over all rows (active and retired) so a title added after
	// a full retirement never reuses a retired seq under the unique index;
	// the active count drives the preferred-promotion rule.
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(seq), 0) + 1,
		       COALESCE(SUM(CASE WHEN retired_at IS NULL THEN 1 ELSE 0 END), 0)
		FROM edition_titles WHERE edition_id = ?`, editionID).Scan(&seq, &active); err != nil {
		return Edition{}, fmt.Errorf("store: next title seq: %w", err)
	}
	// The first title of an edition becomes preferred so the edition has a
	// display title of its own; an empty edition falls back to its work title.
	preferred := in.Preferred || active == 0
	if in.Preferred {
		if _, err := tx.ExecContext(ctx,
			`UPDATE edition_titles SET preferred = 0 WHERE edition_id = ? AND preferred = 1 AND retired_at IS NULL AND id != ?`,
			editionID, id); err != nil {
			return Edition{}, fmt.Errorf("store: demote preferred title: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO edition_titles (id, edition_id, seq, text, search_key, language, kind, preferred, provenance, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, editionID, seq, in.Text, domain.SearchKey(in.Text), in.Language, string(in.Kind), boolInt(preferred), in.Provenance, instantUTC(now)); err != nil {
		return Edition{}, fmt.Errorf("store: insert edition title: %w", err)
	}
	if err := d.bumpEditionRevision(tx, ctx, editionID, revision, now); err != nil {
		return Edition{}, err
	}
	if err := tx.Commit(); err != nil {
		return Edition{}, fmt.Errorf("store: commit edition title: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}

// RetireEditionTitle retires one active edition title, keeping the row as
// history. Unlike work titles, an edition may retire its last title: the
// edition falls back to its work title for display. Retiring the preferred
// title promotes the oldest remaining active title, if any.
func (d *Database) RetireEditionTitle(ctx context.Context, now time.Time, editionID string, expectedRevision int64, titleID string) (Edition, error) {
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Edition{}, err
	}
	defer tx.Rollback()
	revision, err := requireEditionRevision(tx, ctx, editionID, expectedRevision)
	if err != nil {
		return Edition{}, err
	}
	var preferred int
	err = tx.QueryRowContext(ctx,
		`SELECT preferred FROM edition_titles WHERE id = ? AND edition_id = ? AND retired_at IS NULL`,
		titleID, editionID).Scan(&preferred)
	if errors.Is(err, sql.ErrNoRows) {
		return Edition{}, fmt.Errorf("%w: %s", ErrEditionTitleNotFound, titleID)
	}
	if err != nil {
		return Edition{}, fmt.Errorf("store: read title for retirement: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE edition_titles SET retired_at = ? WHERE id = ? AND edition_id = ? AND retired_at IS NULL`,
		instantUTC(now), titleID, editionID); err != nil {
		return Edition{}, fmt.Errorf("store: retire edition title: %w", err)
	}
	if preferred == 1 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE edition_titles SET preferred = 1
			WHERE id = (
				SELECT id FROM edition_titles
				WHERE edition_id = ? AND retired_at IS NULL
				ORDER BY seq
				LIMIT 1
			)`, editionID); err != nil {
			return Edition{}, fmt.Errorf("store: promote preferred title: %w", err)
		}
	}
	if err := d.bumpEditionRevision(tx, ctx, editionID, revision, now); err != nil {
		return Edition{}, err
	}
	if err := tx.Commit(); err != nil {
		return Edition{}, fmt.Errorf("store: commit edition title retirement: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}

// SetPreferredEditionTitle makes one active title the preferred display
// title, demoting the current preferred title without retiring it.
func (d *Database) SetPreferredEditionTitle(ctx context.Context, now time.Time, editionID string, expectedRevision int64, titleID string) (Edition, error) {
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Edition{}, err
	}
	defer tx.Rollback()
	revision, err := requireEditionRevision(tx, ctx, editionID, expectedRevision)
	if err != nil {
		return Edition{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE edition_titles SET preferred = 0 WHERE edition_id = ? AND preferred = 1 AND retired_at IS NULL AND id != ?`,
		editionID, titleID); err != nil {
		return Edition{}, fmt.Errorf("store: demote preferred title: %w", err)
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE edition_titles SET preferred = 1 WHERE id = ? AND edition_id = ? AND retired_at IS NULL`,
		titleID, editionID)
	if err != nil {
		return Edition{}, fmt.Errorf("store: set preferred title: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Edition{}, fmt.Errorf("store: set preferred title: %w", err)
	}
	if n != 1 {
		return Edition{}, fmt.Errorf("%w: %s", ErrEditionTitleNotFound, titleID)
	}
	if err := d.bumpEditionRevision(tx, ctx, editionID, revision, now); err != nil {
		return Edition{}, err
	}
	if err := tx.Commit(); err != nil {
		return Edition{}, fmt.Errorf("store: commit edition preferred: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}

// RecordPublication appends one sourced publication observation. A
// supersedes_id correction targets an existing, not-yet-superseded
// observation of the same edition; the old row stays as history.
// Past-dated observations are stored but do not become current over a newer
// one. The edition revision guards against lost updates.
func (d *Database) RecordPublication(ctx context.Context, now time.Time, editionID string, expectedRevision int64, in PublicationInput) (Edition, error) {
	source := ""
	if in.Source != nil {
		source = *in.Source
	}
	if err := domain.ValidatePublicationAttrs(in.Value, source, in.Provenance, in.ObservedAt, now); err != nil {
		return Edition{}, fmt.Errorf("%w: %v", ErrInvalidPublication, err)
	}
	id, err := domain.NewUUID()
	if err != nil {
		return Edition{}, err
	}
	observedAt := in.ObservedAt
	if observedAt.IsZero() {
		observedAt = now
	}
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return Edition{}, err
	}
	defer tx.Rollback()
	revision, err := requireEditionRevision(tx, ctx, editionID, expectedRevision)
	if err != nil {
		return Edition{}, err
	}
	if in.SupersedesID != nil {
		var targetID string
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM publication_observations WHERE id = ? AND edition_id = ?`,
			*in.SupersedesID, editionID).Scan(&targetID)
		if errors.Is(err, sql.ErrNoRows) {
			return Edition{}, fmt.Errorf("%w: %s", ErrSupersedeConflict, *in.SupersedesID)
		}
		if err != nil {
			return Edition{}, fmt.Errorf("store: read superseded observation: %w", err)
		}
		var superseding int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM publication_observations WHERE supersedes_id = ?`, *in.SupersedesID).Scan(&superseding); err != nil {
			return Edition{}, fmt.Errorf("store: check supersede: %w", err)
		}
		if superseding != 0 {
			return Edition{}, fmt.Errorf("%w: %s is already superseded", ErrSupersedeConflict, *in.SupersedesID)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO publication_observations (id, edition_id, value, observed_at, recorded_at, source, provenance, supersedes_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, editionID, string(in.Value), instantUTC(observedAt), instantUTC(now), in.Source, in.Provenance, in.SupersedesID); err != nil {
		return Edition{}, fmt.Errorf("store: insert publication observation: %w", err)
	}
	if err := d.bumpEditionRevision(tx, ctx, editionID, revision, now); err != nil {
		return Edition{}, err
	}
	if err := tx.Commit(); err != nil {
		return Edition{}, fmt.Errorf("store: commit publication: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}

// requireEditionRevision is the edition-scoped lost-update guard; it reads
// the editions table and reports a missing edition distinctly.
func requireEditionRevision(tx *sql.Tx, ctx context.Context, editionID string, expectedRevision int64) (int64, error) {
	var revision int64
	err := tx.QueryRowContext(ctx, `SELECT revision FROM editions WHERE id = ?`, editionID).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: %s", ErrEditionNotFound, editionID)
	}
	if err != nil {
		return 0, fmt.Errorf("store: read edition revision: %w", err)
	}
	if revision != expectedRevision {
		return 0, fmt.Errorf("%w: expected %d, current %d", ErrRevisionMismatch, expectedRevision, revision)
	}
	return revision, nil
}

// bumpEditionRevision advances the edition revision and updated_at.
func (d *Database) bumpEditionRevision(tx *sql.Tx, ctx context.Context, editionID string, revision int64, now time.Time) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE editions SET revision = revision + 1, updated_at = ? WHERE id = ? AND revision = ?`,
		instantUTC(now), editionID, revision)
	if err != nil {
		return fmt.Errorf("store: bump edition revision: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: bump edition revision: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("%w: expected %d, but the edition was updated concurrently", ErrRevisionMismatch, revision)
	}
	return nil
}
