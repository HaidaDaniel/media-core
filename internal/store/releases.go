package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

// Release binding failures. The application layer maps these to the stable
// wire error codes; the store stays transport-agnostic. A binding conflict is
// an explicit map collision (a confirmed publication already points at a
// different edition); a missing binding is a revoke or read of a publication
// that has no active binding.
var (
	ErrBindingConflict = errors.New("store: release binding conflict")
	ErrBindingNotFound = errors.New("store: release binding not found")
)

// Binding states.
const (
	BindingStateActive  = "active"
	BindingStateRevoked = "revoked"
)

// ReleaseBinding is one explicit, audited publication→edition binding row.
// The immutable import event payload is never rewritten; a binding change
// adds or revokes rows, so the old binding history is always retained.
type ReleaseBinding struct {
	ID            string
	Namespace     string
	PublicationID string
	EditionID     string
	State         string
	CreatedBy     string
	CreatedAt     time.Time
	RevokedBy     *string
	RevokedAt     *time.Time
	ReplacesID    *string
}

// ReleaseProjection is one imported release event made visible as a release
// of an edition through an active binding. It is a projection over the
// immutable event, never a copy: the event row and the binding rows keep the
// history. The release fields are the producer's, labeled by the exact
// (namespace, publication_id) they came from — counts are never summed
// across publications.
type ReleaseProjection struct {
	EventID       string
	EventType     string
	Namespace     string
	PublicationID *string
	EditionID     string
	WorkID        string
	ReleaseID     *string
	ReleaseNumber *string
	ReleaseTitle  *string
	ReleaseURL    *string
	CurrentCount  *int
	CountDelta    *int
	SourceTime    *string
	DetectionTime *string
	CreatedAt     time.Time
}

const bindingColumns = `id, namespace, publication_id, edition_id, state,
	created_by, created_at, revoked_by, revoked_at, replaces_id`

// scanReleaseBinding fills one binding row from a scanner.
func scanReleaseBinding(s interface {
	Scan(dest ...any) error
}) (ReleaseBinding, error) {
	var (
		b         ReleaseBinding
		revokedBy sql.NullString
		revokedAt sql.NullString
		replaces  sql.NullString
		createdAt string
	)
	if err := s.Scan(&b.ID, &b.Namespace, &b.PublicationID, &b.EditionID, &b.State,
		&b.CreatedBy, &createdAt, &revokedBy, &revokedAt, &replaces); err != nil {
		return ReleaseBinding{}, err
	}
	t, err := parseInstant(createdAt)
	if err != nil {
		return ReleaseBinding{}, fmt.Errorf("store: parse binding created_at: %w", err)
	}
	b.CreatedAt = t
	if revokedBy.Valid {
		b.RevokedBy = &revokedBy.String
	}
	if revokedAt.Valid {
		r, err := parseInstant(revokedAt.String)
		if err != nil {
			return ReleaseBinding{}, fmt.Errorf("store: parse binding revoked_at: %w", err)
		}
		b.RevokedAt = &r
	}
	if replaces.Valid {
		b.ReplacesID = &replaces.String
	}
	return b, nil
}

// validateBindingInput checks the explicit map shape before any write: the
// producer namespace, the publication identity, the edition and the acting
// principal are all required. Nothing is inferred from a title or URL.
func validateBindingInput(namespace, publicationID, editionID, actor string) error {
	if strings.TrimSpace(namespace) == "" {
		return fmt.Errorf("%w: namespace is required", ErrInvalidEvent)
	}
	if strings.TrimSpace(publicationID) == "" {
		return fmt.Errorf("%w: publication_id is required", ErrInvalidEvent)
	}
	if strings.TrimSpace(editionID) == "" {
		return fmt.Errorf("%w: edition_id is required", ErrInvalidEvent)
	}
	if strings.TrimSpace(actor) == "" {
		return fmt.Errorf("%w: actor is required", ErrInvalidEvent)
	}
	return nil
}

// activeBindingTx loads the single active binding for one
// (namespace, publication_id) through a queryer, or nil when there is none.
func activeBindingTx(ctx context.Context, q queryer, namespace, publicationID string) (*ReleaseBinding, error) {
	row := q.QueryRowContext(ctx, `SELECT `+bindingColumns+`
		FROM import_bindings WHERE state = ? AND namespace = ? AND publication_id = ?`,
		BindingStateActive, namespace, publicationID)
	b, err := scanReleaseBinding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: find active release binding: %w", err)
	}
	return &b, nil
}

// requireEdition validates that the binding target edition exists, mapping a
// missing edition to the stable not-found error before any write.
func (d *Database) requireEdition(ctx context.Context, editionID string) error {
	if _, err := d.GetEdition(ctx, editionID); err != nil {
		return err
	}
	return nil
}

// BindReleaseBinding explicitly maps one confirmed producer publication to
// one edition. It is idempotent: the same (namespace, publication_id) bound
// to the same edition returns the existing active row unchanged. A
// publication already bound to a different edition is an explicit map
// conflict and is never silently re-pointed; use RebindReleaseBinding to
// correct a binding.
func (d *Database) BindReleaseBinding(ctx context.Context, namespace, publicationID, editionID, actor string, now time.Time) (ReleaseBinding, error) {
	if err := validateBindingInput(namespace, publicationID, editionID, actor); err != nil {
		return ReleaseBinding{}, err
	}
	if err := d.requireEdition(ctx, editionID); err != nil {
		return ReleaseBinding{}, err
	}
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return ReleaseBinding{}, err
	}
	defer tx.Rollback()
	existing, err := activeBindingTx(ctx, tx, namespace, publicationID)
	if err != nil {
		return ReleaseBinding{}, err
	}
	if existing != nil {
		if existing.EditionID == editionID {
			if err := tx.Commit(); err != nil {
				return ReleaseBinding{}, fmt.Errorf("store: commit binding no-op: %w", err)
			}
			return *existing, nil
		}
		return ReleaseBinding{}, fmt.Errorf("%w: publication %s/%s is already bound to edition %s",
			ErrBindingConflict, namespace, publicationID, existing.EditionID)
	}
	id, err := domain.NewUUID()
	if err != nil {
		return ReleaseBinding{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO import_bindings (id, namespace, publication_id, edition_id, state, created_by, created_at, replaces_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, NULL)`,
		id, namespace, publicationID, editionID, BindingStateActive, actor, instantUTC(now)); err != nil {
		return ReleaseBinding{}, fmt.Errorf("store: insert release binding: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ReleaseBinding{}, fmt.Errorf("store: commit release binding: %w", err)
	}
	return ReleaseBinding{
		ID: id, Namespace: namespace, PublicationID: publicationID, EditionID: editionID,
		State: BindingStateActive, CreatedBy: actor, CreatedAt: now,
	}, nil
}

// RebindReleaseBinding explicitly corrects one publication's binding. In one
// transaction it revokes the current active binding (recording who and when)
// and creates its replacement, linked by replaces_id, so the old binding and
// the raw event history stay intact. With no active binding it simply
// creates one. The replaced projection disappears because only active
// bindings project; no duplicate active projection is left behind.
func (d *Database) RebindReleaseBinding(ctx context.Context, namespace, publicationID, newEditionID, actor string, now time.Time) (ReleaseBinding, error) {
	if err := validateBindingInput(namespace, publicationID, newEditionID, actor); err != nil {
		return ReleaseBinding{}, err
	}
	if err := d.requireEdition(ctx, newEditionID); err != nil {
		return ReleaseBinding{}, err
	}
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return ReleaseBinding{}, err
	}
	defer tx.Rollback()
	existing, err := activeBindingTx(ctx, tx, namespace, publicationID)
	if err != nil {
		return ReleaseBinding{}, err
	}
	if existing != nil && existing.EditionID == newEditionID {
		// Rebinding a publication to the edition it already targets is an
		// idempotent no-op: return the current binding without inserting a
		// duplicate active row.
		if err := tx.Commit(); err != nil {
			return ReleaseBinding{}, fmt.Errorf("store: commit rebind no-op: %w", err)
		}
		return *existing, nil
	}
	var replacesID *string
	if existing != nil && existing.EditionID != newEditionID {
		if _, err := tx.ExecContext(ctx,
			`UPDATE import_bindings SET state = ?, revoked_by = ?, revoked_at = ? WHERE id = ?`,
			BindingStateRevoked, actor, instantUTC(now), existing.ID); err != nil {
			return ReleaseBinding{}, fmt.Errorf("store: revoke replaced release binding: %w", err)
		}
		replacesID = &existing.ID
	}
	id, err := domain.NewUUID()
	if err != nil {
		return ReleaseBinding{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO import_bindings (id, namespace, publication_id, edition_id, state, created_by, created_at, replaces_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, namespace, publicationID, newEditionID, BindingStateActive, actor, instantUTC(now), replacesID); err != nil {
		return ReleaseBinding{}, fmt.Errorf("store: insert replaced release binding: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ReleaseBinding{}, fmt.Errorf("store: commit rebind: %w", err)
	}
	return ReleaseBinding{
		ID: id, Namespace: namespace, PublicationID: publicationID, EditionID: newEditionID,
		State: BindingStateActive, CreatedBy: actor, CreatedAt: now, ReplacesID: replacesID,
	}, nil
}

// RevokeReleaseBinding revokes the active binding of one publication,
// un-projecting its events while keeping the binding row and the raw event
// history. Revoking a publication with no active binding is a distinguishable
// not-found, never a silent success.
func (d *Database) RevokeReleaseBinding(ctx context.Context, namespace, publicationID, actor string, now time.Time) error {
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(publicationID) == "" || strings.TrimSpace(actor) == "" {
		return fmt.Errorf("%w: namespace, publication_id and actor are required", ErrInvalidEvent)
	}
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	existing, err := activeBindingTx(ctx, tx, namespace, publicationID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("%w: no active binding for %s/%s", ErrBindingNotFound, namespace, publicationID)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE import_bindings SET state = ?, revoked_by = ?, revoked_at = ? WHERE id = ?`,
		BindingStateRevoked, actor, instantUTC(now), existing.ID); err != nil {
		return fmt.Errorf("store: revoke release binding: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit release binding revoke: %w", err)
	}
	return nil
}

// GetReleaseBinding returns the active binding of one publication, or nil
// when there is none. The full binding history (active and revoked) is read
// by the caller through ListReleaseBindings.
func (d *Database) GetReleaseBinding(ctx context.Context, namespace, publicationID string) (*ReleaseBinding, error) {
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(publicationID) == "" {
		return nil, fmt.Errorf("%w: namespace and publication_id are required", ErrInvalidEvent)
	}
	return activeBindingTx(ctx, d.db, namespace, publicationID)
}

// ListReleaseProjections projects the release history of one edition: the
// imported events made visible by every active binding of the edition,
// ordered by (created_at, event_id), keyset-paginated on the same pair. The
// opaque after and next cursors are "created_at|event_id". The total counts
// before pagination.
func (d *Database) ListReleaseProjections(ctx context.Context, editionID string, limit int, after *string) ([]ReleaseProjection, int64, *string, error) {
	if strings.TrimSpace(editionID) == "" {
		return nil, 0, nil, fmt.Errorf("%w: edition_id is required", ErrInvalidEvent)
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	const fromClause = `
		FROM import_events e
		JOIN import_bindings b ON b.state = ? AND b.namespace = e.namespace AND b.publication_id = e.publication_id
		JOIN editions ed ON ed.id = b.edition_id
		WHERE b.edition_id = ?`
	var total int64
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) `+fromClause,
		BindingStateActive, editionID).Scan(&total); err != nil {
		return nil, 0, nil, fmt.Errorf("store: count release projections: %w", err)
	}
	query := `SELECT e.event_id, e.event_type, e.namespace, e.publication_id,
		b.edition_id, ed.work_id, e.release_id, e.release_number, e.release_title,
		e.release_url, e.current_count, e.count_delta, e.source_time,
		e.detection_time, e.created_at ` + fromClause
	args := []any{BindingStateActive, editionID}
	if after != nil {
		createdAt, eventID, ok := splitReleaseCursor(*after)
		if !ok {
			return nil, 0, nil, fmt.Errorf("%w: malformed release cursor", ErrInvalidEvent)
		}
		query += ` AND (e.created_at > ? OR (e.created_at = ? AND e.event_id > ?))`
		args = append(args, createdAt, createdAt, eventID)
	}
	query += ` ORDER BY e.created_at, e.event_id LIMIT ?`
	args = append(args, limit+1)
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("store: list release projections: %w", err)
	}
	defer rows.Close()
	out := []ReleaseProjection{}
	var next *string
	for rows.Next() {
		var (
			p             ReleaseProjection
			publicationID sql.NullString
			releaseID     sql.NullString
			releaseNumber sql.NullString
			releaseTitle  sql.NullString
			releaseURL    sql.NullString
			currentCount  sql.NullInt64
			countDelta    sql.NullInt64
			sourceTime    sql.NullString
			detectionTime sql.NullString
			createdAt     string
		)
		if err := rows.Scan(&p.EventID, &p.EventType, &p.Namespace, &publicationID,
			&p.EditionID, &p.WorkID, &releaseID, &releaseNumber, &releaseTitle,
			&releaseURL, &currentCount, &countDelta, &sourceTime,
			&detectionTime, &createdAt); err != nil {
			return nil, 0, nil, fmt.Errorf("store: scan release projection: %w", err)
		}
		if publicationID.Valid {
			p.PublicationID = &publicationID.String
		}
		if releaseID.Valid {
			p.ReleaseID = &releaseID.String
		}
		if releaseNumber.Valid {
			p.ReleaseNumber = &releaseNumber.String
		}
		if releaseTitle.Valid {
			p.ReleaseTitle = &releaseTitle.String
		}
		if releaseURL.Valid {
			p.ReleaseURL = &releaseURL.String
		}
		if currentCount.Valid {
			v := int(currentCount.Int64)
			p.CurrentCount = &v
		}
		if countDelta.Valid {
			v := int(countDelta.Int64)
			p.CountDelta = &v
		}
		if sourceTime.Valid {
			p.SourceTime = &sourceTime.String
		}
		if detectionTime.Valid {
			p.DetectionTime = &detectionTime.String
		}
		t, err := parseInstant(createdAt)
		if err != nil {
			return nil, 0, nil, fmt.Errorf("store: parse projection created_at: %w", err)
		}
		p.CreatedAt = t
		if len(out) == limit {
			last := out[len(out)-1]
			next = ptrJoin(instantUTC(last.CreatedAt), last.EventID)
			break
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, nil, fmt.Errorf("store: list release projections: %w", err)
	}
	return out, total, next, nil
}

// ListReleaseBindings returns the full audited binding history (active and
// revoked) of one publication, oldest first. A correction is a new active
// row plus a revoked predecessor, so the history is complete and ordered.
func (d *Database) ListReleaseBindings(ctx context.Context, namespace, publicationID string) ([]ReleaseBinding, error) {
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(publicationID) == "" {
		return nil, fmt.Errorf("%w: namespace and publication_id are required", ErrInvalidEvent)
	}
	rows, err := d.db.QueryContext(ctx,
		`SELECT `+bindingColumns+` FROM import_bindings
		 WHERE namespace = ? AND publication_id = ? ORDER BY created_at, id`,
		namespace, publicationID)
	if err != nil {
		return nil, fmt.Errorf("store: list release bindings: %w", err)
	}
	defer rows.Close()
	out := []ReleaseBinding{}
	for rows.Next() {
		b, err := scanReleaseBinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func splitReleaseCursor(s string) (createdAt, eventID string, ok bool) {
	i := strings.IndexByte(s, '|')
	if i < 0 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}
