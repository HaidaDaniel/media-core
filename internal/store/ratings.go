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

// Rating failures. Application services map these to the stable wire error
// codes; the store stays transport-agnostic.
var (
	ErrInvalidRating  = errors.New("store: invalid rating")
	ErrRatingNotFound = errors.New("store: rating observation not found")
)

// RatingActor is the acting identity recorded on a rating observation. It
// is stored as canonical JSON in the actor column.
type RatingActor struct {
	Principal string `json:"principal"`
	Origin    string `json:"origin,omitempty"`
	Identity  string `json:"identity,omitempty"`
}

// RatingInput is one requested rating observation: an integral 1-10 value
// or a nil value for the explicit clear, an optional comment, an optional
// past-dated observation instant and an optional correction target.
type RatingInput struct {
	Value        *int
	Comment      *string
	ObservedAt   time.Time
	SupersedesID *string
}

// RatingObservation is one stored rating history row. A nil Value is the
// explicit clear, never zero.
type RatingObservation struct {
	ID           string
	EditionID    string
	Value        *int
	Comment      *string
	ObservedAt   time.Time
	RecordedAt   time.Time
	Actor        RatingActor
	SupersedesID *string
}

// validateRatingInput checks the rating value and comment before any
// journal key is reserved: the value must be absent (clear) or integral
// 1-10, and a supplied comment must be non-blank.
func validateRatingInput(in RatingInput) error {
	if in.Value != nil && (*in.Value < 1 || *in.Value > 10) {
		return fmt.Errorf("%w: value must be between 1 and 10", ErrInvalidRating)
	}
	if in.Comment != nil && strings.TrimSpace(*in.Comment) == "" {
		return fmt.Errorf("%w: comment is empty", ErrInvalidRating)
	}
	return nil
}

// currentRating selects the last non-superseded rating observation by
// (observed_at, recorded_at, id). A row is retracted only when another row
// supersedes it; the superseding correction row itself is live. A
// past-dated observation never overtakes a newer one. An edition without
// observations has no current rating (absence, never an invented value).
func (d *Database) currentRating(ctx context.Context, editionID string) (*RatingObservation, error) {
	return d.currentRatingQuery(ctx, d.db, editionID)
}

// currentRatingTx is the in-transaction variant of currentRating.
func (d *Database) currentRatingTx(ctx context.Context, tx *sql.Tx, editionID string) (*RatingObservation, error) {
	return d.currentRatingQuery(ctx, tx, editionID)
}

func (d *Database) currentRatingQuery(ctx context.Context, q queryer, editionID string) (*RatingObservation, error) {
	var (
		o          RatingObservation
		observedAt string
		recordedAt string
		actor      string
		value      sql.NullInt64
		comment    sql.NullString
		supersedes sql.NullString
	)
	err := q.QueryRowContext(ctx, `
		SELECT o.id, o.value, o.observed_at, o.recorded_at, o.actor, o.comment, o.supersedes_id
		FROM rating_observations o
		WHERE o.edition_id = ?
		  AND NOT EXISTS (SELECT 1 FROM rating_observations s WHERE s.supersedes_id = o.id)
		ORDER BY o.observed_at DESC, o.recorded_at DESC, o.id DESC
		LIMIT 1`, editionID).
		Scan(&o.ID, &value, &observedAt, &recordedAt, &actor, &comment, &supersedes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: query current rating: %w", err)
	}
	var a RatingActor
	if err := json.Unmarshal([]byte(actor), &a); err != nil {
		return nil, fmt.Errorf("store: parse rating actor: %w", err)
	}
	o.EditionID = editionID
	o.Actor = a
	if value.Valid {
		v := int(value.Int64)
		o.Value = &v
	}
	if comment.Valid {
		o.Comment = &comment.String
	}
	if supersedes.Valid {
		o.SupersedesID = &supersedes.String
	}
	o.ObservedAt, err = parseInstant(observedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse rating observed_at: %w", err)
	}
	o.RecordedAt, err = parseInstant(recordedAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse rating recorded_at: %w", err)
	}
	return &o, nil
}

// checkRatingSupersede verifies a correction target: an existing rating
// observation of the same edition that has not been superseded yet.
func (d *Database) checkRatingSupersede(ctx context.Context, tx *sql.Tx, editionID, targetID string) error {
	var id string
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM rating_observations WHERE id = ? AND edition_id = ?`,
		targetID, editionID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrRatingNotFound, targetID)
	}
	if err != nil {
		return fmt.Errorf("store: read superseded rating: %w", err)
	}
	var superseding int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM rating_observations WHERE supersedes_id = ?`, targetID).Scan(&superseding); err != nil {
		return fmt.Errorf("store: check rating supersede: %w", err)
	}
	if superseding != 0 {
		return fmt.Errorf("%w: %s is already superseded", ErrSupersedeConflict, targetID)
	}
	return nil
}

// sameRating reports whether a request re-asserts the exact current rating:
// the same value and the same comment. A nil value is the explicit clear.
func sameRating(current *RatingObservation, in RatingInput) bool {
	if current == nil {
		return in.Value == nil && in.Comment == nil
	}
	if (in.Value == nil) != (current.Value == nil) {
		return false
	}
	if in.Value != nil && *in.Value != *current.Value {
		return false
	}
	return sameComment(in.Comment, current.Comment)
}

func sameComment(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// SetRating records one personal rating observation for an edition. The
// current rating is the last non-superseded observation by (observed_at,
// recorded_at, id); a request for the same current value and comment is a
// no-op without a revision bump; a nil value is the explicit clear, never
// zero; a supersedes_id correction targets an existing, not-yet-superseded
// observation of the same edition and the corrected row stays in history;
// past-dated observations are stored but do not become current over a
// newer one. Rating changes never touch consumption, intention,
// publication or progress, and the optional journal row commits in the same
// transaction. The edition revision guards against lost updates.
func (d *Database) SetRating(ctx context.Context, now time.Time, editionID string, expectedRevision int64, in RatingInput, actor RatingActor, journal *JournalInput) (Edition, error) {
	if err := validateRatingInput(in); err != nil {
		return Edition{}, err
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
		return Edition{}, fmt.Errorf("store: encode rating actor: %w", err)
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
	current, err := d.currentRatingTx(ctx, tx, editionID)
	if err != nil {
		return Edition{}, err
	}
	noOp := in.SupersedesID == nil && sameRating(current, in)
	if !noOp {
		if in.SupersedesID != nil {
			if err := d.checkRatingSupersede(ctx, tx, editionID, *in.SupersedesID); err != nil {
				return Edition{}, err
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO rating_observations (id, edition_id, value, comment, observed_at, recorded_at, actor, supersedes_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, editionID, in.Value, in.Comment, instantUTC(observedAt), instantUTC(now), string(actorJSON), in.SupersedesID); err != nil {
			return Edition{}, fmt.Errorf("store: insert rating observation: %w", err)
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
		return Edition{}, fmt.Errorf("store: commit rating: %w", err)
	}
	return d.GetEdition(ctx, editionID)
}
