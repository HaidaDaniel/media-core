package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/importer"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// isAppCode reports whether err is a typed application error carrying code.
func isAppCode(err error, want ErrorCode) bool {
	var ae *AppError
	if !errors.As(err, &ae) {
		return false
	}
	return ae.Code == want
}

// mustInsertReleaseEvent stages one Release Watcher event directly through
// the store so the binding and projection use cases can be exercised without
// the import envelope path.
func mustInsertReleaseEvent(t *testing.T, s *Service, eventID, eventType, pub, releaseNum string, at time.Time) {
	t.Helper()
	pubPtr := &pub
	numPtr := &releaseNum
	in := store.EventInput{
		EventID: eventID, SchemaVersion: "1.0", EventType: eventType,
		RawPayload: `{"event_id":"` + eventID + `"}`, Canonical: `{"event_id":"` + eventID + `"}`,
		Namespace: importer.DefaultNamespace, PublicationID: pubPtr, ReleaseNumber: numPtr, CreatedAt: at,
	}
	if _, err := s.db.InsertEvent(context.Background(), in); err != nil {
		t.Fatalf("InsertEvent(%s): %v", eventID, err)
	}
}

// releaseTestApp builds a service plus a work and two editions, so the
// binding use cases can be exercised end to end at the application layer.
func releaseTestApp(t *testing.T) (*Service, string, string, string) {
	t.Helper()
	s := newTestService(t)
	ctx := context.Background()
	now := testNow()
	w, err := s.db.CreateWork(ctx, now, []store.TitleInput{{Text: "Release Story", Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	e1, err := s.db.AddEdition(ctx, now, w.ID, w.Revision, store.EditionInput{Kind: domain.KindManga, Titles: []store.TitleInput{{Text: "Manga", Kind: domain.TitleOriginal, Provenance: "manual"}}})
	if err != nil {
		t.Fatalf("AddEdition e1: %v", err)
	}
	w2, err := s.db.GetWork(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	e2, err := s.db.AddEdition(ctx, now, w.ID, w2.Revision, store.EditionInput{Kind: domain.KindWebNovel, Titles: []store.TitleInput{{Text: "Web", Kind: domain.TitleOriginal, Provenance: "manual"}}})
	if err != nil {
		t.Fatalf("AddEdition e2: %v", err)
	}
	return s, w.ID, e1.ID, e2.ID
}

func bindReq(requestID, pub, edition string) BindReleaseRequest {
	return BindReleaseRequest{
		MutationBase:  MutationBase{RequestID: requestID, Actor: Actor{Principal: "local", Origin: domain.OriginUser}},
		Namespace:     importer.DefaultNamespace,
		PublicationID: pub,
		EditionID:     edition,
	}
}

// TestReleaseBindIdempotent proves the service idempotency: binding the same
// confirmed publication to the same edition returns the same active binding,
// and the view carries the target work id.
func TestReleaseBindIdempotent(t *testing.T) {
	s, _, e1, _ := releaseTestApp(t)
	ctx := context.Background()

	first, err := s.BindRelease(ctx, bindReq("req-1", "pub_1", e1))
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	if first.EditionID != e1 || first.State != store.BindingStateActive || first.WorkID == "" {
		t.Fatalf("first = %+v, want active for e1 with a work id", first)
	}
	replay, err := s.BindRelease(ctx, bindReq("req-1", "pub_1", e1))
	if err != nil {
		t.Fatalf("rebind no-op: %v", err)
	}
	if replay.ID != first.ID {
		t.Fatalf("replay id = %s, want the same active binding %s", replay.ID, first.ID)
	}
}

// TestReleaseBindConflict proves a publication already bound to a different
// edition is an explicit map conflict (a conflict-class error), never a
// silent re-point, and the original binding is left intact.
func TestReleaseBindConflict(t *testing.T) {
	s, _, e1, e2 := releaseTestApp(t)
	ctx := context.Background()

	if _, err := s.BindRelease(ctx, bindReq("req-1", "pub_c", e1)); err != nil {
		t.Fatalf("bind e1: %v", err)
	}
	_, err := s.BindRelease(ctx, bindReq("req-2", "pub_c", e2))
	if !isAppCode(err, ErrConflict) {
		t.Fatalf("bind to a different edition: err = %v, want a conflict", err)
	}
	active, err := s.GetReleaseBinding(ctx, importer.DefaultNamespace, "pub_c")
	if err != nil {
		t.Fatalf("GetReleaseBinding: %v", err)
	}
	if active == nil || active.EditionID != e1 {
		t.Fatalf("active binding = %+v, want the original e1 binding untouched", active)
	}
}

// TestReleaseRebind proves a correction: the rebind returns the replacement
// (linked to the prior binding) on the new edition, the active binding moves,
// and the audited history keeps the revoked predecessor.
func TestReleaseRebind(t *testing.T) {
	s, _, e1, e2 := releaseTestApp(t)
	ctx := context.Background()

	old, err := s.BindRelease(ctx, bindReq("req-1", "pub_rb", e1))
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	rep, err := s.RebindRelease(ctx, bindReq("req-2", "pub_rb", e2))
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if rep.EditionID != e2 || rep.ReplacesID == nil || *rep.ReplacesID != old.ID {
		t.Fatalf("replacement = %+v, want e2 replacing %s", rep, old.ID)
	}
	history, err := s.ListReleaseBindings(ctx, importer.DefaultNamespace, "pub_rb")
	if err != nil {
		t.Fatalf("ListReleaseBindings: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history = %d rows, want the revoked predecessor and the replacement", len(history))
	}
	if history[0].State != store.BindingStateRevoked {
		t.Fatalf("predecessor state = %q, want revoked", history[0].State)
	}
}

// TestReleaseRevoke proves revoking un-projects the publication at the
// service layer and that a second revoke is a distinguishable not-found.
func TestReleaseRevoke(t *testing.T) {
	s, _, e1, _ := releaseTestApp(t)
	ctx := context.Background()

	if _, err := s.BindRelease(ctx, bindReq("req-1", "pub_rv", e1)); err != nil {
		t.Fatalf("bind: %v", err)
	}
	req := RevokeReleaseRequest{
		MutationBase:  MutationBase{RequestID: "req-2", Actor: Actor{Principal: "local", Origin: domain.OriginUser}},
		Namespace:     importer.DefaultNamespace,
		PublicationID: "pub_rv",
	}
	if err := s.RevokeRelease(ctx, req); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if active, err := s.GetReleaseBinding(ctx, importer.DefaultNamespace, "pub_rv"); err != nil || active != nil {
		t.Fatalf("after revoke: active=%v err=%v, want none", active, err)
	}
	if err := s.RevokeRelease(ctx, req); !isAppCode(err, ErrNotFound) {
		t.Fatalf("second revoke: err = %v, want not-found", err)
	}
}

// TestReleaseListProjections proves the release-history listing: bound events
// project for the edition, are ordered and paginated, and an unbound
// publication's events do not appear.
func TestReleaseListProjections(t *testing.T) {
	s, _, e1, _ := releaseTestApp(t)
	ctx := context.Background()
	now := testNow()
	mustInsertReleaseEvent(t, s, "evt_p1", "release.added", "pub_list", "1", now)
	mustInsertReleaseEvent(t, s, "evt_p2", "release.added", "pub_list", "2", now.Add(time.Second))
	// An event for a publication that is never bound must not project.
	mustInsertReleaseEvent(t, s, "evt_p3", "release.added", "pub_other", "1", now.Add(2*time.Second))

	if _, err := s.BindRelease(ctx, bindReq("req-1", "pub_list", e1)); err != nil {
		t.Fatalf("bind: %v", err)
	}
	page, err := s.ListReleases(ctx, ReleaseQuery{EditionID: e1, Limit: 1})
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatalf("page = total %d, %d items, next %v; want 2 total, 1 item, a cursor", page.Total, len(page.Items), page.NextCursor)
	}
	first := page.Items[0].(ReleaseProjectionView)
	if first.EventID != "evt_p1" || first.WorkID == "" || first.EditionID != e1 {
		t.Fatalf("first projection = %+v, want evt_p1 on e1 with a work id", first)
	}
	page2, err := s.ListReleases(ctx, ReleaseQuery{EditionID: e1, Limit: 1, Cursor: page.NextCursor})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page2.Items) != 1 || page2.NextCursor != nil {
		t.Fatalf("page 2 = %+v, want the single row after the cursor and no next", page2)
	}
}

// TestReleaseBindingNotFound proves reading or listing a publication with no
// binding reports the explicit unknown state (nil / empty), never an error
// and never a guess.
func TestReleaseBindingNotFound(t *testing.T) {
	s, _, _, _ := releaseTestApp(t)
	ctx := context.Background()
	if active, err := s.GetReleaseBinding(ctx, importer.DefaultNamespace, "pub_none"); err != nil || active != nil {
		t.Fatalf("GetReleaseBinding(unknown): active=%v err=%v, want nil and no error", active, err)
	}
	history, err := s.ListReleaseBindings(ctx, importer.DefaultNamespace, "pub_none")
	if err != nil || len(history) != 0 {
		t.Fatalf("ListReleaseBindings(unknown): %d rows err=%v, want none", len(history), err)
	}
}
