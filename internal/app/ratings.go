package app

import (
	"context"
	"strings"
	"time"

	"github.com/HaidaDaniel/media-core/internal/store"
)

// SetRatingRequest is one personal rating update for an edition: a new
// integral 1-10 value (Value set), an explicit clear (Value nil), an
// optional comment, an optional past-dated observation instant and an
// optional correction target.
type SetRatingRequest struct {
	MutationBase
	EditionID    string
	Value        *int
	Comment      *string
	ObservedAt   *time.Time
	SupersedesID *string
}

// SetRating records one rating observation and returns the updated edition
// view. The current rating is the last non-superseded observation by
// (observed_at, recorded_at, id); a request for the same current value and
// comment is a no-op without a revision bump; a nil value is the explicit
// clear, never zero, and renders as the absence of a rating; a supersedes_id
// correction retracts the named row, which stays in history. Rating changes
// never change consumption, intention, publication or progress.
func (s *Service) SetRating(ctx context.Context, req SetRatingRequest) (EditionView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return EditionView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return EditionView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if strings.TrimSpace(req.EditionID) == "" {
		return EditionView{}, invalidArgument("edition id is required", req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return EditionView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayEditionView(ctx, entry, req.MutationBase, hash)
	}
	in := store.RatingInput{
		Value:        req.Value,
		Comment:      req.Comment,
		SupersedesID: req.SupersedesID,
	}
	if req.ObservedAt != nil {
		in.ObservedAt = *req.ObservedAt
	}
	e, err := s.db.SetRating(ctx, s.now(), req.EditionID, *req.ExpectedRevision, in, store.RatingActor{
		Principal: req.Actor.Principal,
		Origin:    string(req.Actor.Origin),
		Identity:  req.Actor.Identity,
	}, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "set_rating",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleEditionView(ctx, req.MutationBase, hash, err)
	}
	return editionViewFrom(e), nil
}
