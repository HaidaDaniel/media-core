package app

import (
	"context"
	"strings"
	"time"

	"github.com/HaidaDaniel/media-core/internal/store"
)

// SetIntentionRequest is one personal intention update for an edition: a
// new value (or an explicit retraction to none), an optional reason, an
// optional past-dated observation instant and an optional correction target.
type SetIntentionRequest struct {
	MutationBase
	EditionID    string
	Value        string
	Reason       *string
	ObservedAt   *time.Time
	SupersedesID *string
}

// SetIntention records one intention observation and returns the updated
// edition view. The current intention is the last non-superseded
// observation by (observed_at, recorded_at, id); a same-current-value
// request is a no-op without a revision bump; a supersedes_id correction
// supersedes the named row, which stays in history; none is rendered as the
// absence of a recorded intention. Intention never changes consumption,
// publication or progress, and consumption never changes intention.
func (s *Service) SetIntention(ctx context.Context, req SetIntentionRequest) (EditionView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return EditionView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return EditionView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if strings.TrimSpace(req.EditionID) == "" {
		return EditionView{}, invalidArgument("edition id is required", req.MutationBase)
	}
	if !store.ValidIntentionValue(req.Value) {
		return EditionView{}, invalidArgument("unknown intention value "+req.Value, req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return EditionView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayEditionView(ctx, entry, req.MutationBase, hash)
	}
	in := store.IntentionInput{
		Value:        req.Value,
		Reason:       req.Reason,
		SupersedesID: req.SupersedesID,
	}
	if req.ObservedAt != nil {
		in.ObservedAt = *req.ObservedAt
	}
	e, err := s.db.SetIntention(ctx, s.now(), req.EditionID, *req.ExpectedRevision, in, store.IntentionActor{
		Principal: req.Actor.Principal,
		Origin:    string(req.Actor.Origin),
		Identity:  req.Actor.Identity,
	}, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "set_intention",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleEditionView(ctx, req.MutationBase, hash, err)
	}
	return editionViewFrom(e), nil
}
