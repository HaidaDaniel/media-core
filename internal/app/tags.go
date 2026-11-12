package app

import (
	"context"
	"errors"
	"strings"

	"github.com/HaidaDaniel/media-core/internal/store"
)

// TagRequest is one idempotent tag assignment change on a work or an
// edition: attach (Detach false) or detach (Detach true) one normalized
// tag name.
type TagRequest struct {
	MutationBase
	TargetID string
	Tag      string
	Detach   bool
}

var (
	errTagTargetBlank = errors.New("the target id is required")
	errTagNameBlank   = errors.New("the tag name is required")
)

func requireTagRequest(req TagRequest) error {
	if strings.TrimSpace(req.TargetID) == "" {
		return errTagTargetBlank
	}
	if strings.TrimSpace(req.Tag) == "" {
		return errTagNameBlank
	}
	return nil
}

// SetWorkTag attaches or detaches one tag on a work and returns the
// updated work view. A repeated attach or detach is a no-op without a
// revision bump; a detach keeps the assignment as audit history. Tags
// never change consumption, intention, publication or progress.
func (s *Service) SetWorkTag(ctx context.Context, req TagRequest) (WorkView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return WorkView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return WorkView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if err := requireTagRequest(req); err != nil {
		return WorkView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return WorkView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayWorkView(ctx, entry, req.MutationBase, hash)
	}
	w, err := s.db.SetWorkTag(ctx, s.now(), req.TargetID, *req.ExpectedRevision, req.Tag, req.Detach, store.TagActor{
		Principal: req.Actor.Principal,
		Origin:    string(req.Actor.Origin),
		Identity:  req.Actor.Identity,
	}, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "set_work_tag",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleWorkView(ctx, req.MutationBase, hash, err)
	}
	editions, err := s.db.ListEditions(ctx, w.ID)
	if err != nil {
		return WorkView{}, mapStoreError(err, req.MutationBase)
	}
	return workViewFrom(w, editions), nil
}

// SetEditionTag attaches or detaches one tag on an edition and returns the
// updated edition view with the same no-op, audit and revision rules as
// SetWorkTag.
func (s *Service) SetEditionTag(ctx context.Context, req TagRequest) (EditionView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return EditionView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return EditionView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if err := requireTagRequest(req); err != nil {
		return EditionView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return EditionView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayEditionView(ctx, entry, req.MutationBase, hash)
	}
	e, err := s.db.SetEditionTag(ctx, s.now(), req.TargetID, *req.ExpectedRevision, req.Tag, req.Detach, store.TagActor{
		Principal: req.Actor.Principal,
		Origin:    string(req.Actor.Origin),
		Identity:  req.Actor.Identity,
	}, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "set_edition_tag",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleEditionView(ctx, req.MutationBase, hash, err)
	}
	return editionViewFrom(e), nil
}
