package app

import (
	"context"
	"errors"

	"github.com/HaidaDaniel/media-core/internal/importer"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// ReleaseBindingView is the wire shape of one explicit, audited
// publication→edition binding.
type ReleaseBindingView struct {
	ID            string  `json:"id"`
	Namespace     string  `json:"namespace"`
	PublicationID string  `json:"publication_id"`
	EditionID     string  `json:"edition_id"`
	WorkID        string  `json:"work_id"`
	State         string  `json:"state"`
	CreatedBy     string  `json:"created_by"`
	CreatedAt     string  `json:"created_at"`
	ReplacesID    *string `json:"replaces_id,omitempty"`
}

// ReleaseProjectionView is one imported release made visible for an edition
// through an active binding. The release fields are the producer's, labeled
// by the exact publication they came from; counts are never summed.
type ReleaseProjectionView struct {
	EventID       string  `json:"event_id"`
	EventType     string  `json:"event_type"`
	Namespace     string  `json:"namespace"`
	PublicationID *string `json:"publication_id,omitempty"`
	EditionID     string  `json:"edition_id"`
	WorkID        string  `json:"work_id"`
	ReleaseID     *string `json:"release_id,omitempty"`
	ReleaseNumber *string `json:"release_number,omitempty"`
	ReleaseTitle  *string `json:"release_title,omitempty"`
	ReleaseURL    *string `json:"release_url,omitempty"`
	CurrentCount  *int    `json:"current_count,omitempty"`
	CountDelta    *int    `json:"count_delta,omitempty"`
	SourceTime    *string `json:"source_time,omitempty"`
	DetectionTime *string `json:"detection_time,omitempty"`
	CreatedAt     string  `json:"created_at"`
}

// BindReleaseRequest explicitly maps one confirmed producer publication to
// one edition. The namespace defaults to the stable Release Watcher name.
type BindReleaseRequest struct {
	MutationBase
	Namespace     string
	PublicationID string
	EditionID     string
}

// releaseStoreError maps a release store failure to the application error:
// an invalid binding input or a malformed cursor is a caller input error, a
// map collision is a conflict, a missing binding is a not-found, and the
// remaining store failures map through the shared table.
func releaseStoreError(err error, base MutationBase) *AppError {
	switch {
	case errors.Is(err, store.ErrInvalidEvent):
		return &AppError{Code: ErrInvalidArgument, Message: err.Error(), RequestID: base.RequestID, Actor: base.Actor.Principal}
	case errors.Is(err, store.ErrBindingConflict):
		return &AppError{Code: ErrConflict, Message: err.Error(), RequestID: base.RequestID, Actor: base.Actor.Principal}
	case errors.Is(err, store.ErrBindingNotFound):
		return &AppError{Code: ErrNotFound, Message: err.Error(), RequestID: base.RequestID, Actor: base.Actor.Principal}
	}
	return mapStoreError(err, base)
}

func defaultNamespace(namespace string) string {
	if namespace == "" {
		return importer.DefaultNamespace
	}
	return namespace
}

func bindingView(ctx context.Context, s *Service, b store.ReleaseBinding) (ReleaseBindingView, error) {
	v := ReleaseBindingView{
		ID: b.ID, Namespace: b.Namespace, PublicationID: b.PublicationID, EditionID: b.EditionID,
		State: b.State, CreatedBy: b.CreatedBy, CreatedAt: b.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		ReplacesID: b.ReplacesID,
	}
	if ed, err := s.db.GetEdition(ctx, b.EditionID); err == nil {
		v.WorkID = ed.WorkID
	}
	return v, nil
}

// BindRelease explicitly maps a confirmed publication to an edition. It is
// idempotent: the same publication bound to the same edition returns the
// existing active binding unchanged. A publication already bound to a
// different edition is an explicit map conflict — never a silent re-point;
// use RebindRelease to correct a binding. No edition is inferred from a
// title or URL.
func (s *Service) BindRelease(ctx context.Context, req BindReleaseRequest) (ReleaseBindingView, error) {
	base := req.MutationBase
	b, err := s.db.BindReleaseBinding(ctx,
		defaultNamespace(req.Namespace), req.PublicationID, req.EditionID, base.Actor.Principal, s.now())
	if err != nil {
		return ReleaseBindingView{}, releaseStoreError(err, base)
	}
	return bindingView(ctx, s, b)
}

// RebindRelease explicitly corrects a publication's binding: in one step it
// revokes the current active binding and creates its replacement, so the old
// binding and the raw event history stay intact and only the replacement
// projects. With no active binding it simply creates one.
func (s *Service) RebindRelease(ctx context.Context, req BindReleaseRequest) (ReleaseBindingView, error) {
	base := req.MutationBase
	b, err := s.db.RebindReleaseBinding(ctx,
		defaultNamespace(req.Namespace), req.PublicationID, req.EditionID, base.Actor.Principal, s.now())
	if err != nil {
		return ReleaseBindingView{}, releaseStoreError(err, base)
	}
	return bindingView(ctx, s, b)
}

// RevokeReleaseRequest revokes one publication's active binding, un-projecting
// its events while keeping the binding row and the raw event history.
type RevokeReleaseRequest struct {
	MutationBase
	Namespace     string
	PublicationID string
}

// RevokeRelease revokes a publication's active binding. Revoking a
// publication with no active binding is a distinguishable not-found.
func (s *Service) RevokeRelease(ctx context.Context, req RevokeReleaseRequest) error {
	base := req.MutationBase
	if err := s.db.RevokeReleaseBinding(ctx,
		defaultNamespace(req.Namespace), req.PublicationID, base.Actor.Principal, s.now()); err != nil {
		return releaseStoreError(err, base)
	}
	return nil
}

// GetReleaseBinding returns the active binding of one publication, or nil
// when there is none.
func (s *Service) GetReleaseBinding(ctx context.Context, namespace, publicationID string) (*ReleaseBindingView, error) {
	b, err := s.db.GetReleaseBinding(ctx, defaultNamespace(namespace), publicationID)
	if err != nil {
		return nil, releaseStoreError(err, MutationBase{})
	}
	if b == nil {
		return nil, nil
	}
	v, err := bindingView(ctx, s, *b)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// ListReleaseBindings returns the full audited binding history (active and
// revoked) of one publication, oldest first.
func (s *Service) ListReleaseBindings(ctx context.Context, namespace, publicationID string) ([]ReleaseBindingView, error) {
	rows, err := s.db.ListReleaseBindings(ctx, defaultNamespace(namespace), publicationID)
	if err != nil {
		return nil, releaseStoreError(err, MutationBase{})
	}
	out := make([]ReleaseBindingView, 0, len(rows))
	for _, b := range rows {
		v, err := bindingView(ctx, s, b)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// ReleaseQuery is one bounded release-history listing for an edition.
type ReleaseQuery struct {
	EditionID string
	Limit     int
	Cursor    *string
}

// ListReleases projects the release history of one edition: the imported
// events made visible by every active binding of the edition, oldest first,
// keyset-paginated. Reads never mutate; personal progress and intention are
// never touched.
func (s *Service) ListReleases(ctx context.Context, q ReleaseQuery) (ListPayload, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	rows, total, next, err := s.db.ListReleaseProjections(ctx, q.EditionID, limit, q.Cursor)
	if err != nil {
		return ListPayload{}, releaseStoreError(err, MutationBase{})
	}
	items := make([]any, 0, len(rows))
	for _, p := range rows {
		items = append(items, ReleaseProjectionView{
			EventID: p.EventID, EventType: p.EventType, Namespace: p.Namespace,
			PublicationID: p.PublicationID, EditionID: p.EditionID, WorkID: p.WorkID,
			ReleaseID: p.ReleaseID, ReleaseNumber: p.ReleaseNumber, ReleaseTitle: p.ReleaseTitle,
			ReleaseURL: p.ReleaseURL, CurrentCount: p.CurrentCount, CountDelta: p.CountDelta,
			SourceTime: p.SourceTime, DetectionTime: p.DetectionTime,
			CreatedAt: p.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		})
	}
	return ListPayload{Items: items, NextCursor: next, Total: total}, nil
}
