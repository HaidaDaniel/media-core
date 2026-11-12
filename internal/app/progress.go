package app

import (
	"context"
	"strings"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// RetractProgressRequest is one explicit retraction of a progress
// observation: the edition, the retracted record and the required reason.
type RetractProgressRequest struct {
	MutationBase
	EditionID string
	RecordID  string
	Reason    string
}

// RecordProgress records one progress observation for an edition and unit
// and returns the updated edition view. The current position is the last
// non-superseded observation by (observed_at, recorded_at, id); a same-
// current-value request is a no-op without a revision bump; a supersedes_id
// correction retracts the named row of the same unit, which stays in
// history; lower positions are allowed for rereads; an unknown total is
// never carried forward. Progress never changes consumption, intention or
// publication.
func (s *Service) RecordProgress(ctx context.Context, req RecordProgressRequest) (EditionView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return EditionView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return EditionView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if strings.TrimSpace(req.EditionID) == "" {
		return EditionView{}, invalidArgument("edition id is required", req.MutationBase)
	}
	unit := req.Unit
	if !unit.Valid() {
		return EditionView{}, invalidArgument("unknown progress unit "+string(unit), req.MutationBase)
	}
	if err := store.ValidateProgressValues(unit, req.Position, req.Total); err != nil {
		return EditionView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return EditionView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayEditionView(ctx, entry, req.MutationBase, hash)
	}
	in := store.ProgressInput{
		Unit:         unit,
		Position:     req.Position,
		Total:        req.Total,
		Label:        req.Label,
		Reason:       req.Reason,
		SupersedesID: req.SupersedesID,
	}
	if !req.ObservedAt.IsZero() {
		in.ObservedAt = req.ObservedAt
	}
	e, err := s.db.RecordProgress(ctx, s.now(), req.EditionID, *req.ExpectedRevision, in, store.ProgressActor{
		Principal: req.Actor.Principal,
		Origin:    string(req.Actor.Origin),
		Identity:  req.Actor.Identity,
	}, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "record_progress",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
		Origin:      string(req.Actor.Origin),
		Identity:    req.Actor.Identity,
	})
	if err != nil {
		return s.settleEditionView(ctx, req.MutationBase, hash, err)
	}
	return editionViewFrom(e), nil
}

// RetractProgress explicitly retracts one progress observation and returns
// the updated edition view. The retracted row stays in history superseded,
// the unit's current position falls back to the next live observation or
// to absence, and the edition revision guards against lost updates.
func (s *Service) RetractProgress(ctx context.Context, req RetractProgressRequest) (EditionView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return EditionView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return EditionView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if strings.TrimSpace(req.EditionID) == "" {
		return EditionView{}, invalidArgument("edition id is required", req.MutationBase)
	}
	if strings.TrimSpace(req.RecordID) == "" {
		return EditionView{}, invalidArgument("record id is required", req.MutationBase)
	}
	if strings.TrimSpace(req.Reason) == "" {
		return EditionView{}, invalidArgument("a retraction requires a reason", req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return EditionView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayEditionView(ctx, entry, req.MutationBase, hash)
	}
	e, err := s.db.RetractProgress(ctx, s.now(), req.EditionID, *req.ExpectedRevision, req.RecordID, req.Reason, store.ProgressActor{
		Principal: req.Actor.Principal,
		Origin:    string(req.Actor.Origin),
		Identity:  req.Actor.Identity,
	}, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "retract_progress",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
		Origin:      string(req.Actor.Origin),
		Identity:    req.Actor.Identity,
	})
	if err != nil {
		return s.settleEditionView(ctx, req.MutationBase, hash, err)
	}
	return editionViewFrom(e), nil
}

// progressViewFrom maps the live current progress observations of an
// edition onto the frozen per-unit view shape; a unit without a live
// observation stays null, never zero.
func progressViewFrom(rows []store.ProgressObservation) ProgressByUnit {
	view := ProgressByUnit{}
	for _, r := range rows {
		rec := &ProgressRecord{
			ID:           r.ID,
			Unit:         r.Unit,
			Position:     *r.Position,
			Total:        r.Total,
			Label:        r.Label,
			ObservedAt:   r.ObservedAt,
			RecordedAt:   r.RecordedAt,
			Actor:        Actor{Principal: r.Actor.Principal, Origin: domain.ActorOrigin(r.Actor.Origin), Identity: r.Actor.Identity},
			SupersedesID: r.SupersedesID,
		}
		switch r.Unit {
		case domain.UnitChapter:
			view.Chapter = rec
		case domain.UnitVolume:
			view.Volume = rec
		case domain.UnitEpisode:
			view.Episode = rec
		case domain.UnitPage:
			view.Page = rec
		case domain.UnitPercent:
			view.Percent = rec
		}
	}
	return view
}
