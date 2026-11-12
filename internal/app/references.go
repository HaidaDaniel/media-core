package app

import (
	"context"
	"strings"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// AddReferenceRequest attaches one sourced external reference to an
// edition: a provider namespace plus an external ID or an absolute http(s)
// URL, a role, the explicit authoritative assertion, an optional language
// and the explicit provenance claim.
type AddReferenceRequest struct {
	MutationBase
	EditionID     string
	Provider      string
	ExternalID    *string
	URL           *string
	Role          domain.ReferenceRole
	Authoritative bool
	Language      *string
	Provenance    string
}

// RetireReferenceRequest retires one reference of an edition with
// correction provenance; the rows stay in history, never erased.
type RetireReferenceRequest struct {
	MutationBase
	EditionID   string
	ReferenceID string
	Reason      *string
}

// AddReference attaches one sourced external reference and returns the
// updated edition view. The 0012 identity reservation decides conflicts: an
// active identity on another edition is an identity conflict, never a
// silent reassignment; the same edition plus the same identity is one
// reference, so a new URL on the same edition adds a reference and never
// creates another edition. The authoritative flag is an explicit
// assertion, never inferred from the URL or a metadata database, and links
// are never fetched.
func (s *Service) AddReference(ctx context.Context, req AddReferenceRequest) (EditionView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return EditionView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return EditionView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if strings.TrimSpace(req.EditionID) == "" {
		return EditionView{}, invalidArgument("edition id is required", req.MutationBase)
	}
	if !req.Role.Valid() {
		return EditionView{}, invalidArgument("unknown reference role "+string(req.Role), req.MutationBase)
	}
	if strings.TrimSpace(req.Provider) == "" {
		return EditionView{}, invalidArgument("provider is required", req.MutationBase)
	}
	if strings.TrimSpace(req.Provenance) == "" {
		return EditionView{}, invalidArgument("provenance is required", req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return EditionView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayEditionView(ctx, entry, req.MutationBase, hash)
	}
	e, err := s.db.AddReference(ctx, s.now(), req.EditionID, *req.ExpectedRevision, store.ReferenceRequest{
		Provider:      req.Provider,
		ExternalID:    req.ExternalID,
		URL:           req.URL,
		Role:          req.Role,
		Authoritative: req.Authoritative,
		Language:      req.Language,
		Provenance:    req.Provenance,
	}, store.ReferenceActor{
		Principal: req.Actor.Principal,
		Origin:    string(req.Actor.Origin),
		Identity:  req.Actor.Identity,
	}, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "add_reference",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleEditionView(ctx, req.MutationBase, hash, err)
	}
	return editionViewFrom(e), nil
}

// RetireReference retires one reference of an edition and returns the
// updated edition view. The reference row and its identity reservation keep
// their rows with the retirement instant and the optional correction
// reason; retiring an already-retired reference is a no-op.
func (s *Service) RetireReference(ctx context.Context, req RetireReferenceRequest) (EditionView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return EditionView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return EditionView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if strings.TrimSpace(req.EditionID) == "" {
		return EditionView{}, invalidArgument("edition id is required", req.MutationBase)
	}
	if strings.TrimSpace(req.ReferenceID) == "" {
		return EditionView{}, invalidArgument("reference id is required", req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return EditionView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayEditionView(ctx, entry, req.MutationBase, hash)
	}
	e, err := s.db.RetireReference(ctx, s.now(), req.EditionID, *req.ExpectedRevision, req.ReferenceID, req.Reason, store.ReferenceActor{
		Principal: req.Actor.Principal,
		Origin:    string(req.Actor.Origin),
		Identity:  req.Actor.Identity,
	}, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "retire_reference",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleEditionView(ctx, req.MutationBase, hash, err)
	}
	return editionViewFrom(e), nil
}
