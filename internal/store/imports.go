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

// Import event and binding failures. The application layer maps these to
// the stable wire error codes; the store stays transport-agnostic.
var (
	ErrEventConflict = errors.New("store: import event identity conflict")
	ErrEventNotFound = errors.New("store: import event not found")
	ErrInvalidEvent  = errors.New("store: invalid import event")
)

// Event outcome statuses, the idempotent result of persisting one event.
const (
	EventStatusImported = "imported"
	EventStatusReplayed = "replayed"
)

// EventInput is one decoded, validated Release Watcher event to persist.
// The raw and canonical payloads are preserved verbatim; the typed fields
// are the queryable projection and may be nil when the producer omitted
// them.
type EventInput struct {
	ID              string
	EventID         string
	SchemaVersion   string
	EventType       string
	RawPayload      string
	Canonical       string
	Namespace       string
	PublicationID   *string
	ExternalWorkRef *string
	ReleaseID       *string
	ReleaseNumber   *string
	ReleaseTitle    *string
	ReleaseURL      *string
	CurrentCount    *int
	CountDelta      *int
	SourceTime      *string
	DetectionTime   *string
	WorkHint        *string
	EditionHint     *string
	CreatedAt       time.Time
}

// EventOutcome is the store outcome of one idempotent event insert.
type EventOutcome struct {
	EventID string
	Status  string
}

// importEventRow is the full stored event row.
type importEventRow struct {
	ID              string
	EventID         string
	SchemaVersion   string
	EventType       string
	RawPayload      string
	Canonical       string
	Namespace       string
	PublicationID   *string
	ExternalWorkRef *string
	ReleaseID       *string
	ReleaseNumber   *string
	ReleaseTitle    *string
	ReleaseURL      *string
	CurrentCount    *int
	CountDelta      *int
	SourceTime      *string
	DetectionTime   *string
	WorkHint        *string
	EditionHint     *string
	CreatedAt       time.Time
}

// PendingEvent is one import event with no active binding, in the pending
// queue. The typed fields mirror the stored projection.
type PendingEvent struct {
	ID              string
	EventID         string
	EventType       string
	Namespace       string
	PublicationID   *string
	ExternalWorkRef *string
	ReleaseID       *string
	ReleaseNumber   *string
	ReleaseTitle    *string
	ReleaseURL      *string
	CurrentCount    *int
	CountDelta      *int
	SourceTime      *string
	DetectionTime   *string
	CreatedAt       time.Time
}

// eventColumns is the full import event row in a stable order.
const eventColumns = `id, event_id, schema_version, event_type, raw_payload, canonical_payload,
	namespace, publication_id, external_work_ref, release_id, release_number,
	release_title, release_url, current_count, count_delta, source_time,
	detection_time, work_hint, edition_hint, created_at`

// pendingFilter selects events with no active binding for their
// (namespace, publication_id); an event without a publication id is always
// pending.
// The disjunction is parenthesized so a cursor AND applied by the caller
// binds to the whole predicate: (IS NULL OR NOT EXISTS) AND <cursor>, not
// IS NULL OR (NOT EXISTS AND <cursor>).
const pendingFilter = `(e.publication_id IS NULL
	OR NOT EXISTS (SELECT 1 FROM import_bindings b
	               WHERE b.state = 'active' AND b.namespace = e.namespace AND b.publication_id = e.publication_id))`

// scanImportEventRow fills one full event row from a scanner.
func scanImportEventRow(s interface {
	Scan(dest ...any) error
}) (importEventRow, error) {
	var (
		e             importEventRow
		publicationID sql.NullString
		externalRef   sql.NullString
		releaseID     sql.NullString
		releaseNumber sql.NullString
		releaseTitle  sql.NullString
		releaseURL    sql.NullString
		currentCount  sql.NullInt64
		countDelta    sql.NullInt64
		sourceTime    sql.NullString
		detectionTime sql.NullString
		workHint      sql.NullString
		editionHint   sql.NullString
		createdAt     string
	)
	if err := s.Scan(&e.ID, &e.EventID, &e.SchemaVersion, &e.EventType, &e.RawPayload, &e.Canonical,
		&e.Namespace, &publicationID, &externalRef, &releaseID, &releaseNumber,
		&releaseTitle, &releaseURL, &currentCount, &countDelta, &sourceTime,
		&detectionTime, &workHint, &editionHint, &createdAt); err != nil {
		return importEventRow{}, err
	}
	if publicationID.Valid {
		e.PublicationID = &publicationID.String
	}
	if externalRef.Valid {
		e.ExternalWorkRef = &externalRef.String
	}
	if releaseID.Valid {
		e.ReleaseID = &releaseID.String
	}
	if releaseNumber.Valid {
		e.ReleaseNumber = &releaseNumber.String
	}
	if releaseTitle.Valid {
		e.ReleaseTitle = &releaseTitle.String
	}
	if releaseURL.Valid {
		e.ReleaseURL = &releaseURL.String
	}
	if currentCount.Valid {
		v := int(currentCount.Int64)
		e.CurrentCount = &v
	}
	if countDelta.Valid {
		v := int(countDelta.Int64)
		e.CountDelta = &v
	}
	if sourceTime.Valid {
		e.SourceTime = &sourceTime.String
	}
	if detectionTime.Valid {
		e.DetectionTime = &detectionTime.String
	}
	if workHint.Valid {
		e.WorkHint = &workHint.String
	}
	if editionHint.Valid {
		e.EditionHint = &editionHint.String
	}
	t, err := parseInstant(createdAt)
	if err != nil {
		return importEventRow{}, fmt.Errorf("store: parse import event created_at: %w", err)
	}
	e.CreatedAt = t
	return e, nil
}

// validateEventInput checks the event shape before any write: a non-empty
// idempotency key, schema version, event type, namespace, canonical payload
// and creation instant.
func validateEventInput(in EventInput) error {
	if strings.TrimSpace(in.EventID) == "" {
		return fmt.Errorf("%w: event_id is required", ErrInvalidEvent)
	}
	if in.SchemaVersion == "" {
		return fmt.Errorf("%w: schema_version is required", ErrInvalidEvent)
	}
	if strings.TrimSpace(in.EventType) == "" {
		return fmt.Errorf("%w: event_type is required", ErrInvalidEvent)
	}
	if strings.TrimSpace(in.Namespace) == "" {
		return fmt.Errorf("%w: namespace is required", ErrInvalidEvent)
	}
	if strings.TrimSpace(in.Canonical) == "" {
		return fmt.Errorf("%w: canonical payload is required", ErrInvalidEvent)
	}
	if in.CreatedAt.IsZero() {
		return fmt.Errorf("%w: created_at is required", ErrInvalidEvent)
	}
	return nil
}

// findEvent loads the event row by its idempotency key, or nil.
func findEvent(ctx context.Context, q queryer, eventID string) (*importEventRow, error) {
	row := q.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM import_events WHERE event_id = ?`, eventID)
	e, err := scanImportEventRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: find import event: %w", err)
	}
	return &e, nil
}

// InsertEvent persists one Release Watcher event idempotently in a single
// event-level transaction. The same event_id with a semantically identical
// canonical payload is a no-op replay; the same event_id with a different
// canonical payload is an identity conflict and is never an overwrite. A
// new event_id inserts the immutable row.
func (d *Database) InsertEvent(ctx context.Context, in EventInput) (EventOutcome, error) {
	if err := validateEventInput(in); err != nil {
		return EventOutcome{}, err
	}
	id, err := domain.NewUUID()
	if err != nil {
		return EventOutcome{}, err
	}
	nowS := instantUTC(in.CreatedAt)
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return EventOutcome{}, err
	}
	defer tx.Rollback()
	existing, err := findEvent(ctx, tx, in.EventID)
	if err != nil {
		return EventOutcome{}, err
	}
	if existing != nil {
		if existing.Canonical == in.Canonical {
			// An identical replay: a no-op, the immutable row is untouched.
			if err := tx.Commit(); err != nil {
				return EventOutcome{}, fmt.Errorf("store: commit event replay: %w", err)
			}
			return EventOutcome{EventID: in.EventID, Status: EventStatusReplayed}, nil
		}
		// Same event_id, different payload: an identity conflict. The
		// existing row is never rewritten.
		return EventOutcome{}, fmt.Errorf("%w: event %s already exists with a different payload", ErrEventConflict, in.EventID)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO import_events (`+eventColumns+`)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.EventID, in.SchemaVersion, in.EventType, in.RawPayload, in.Canonical,
		in.Namespace, in.PublicationID, in.ExternalWorkRef, in.ReleaseID, in.ReleaseNumber,
		in.ReleaseTitle, in.ReleaseURL, in.CurrentCount, in.CountDelta, in.SourceTime,
		in.DetectionTime, in.WorkHint, in.EditionHint, nowS); err != nil {
		return EventOutcome{}, fmt.Errorf("store: insert import event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return EventOutcome{}, fmt.Errorf("store: commit import event: %w", err)
	}
	return EventOutcome{EventID: in.EventID, Status: EventStatusImported}, nil
}

// HasActiveBinding reports whether one active binding links the namespace +
// publication id to an edition.
func (d *Database) HasActiveBinding(ctx context.Context, namespace, publicationID string) (bool, error) {
	var n int
	if err := d.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM import_bindings WHERE state = 'active' AND namespace = ? AND publication_id = ?`,
		namespace, publicationID).Scan(&n); err != nil {
		return false, fmt.Errorf("store: count active import binding: %w", err)
	}
	return n > 0, nil
}

// ListPendingEvents lists import events with no active binding, oldest
// first, keyset-paginated on (created_at, event_id). The after and next
// cursors are opaque: "created_at|event_id". The total is the count of all
// pending events before pagination.
func (d *Database) ListPendingEvents(ctx context.Context, limit int, after *string) ([]PendingEvent, int64, *string, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	var total int64
	if err := d.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM import_events e WHERE `+pendingFilter).Scan(&total); err != nil {
		return nil, 0, nil, fmt.Errorf("store: count pending import events: %w", err)
	}
	query := `SELECT ` + eventColumns + ` FROM import_events e
	 WHERE ` + pendingFilter
	args := []any{}
	if after != nil {
		createdAt, eventID, ok := splitPendingCursor(*after)
		if !ok {
			return nil, 0, nil, fmt.Errorf("%w: malformed pending cursor", ErrInvalidEvent)
		}
		query += ` AND (e.created_at > ? OR (e.created_at = ? AND e.event_id > ?))`
		args = append(args, createdAt, createdAt, eventID)
	}
	query += ` ORDER BY e.created_at, e.event_id LIMIT ?`
	args = append(args, limit+1)
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("store: list pending import events: %w", err)
	}
	defer rows.Close()
	out := []PendingEvent{}
	var next *string
	for rows.Next() {
		e, err := scanImportEventRow(rows)
		if err != nil {
			return nil, 0, nil, err
		}
		if len(out) == limit {
			// A probe row exists: the next cursor points at the last
			// returned row so the next page resumes exactly after it.
			last := out[len(out)-1]
			next = ptrJoin(instantUTC(last.CreatedAt), last.EventID)
			break
		}
		out = append(out, toPendingEvent(e))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, nil, fmt.Errorf("store: list pending import events: %w", err)
	}
	return out, total, next, nil
}

func toPendingEvent(e importEventRow) PendingEvent {
	return PendingEvent{
		ID: e.ID, EventID: e.EventID, EventType: e.EventType, Namespace: e.Namespace,
		PublicationID: e.PublicationID, ExternalWorkRef: e.ExternalWorkRef,
		ReleaseID: e.ReleaseID, ReleaseNumber: e.ReleaseNumber, ReleaseTitle: e.ReleaseTitle,
		ReleaseURL: e.ReleaseURL, CurrentCount: e.CurrentCount, CountDelta: e.CountDelta,
		SourceTime: e.SourceTime, DetectionTime: e.DetectionTime, CreatedAt: e.CreatedAt,
	}
}

func ptrJoin(a, b string) *string {
	s := a + "|" + b
	return &s
}

func splitPendingCursor(s string) (createdAt, eventID string, ok bool) {
	i := strings.IndexByte(s, '|')
	if i < 0 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}
