package app

import (
	"context"
	"strings"
	"time"

	"github.com/HaidaDaniel/media-core/internal/store"
)

// SetConsumptionRequest is one personal consumption update for an edition:
// a new value (or an explicit retraction to unknown), an optional reason,
// an optional past-dated observation instant and an optional correction
// target.
type SetConsumptionRequest struct {
	MutationBase
	EditionID    string
	Value        string
	Reason       *string
	ObservedAt   *time.Time
	SupersedesID *string
}

// SetConsumption records one consumption observation and returns the
// updated edition view. The current state is the last non-superseded
// observation by (observed_at, recorded_at, id); a same-current-value
// request is a no-op without a revision bump; a supersedes_id correction
// retracts the named row, which stays in history; unknown is rendered as
// the absence of a consumption state. Status never changes publication,
// intention or progress.
func (s *Service) SetConsumption(ctx context.Context, req SetConsumptionRequest) (EditionView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return EditionView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return EditionView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if strings.TrimSpace(req.EditionID) == "" {
		return EditionView{}, invalidArgument("edition id is required", req.MutationBase)
	}
	if !store.ValidConsumptionValue(req.Value) {
		return EditionView{}, invalidArgument("unknown consumption value "+req.Value, req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return EditionView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayEditionView(ctx, entry, req.MutationBase, hash)
	}
	in := store.ConsumptionInput{
		Value:        req.Value,
		Reason:       req.Reason,
		SupersedesID: req.SupersedesID,
	}
	if req.ObservedAt != nil {
		in.ObservedAt = *req.ObservedAt
	}
	e, err := s.db.SetConsumption(ctx, s.now(), req.EditionID, *req.ExpectedRevision, in, store.ConsumptionActor{
		Principal: req.Actor.Principal,
		Origin:    string(req.Actor.Origin),
		Identity:  req.Actor.Identity,
	}, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "set_consumption",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleEditionView(ctx, req.MutationBase, hash, err)
	}
	return editionViewFrom(e), nil
}
