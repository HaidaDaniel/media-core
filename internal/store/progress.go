package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

// Progress failures. Application services map these to the stable wire
// error codes; the store stays transport-agnostic.
var (
	ErrInvalidProgress  = errors.New("store: invalid progress")
	ErrProgressNotFound = errors.New("store: progress observation not found")
)

// maxProgressMilli bounds stored positions and totals to 1,000,000,000 in
// milli-units (three fractional digits), so the bound check is exact.
const maxProgressMilli = 1_000_000_000_000

// ProgressActor is the acting identity recorded on a progress observation:
// the transport-set principal plus the asserted origin and identity. It is
// stored as canonical JSON in the actor column.
type ProgressActor struct {
	Principal string `json:"principal"`
	Origin    string `json:"origin,omitempty"`
	Identity  string `json:"identity,omitempty"`
}

// ProgressInput is one requested progress observation.
type ProgressInput struct {
	Unit         domain.ProgressUnit
	Position     string
	Total        *string
	Label        *string
	Reason       *string
	ObservedAt   time.Time
	SupersedesID *string
}

// ProgressObservation is one stored progress history row. Position is nil
// only for an explicit retraction marker row.
type ProgressObservation struct {
	ID           string
	EditionID    string
	Unit         domain.ProgressUnit
	Position     *string
	Total        *string
	Label        *string
	Reason       *string
	ObservedAt   time.Time
	RecordedAt   time.Time
	Actor        ProgressActor
	SupersedesID *string
}

// ParseProgressDecimal parses one exact decimal value of at most
// 1,000,000,000 with up to three fractional digits. It accepts neither a
// sign, nor leading zeros, nor more than three fractional digits, and
// returns the value scaled by 1000.
func ParseProgressDecimal(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	intPart := s
	frac := 0
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		intPart = s[:dot]
		f := s[dot+1:]
		if len(f) == 0 || len(f) > 3 {
			return 0, false
		}
		for _, c := range f {
			if c < '0' || c > '9' {
				return 0, false
			}
			frac = frac*10 + int(c-'0')
		}
	}
	if intPart == "" || (len(intPart) > 1 && intPart[0] == '0') {
		return 0, false
	}
	var n int64
	for _, c := range intPart {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
	}
	milli := n*1000 + int64(frac)
	if milli > maxProgressMilli {
		return 0, false
	}
	return milli, true
}

// ValidateProgressValues checks the unit/position/total rules: a non-
// negative exact decimal position (page integral, percent 0-100 with an
// implicit total of 100 and no explicit total, all values at most
// 1,000,000,000) and a known total that is positive and at least the
// position.
func ValidateProgressValues(unit domain.ProgressUnit, position string, total *string) error {
	if !unit.Valid() {
		return fmt.Errorf("unknown progress unit %q", unit)
	}
	posMilli, ok := ParseProgressDecimal(position)
	if !ok {
		return fmt.Errorf("invalid position %q: non-negative decimal with up to 3 fractional digits, at most 1000000000", position)
	}
	if unit == domain.UnitPage && strings.Contains(position, ".") {
		return fmt.Errorf("page position %q must be integral", position)
	}
	if unit == domain.UnitPercent {
		if total != nil {
			return fmt.Errorf("percent progress has an implicit total of 100; do not supply a total")
		}
		if posMilli > 100*1000 {
			return fmt.Errorf("percent position %q is above 100", position)
		}
	}
	if total != nil {
		totalMilli, ok := ParseProgressDecimal(*total)
		if !ok {
			return fmt.Errorf("invalid total %q: non-negative decimal with up to 3 fractional digits, at most 1000000000", *total)
		}
		if totalMilli <= 0 {
			return fmt.Errorf("total %q must be positive", *total)
		}
		if totalMilli < posMilli {
			return fmt.Errorf("position %q is above known total %q", position, *total)
		}
	}
	return nil
}

// progressUnits is the bounded set of units in a fixed order for the
// current-progress fetch of an edition.
var progressUnits = []domain.ProgressUnit{
	domain.UnitChapter, domain.UnitVolume, domain.UnitEpisode, domain.UnitPage, domain.UnitPercent,
}

// currentProgressQuery selects the latest live observation of one unit:
// the last non-retracted row with a position by (observed_at, recorded_at,
// id). A row is retracted only when another row supersedes it; the
// superseding correction or retraction row itself is live. Retraction
// markers (position NULL) never become current: a unit whose latest
// observation was retracted falls back to the next live observation or has
// no current progress. A past-dated observation never overtakes a newer one.
func (d *Database) currentProgressQuery(ctx context.Context, q queryer, editionID string, unit domain.ProgressUnit) (*ProgressObservation, error) {
	var (
		o          ProgressObservation
		observedAt string
		recordedAt string
		position   string
		total      sql.NullString
		label      sql.NullString
		reason     sql.NullString
		supersedes sql.NullString
		unitCol    string
		actor      string
	)
	err := q.QueryRowContext(ctx, `
		SELECT o.id, o.unit, o.position, o.total, o.label, o.reason, o.observed_at, o.recorded_at, o.actor, o.supersedes_id
		FROM progress_observations o
		WHERE o.edition_id = ? AND o.unit = ? AND o.position IS NOT NULL
		  AND NOT EXISTS (SELECT 1 FROM progress_observations s WHERE s.supersedes_id = o.id)
		ORDER BY o.observed_at DESC, o.recorded_at DESC, o.id DESC
		LIMIT 1`, editionID, string(unit)).
		Scan(&o.ID, &unitCol, &position, &total, &label, &reason, &observedAt, &recordedAt, &actor, &supersedes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: query current progress: %w", err)
	}
	var a ProgressActor
	if err := json.Unmarshal([]byte(actor), &a); err != nil {
		return nil, fmt.Errorf("store: parse progress actor: %w", err)
	}
	o.EditionID = editionID
	o.Unit = domain.ProgressUnit(unitCol)
	o.Position = &position
	if total.Valid {
		o.Total = &total.String
	}
	if label.Valid {
		o.Label = &label.String
	}
	if reason.Valid {
		o.Reason = &reason.String
	}
	if supersedes.Valid {
		o.SupersedesID = &supersedes.String
	}
	o.ObservedAt, err = parseInstant(observedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse progress observed_at: %w", err)
	}
	o.RecordedAt, err = parseInstant(recordedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse progress recorded_at: %w", err)
	}
	o.Actor = a
	return &o, nil
}

// currentProgressTx is the in-transaction variant of the unit current-state
// query.
func (d *Database) currentProgressTx(ctx context.Context, tx *sql.Tx, editionID string, unit domain.ProgressUnit) (*ProgressObservation, error) {
	return d.currentProgressQuery(ctx, tx, editionID, unit)
}

// currentProgress fetches the live current observation of every unit of an
// edition; units without one are absent.
func (d *Database) currentProgress(ctx context.Context, editionID string) ([]ProgressObservation, error) {
	out := []ProgressObservation{}
	for _, unit := range progressUnits {
		o, err := d.currentProgressQuery(ctx, d.db, editionID, unit)
		if err != nil {
			return nil, err
		}
		if o != nil {
			out = append(out, *o)
		}
	}
	return out, nil
}

// checkProgressSupersede verifies a correction target: an existing
// observation (or retraction marker) of the same edition and unit that has
// not been superseded yet.
func (d *Database) checkProgressSupersede(ctx context.Context, tx *sql.Tx, editionID, targetID string, unit domain.ProgressUnit) error {
	var id, rowUnit string
	err := tx.QueryRowContext(ctx,
		`SELECT id, unit FROM progress_observations WHERE id = ? AND edition_id = ?`,
		targetID, editionID).Scan(&id, &rowUnit)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrProgressNotFound, targetID)
	}
	if err != nil {
		return fmt.Errorf("store: read superseded progress: %w", err)
	}
	if rowUnit != string(unit) {
		return fmt.Errorf("%w: %s is a %s observation, not a %s one", ErrInvalidProgress, targetID, rowUnit, unit)
	}
	var superseding int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM progress_observations WHERE supersedes_id = ?`, targetID).Scan(&superseding); err != nil {
		return fmt.Errorf("store: check progress supersede: %w", err)
	}
	if superseding != 0 {
		return fmt.Errorf("%w: %s is already superseded", ErrSupersedeConflict, targetID)
	}
	return nil
}

// sameProgressAttr reports whether two nullable attributes hold the same
// canonical value for no-op comparison.
func sameProgressAttr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// RecordProgress records one personal progress observation for an edition
// and unit. The current position is the last non-superseded observation by
// (observed_at, recorded_at, id). A request for the same current
// position/total/label is a no-op without a revision bump; a supersedes_id
// correction targets an existing, not-yet-superseded observation of the
// same edition and unit and always applies. Lower positions are allowed for
// rereads. A known total is never carried forward from an older row.
// Past-dated observations are stored but do not become current over a newer
// one. All history rows are preserved, and the optional journal row commits
// in the same transaction. The edition revision guards against lost updates.
func (d *Database) RecordProgress(ctx context.Context, now time.Time, editionID string, expectedRevision int64, in ProgressInput, actor ProgressActor, journal *JournalInput) (Edition, error) {
	if err := ValidateProgressValues(in.Unit, in.Position, in.Total); err != nil {
		return Edition{}, fmt.Errorf("%w: %v", ErrInvalidProgress, err)
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
		return Edition{}, fmt.Errorf("store: encode progress actor: %w", err)
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
	current, err := d.currentProgressTx(ctx, tx, editionID, in.Unit)
	if err != nil {
		return Edition{}, err
	}
	noOp := in.SupersedesID == nil &&
		current != nil &&
		*current.Position == in.Position &&
		sameProgressAttr(current.Total, in.Total) &&
		sameProgressAttr(current.Label, in.Label)
	if !noOp {
		if in.SupersedesID != nil {
			if err := d.checkProgressSupersede(ctx, tx, editionID, *in.SupersedesID, in.Unit); err != nil {
				return Edition{}, err
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO progress_observations (id, edition_id, unit, position, total, label, reason, observed_at, recorded_at, actor, supersedes_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, editionID, string(in.Unit), in.Position, in.Total, in.Label, in.Reason, instantUTC(observedAt), instantUTC(now), string(actorJSON), in.SupersedesID); err != nil {
			return Edition{}, fmt.Errorf("store: insert progress observation: %w", err)
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
		return Edition{}, fmt.Errorf("store: commit progress: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}

// RetractProgress explicitly retracts one progress observation of an
// edition: a marker row for the same unit with position NULL, the required
// reason and the retracted row id commits in one transaction. The retracted
// row stays in history superseded, the unit's current position falls back
// to the next live observation or to absence, and the edition revision
// guards against lost updates. Retraction markers cannot themselves be
// retracted.
func (d *Database) RetractProgress(ctx context.Context, now time.Time, editionID string, expectedRevision int64, recordID, reason string, actor ProgressActor, journal *JournalInput) (Edition, error) {
	if strings.TrimSpace(reason) == "" {
		return Edition{}, fmt.Errorf("%w: retraction requires a reason", ErrInvalidProgress)
	}
	id, err := domain.NewUUID()
	if err != nil {
		return Edition{}, err
	}
	actorJSON, err := json.Marshal(actor)
	if err != nil {
		return Edition{}, fmt.Errorf("store: encode progress actor: %w", err)
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
		unit     string
		position sql.NullString
	)
	if err := tx.QueryRowContext(ctx,
		`SELECT unit, position FROM progress_observations WHERE id = ? AND edition_id = ?`,
		recordID, editionID).Scan(&unit, &position); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Edition{}, fmt.Errorf("%w: %s", ErrProgressNotFound, recordID)
		}
		return Edition{}, fmt.Errorf("store: read progress for retraction: %w", err)
	}
	if !position.Valid {
		return Edition{}, fmt.Errorf("%w: %s is a retraction and cannot be retracted", ErrInvalidProgress, recordID)
	}
	var superseding int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM progress_observations WHERE supersedes_id = ?`, recordID).Scan(&superseding); err != nil {
		return Edition{}, fmt.Errorf("store: check progress retraction: %w", err)
	}
	if superseding != 0 {
		return Edition{}, fmt.Errorf("%w: %s is already superseded", ErrSupersedeConflict, recordID)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO progress_observations (id, edition_id, unit, position, total, label, reason, observed_at, recorded_at, actor, supersedes_id)
		 VALUES (?, ?, ?, NULL, NULL, NULL, ?, ?, ?, ?, ?)`,
		id, editionID, unit, reason, instantUTC(now), instantUTC(now), string(actorJSON), recordID); err != nil {
		return Edition{}, fmt.Errorf("store: insert progress retraction: %w", err)
	}
	if err := d.bumpEditionRevision(tx, ctx, editionID, revision, now); err != nil {
		return Edition{}, err
	}
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("edition_id", editionID, revision+1)); err != nil {
		return Edition{}, err
	}
	if err := tx.Commit(); err != nil {
		return Edition{}, fmt.Errorf("store: commit progress retraction: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}
