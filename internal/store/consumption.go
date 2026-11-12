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

// Consumption failures. Application services map these to the stable wire
// error codes; the store stays transport-agnostic.
var (
	ErrInvalidConsumption  = errors.New("store: invalid consumption")
	ErrConsumptionNotFound = errors.New("store: consumption observation not found")
)

// ConsumptionUnknown is the explicit retraction value. It records the
// user's action of withdrawing the state to unknown; the current-state
// selection treats it as unknown, views render it as the absence of a
// consumption state, and it stays in history distinct from an edition that
// never had an observation (unknown historical consumption).
const ConsumptionUnknown = "unknown"

// ValidConsumptionValue reports whether v is a storable consumption value:
// one of the five consumption states or the explicit retraction value.
func ValidConsumptionValue(v string) bool {
	if v == ConsumptionUnknown {
		return true
	}
	return domain.Consumption(v).Valid()
}

// ConsumptionActor is the acting identity recorded on a consumption
// observation: the transport-set principal plus the asserted origin and
// identity. It is stored as canonical JSON in the actor column.
type ConsumptionActor struct {
	Principal string `json:"principal"`
	Origin    string `json:"origin,omitempty"`
	Identity  string `json:"identity,omitempty"`
}

// ConsumptionInput is one requested consumption observation.
type ConsumptionInput struct {
	Value        string
	Reason       *string
	ObservedAt   time.Time
	SupersedesID *string
}

// ConsumptionObservation is one stored consumption history row.
type ConsumptionObservation struct {
	ID           string
	EditionID    string
	Value        string
	Reason       *string
	ObservedAt   time.Time
	RecordedAt   time.Time
	Actor        ConsumptionActor
	SupersedesID *string
}

// currentConsumptionValue reports the effective current value of an
// edition: the last live observation's value, or ConsumptionUnknown when
// the edition has no live observation at all.
func currentConsumptionValue(o *ConsumptionObservation) string {
	if o == nil {
		return ConsumptionUnknown
	}
	return o.Value
}

// currentConsumption selects the last non-retracted observation by
// (observed_at, recorded_at, id). A row is retracted only when another row
// supersedes it; the superseding correction row itself is live. A
// past-dated observation never overtakes a newer one.
func (d *Database) currentConsumption(ctx context.Context, editionID string) (*ConsumptionObservation, error) {
	return d.currentConsumptionQuery(ctx, d.db, editionID)
}

// currentConsumptionTx is the in-transaction variant of currentConsumption.
func (d *Database) currentConsumptionTx(ctx context.Context, tx *sql.Tx, editionID string) (*ConsumptionObservation, error) {
	return d.currentConsumptionQuery(ctx, tx, editionID)
}

func (d *Database) currentConsumptionQuery(ctx context.Context, q queryer, editionID string) (*ConsumptionObservation, error) {
	var (
		o          ConsumptionObservation
		observedAt string
		recordedAt string
		value      string
		actor      string
		reason     sql.NullString
		supersedes sql.NullString
	)
	err := q.QueryRowContext(ctx, `
		SELECT o.id, o.value, o.observed_at, o.recorded_at, o.actor, o.reason, o.supersedes_id
		FROM consumption_observations o
		WHERE o.edition_id = ?
		  AND NOT EXISTS (SELECT 1 FROM consumption_observations s WHERE s.supersedes_id = o.id)
		ORDER BY o.observed_at DESC, o.recorded_at DESC, o.id DESC
		LIMIT 1`, editionID).
		Scan(&o.ID, &value, &observedAt, &recordedAt, &actor, &reason, &supersedes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: query current consumption: %w", err)
	}
	var a ConsumptionActor
	if err := json.Unmarshal([]byte(actor), &a); err != nil {
		return nil, fmt.Errorf("store: parse consumption actor: %w", err)
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
		return nil, fmt.Errorf("store: parse consumption observed_at: %w", err)
	}
	o.RecordedAt, err = parseInstant(recordedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse consumption recorded_at: %w", err)
	}
	return &o, nil
}

// checkConsumptionSupersede verifies a correction target: an existing
// observation of the same edition that has not been superseded yet.
func (d *Database) checkConsumptionSupersede(ctx context.Context, tx *sql.Tx, editionID, targetID string) error {
	var id string
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM consumption_observations WHERE id = ? AND edition_id = ?`,
		targetID, editionID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrConsumptionNotFound, targetID)
	}
	if err != nil {
		return fmt.Errorf("store: read superseded consumption: %w", err)
	}
	var superseding int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM consumption_observations WHERE supersedes_id = ?`, targetID).Scan(&superseding); err != nil {
		return fmt.Errorf("store: check consumption supersede: %w", err)
	}
	if superseding != 0 {
		return fmt.Errorf("%w: %s is already superseded", ErrSupersedeConflict, targetID)
	}
	return nil
}

// SetConsumption records one personal consumption observation for an
// edition. The current state is the last non-superseded observation by
// (observed_at, recorded_at, id). A request for the same current value is a
// no-op without a revision bump; a supersedes_id correction targets an
// existing, not-yet-superseded observation of the same edition and always
// applies. Past-dated observations are stored but do not become current
// over a newer one. All history rows are preserved, and the optional
// journal row commits in the same transaction. The edition revision guards
// against lost updates.
func (d *Database) SetConsumption(ctx context.Context, now time.Time, editionID string, expectedRevision int64, in ConsumptionInput, actor ConsumptionActor, journal *JournalInput) (Edition, error) {
	if !ValidConsumptionValue(in.Value) {
		return Edition{}, fmt.Errorf("%w: %s", ErrInvalidConsumption, in.Value)
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
		return Edition{}, fmt.Errorf("store: encode consumption actor: %w", err)
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
	current, err := d.currentConsumptionTx(ctx, tx, editionID)
	if err != nil {
		return Edition{}, err
	}
	noOp := in.SupersedesID == nil && currentConsumptionValue(current) == in.Value
	if !noOp {
		if in.SupersedesID != nil {
			if err := d.checkConsumptionSupersede(ctx, tx, editionID, *in.SupersedesID); err != nil {
				return Edition{}, err
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO consumption_observations (id, edition_id, value, observed_at, recorded_at, actor, reason, supersedes_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, editionID, in.Value, instantUTC(observedAt), instantUTC(now), string(actorJSON), in.Reason, in.SupersedesID); err != nil {
			return Edition{}, fmt.Errorf("store: insert consumption observation: %w", err)
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
		return Edition{}, fmt.Errorf("store: commit consumption: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}

// insertDefaultConsumptionTx records the not_started default of a newly
// manually created edition inside the caller's transaction, so a fresh
// manual edition starts not_started while an imported or historical edition
// without observations stays unknown.
func insertDefaultConsumptionTx(ctx context.Context, tx *sql.Tx, editionID string, actor ConsumptionActor, nowS string) error {
	id, err := domain.NewUUID()
	if err != nil {
		return err
	}
	actorJSON, err := json.Marshal(actor)
	if err != nil {
		return fmt.Errorf("store: encode consumption actor: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO consumption_observations (id, edition_id, value, observed_at, recorded_at, actor, reason, supersedes_id)
		 VALUES (?, ?, ?, ?, ?, ?, NULL, NULL)`,
		id, editionID, string(domain.ConsumptionNotStarted), nowS, nowS, string(actorJSON)); err != nil {
		return fmt.Errorf("store: insert default consumption: %w", err)
	}
	return nil
}
