package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

// Intention failures. Application services map these to the stable wire
// error codes; the store stays transport-agnostic.
var (
	ErrInvalidIntention  = errors.New("store: invalid intention")
	ErrIntentionNotFound = errors.New("store: intention observation not found")
)

// IntentionNone is the explicit retraction value. It records the user's
// action of withdrawing the intention; the current-state selection keeps it
// in history, and views render it as the absence of a recorded intention.
// An edition that never had an intention observation is also without an
// intention; both are absence, never a value.
const IntentionNone = "none"

// ValidIntentionValue reports whether v is a storable intention value: one
// of the five bounded intention values including the explicit retraction.
func ValidIntentionValue(v string) bool {
	return domain.Intention(v).Valid()
}

// IntentionActor is the acting identity recorded on an intention
// observation: the transport-set principal plus the asserted origin and
// identity. It is stored as canonical JSON in the actor column.
type IntentionActor struct {
	Principal string `json:"principal"`
	Origin    string `json:"origin,omitempty"`
	Identity  string `json:"identity,omitempty"`
}

// IntentionInput is one requested intention observation.
type IntentionInput struct {
	Value        string
	Reason       *string
	ObservedAt   time.Time
	SupersedesID *string
}

// IntentionObservation is one stored intention history row.
type IntentionObservation struct {
	ID           string
	EditionID    string
	Value        string
	Reason       *string
	ObservedAt   time.Time
	RecordedAt   time.Time
	Actor        IntentionActor
	SupersedesID *string
}

// currentIntentionValue reports the effective current intention of an
// edition: the last live observation's value, or IntentionNone when the
// edition has no live observation at all.
func currentIntentionValue(o *IntentionObservation) string {
	if o == nil {
		return IntentionNone
	}
	return o.Value
}

// currentIntention selects the last non-superseded observation by
// (observed_at, recorded_at, id). A row is superseded only when another row
// supersedes it; the superseding correction row itself is live. A
// past-dated observation never overtakes a newer one.
func (d *Database) currentIntention(ctx context.Context, editionID string) (*IntentionObservation, error) {
	return d.currentIntentionQuery(ctx, d.db, editionID)
}

// currentIntentionTx is the in-transaction variant of currentIntention.
func (d *Database) currentIntentionTx(ctx context.Context, tx *sql.Tx, editionID string) (*IntentionObservation, error) {
	return d.currentIntentionQuery(ctx, tx, editionID)
}

func (d *Database) currentIntentionQuery(ctx context.Context, q queryer, editionID string) (*IntentionObservation, error) {
	var (
		o          IntentionObservation
		observedAt string
		recordedAt string
		value      string
		actor      string
		reason     sql.NullString
		supersedes sql.NullString
	)
	err := q.QueryRowContext(ctx, `
		SELECT o.id, o.value, o.observed_at, o.recorded_at, o.actor, o.reason, o.supersedes_id
		FROM intention_observations o
		WHERE o.edition_id = ?
		  AND NOT EXISTS (SELECT 1 FROM intention_observations s WHERE s.supersedes_id = o.id)
		ORDER BY o.observed_at DESC, o.recorded_at DESC, o.id DESC
		LIMIT 1`, editionID).
		Scan(&o.ID, &value, &observedAt, &recordedAt, &actor, &reason, &supersedes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: query current intention: %w", err)
	}
	var a IntentionActor
	if err := json.Unmarshal([]byte(actor), &a); err != nil {
		return nil, fmt.Errorf("store: parse intention actor: %w", err)
	}
	o.EditionID = editionID
	o.Value = value
	o.Actor = a
	if reason.Valid {
		o.Reason = &reason.String
	}
	if supersedes.Valid {
		o.SupersedesID = &supersedes.String
	}
	o.ObservedAt, err = parseInstant(observedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse intention observed_at: %w", err)
	}
	o.RecordedAt, err = parseInstant(recordedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse intention recorded_at: %w", err)
	}
	return &o, nil
}

// checkIntentionSupersede verifies a correction target: an existing
// observation of the same edition that has not been superseded yet.
func (d *Database) checkIntentionSupersede(ctx context.Context, tx *sql.Tx, editionID, targetID string) error {
	var id string
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM intention_observations WHERE id = ? AND edition_id = ?`,
		targetID, editionID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrIntentionNotFound, targetID)
	}
	if err != nil {
		return fmt.Errorf("store: read superseded intention: %w", err)
	}
	var superseding int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM intention_observations WHERE supersedes_id = ?`, targetID).Scan(&superseding); err != nil {
		return fmt.Errorf("store: check intention supersede: %w", err)
	}
	if superseding != 0 {
		return fmt.Errorf("%w: %s is already superseded", ErrSupersedeConflict, targetID)
	}
	return nil
}

// SetIntention records one personal intention observation for an edition.
// The current intention is the last non-superseded observation by
// (observed_at, recorded_at, id). A request for the same current value is a
// no-op without a revision bump; a supersedes_id correction targets an
// existing, not-yet-superseded observation of the same edition and always
// applies; the explicit none value retracts to no intention and renders as
// absence. Past-dated observations are stored but do not become current
// over a newer one. All history rows are preserved, and the optional
// journal row commits in the same transaction. The edition revision guards
// against lost updates.
func (d *Database) SetIntention(ctx context.Context, now time.Time, editionID string, expectedRevision int64, in IntentionInput, actor IntentionActor, journal *JournalInput) (Edition, error) {
	if !ValidIntentionValue(in.Value) {
		return Edition{}, fmt.Errorf("%w: %s", ErrInvalidIntention, in.Value)
	}
	id, err := domain.NewUUID()
	if err != nil {
		return Edition{}, err
	}
	observedAt := in.ObservedAt
	if observedAt.IsZero() {
		observedAt = now
	}
	actorJSON, err := json.Marshal(actor)
	if err != nil {
		return Edition{}, fmt.Errorf("store: encode intention actor: %w", err)
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
	current, err := d.currentIntentionTx(ctx, tx, editionID)
	if err != nil {
		return Edition{}, err
	}
	noOp := in.SupersedesID == nil && currentIntentionValue(current) == in.Value
	if !noOp {
		if in.SupersedesID != nil {
			if err := d.checkIntentionSupersede(ctx, tx, editionID, *in.SupersedesID); err != nil {
				return Edition{}, err
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO intention_observations (id, edition_id, value, observed_at, recorded_at, actor, reason, supersedes_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, editionID, in.Value, instantUTC(observedAt), instantUTC(now), string(actorJSON), in.Reason, in.SupersedesID); err != nil {
			return Edition{}, fmt.Errorf("store: insert intention observation: %w", err)
		}
		if err := d.bumpEditionRevision(tx, ctx, editionID, revision, now); err != nil {
			return Edition{}, err
		}
		revision++
	}
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("edition_id", editionID, revision)); err != nil {
		return Edition{}, err
	}
	if err := tx.Commit(); err != nil {
		return Edition{}, fmt.Errorf("store: commit intention: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}
