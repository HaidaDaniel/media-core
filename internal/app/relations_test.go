package app

import (
	"context"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

func mustAddRelationView(t *testing.T, s *Service, req AddRelationRequest) RelationView {
	t.Helper()
	view, err := s.AddRelation(context.Background(), req)
	if err != nil {
		t.Fatalf("AddRelation(%+v): %v", req, err)
	}
	return view
}

func mustConfirmRelationView(t *testing.T, s *Service, req ConfirmRelationRequest) RelationView {
	t.Helper()
	view, err := s.ConfirmRelation(context.Background(), req)
	if err != nil {
		t.Fatalf("ConfirmRelation(%+v): %v", req, err)
	}
	return view
}

func mustRevokeRelationView(t *testing.T, s *Service, req RevokeRelationRequest) RelationView {
	t.Helper()
	view, err := s.RevokeRelation(context.Background(), req)
	if err != nil {
		t.Fatalf("RevokeRelation(%+v): %v", req, err)
	}
	return view
}

func mustRecordOriginView(t *testing.T, s *Service, req RecordOriginRequest) OriginView {
	t.Helper()
	view, err := s.RecordOrigin(context.Background(), req)
	if err != nil {
		t.Fatalf("RecordOrigin(%+v): %v", req, err)
	}
	return view
}

func mustConfirmOriginView(t *testing.T, s *Service, req ConfirmOriginRequest) OriginView {
	t.Helper()
	view, err := s.ConfirmOrigin(context.Background(), req)
	if err != nil {
		t.Fatalf("ConfirmOrigin(%+v): %v", req, err)
	}
	return view
}

func mustRevokeOriginView(t *testing.T, s *Service, req RevokeOriginRequest) OriginView {
	t.Helper()
	view, err := s.RevokeOrigin(context.Background(), req)
	if err != nil {
		t.Fatalf("RevokeOrigin(%+v): %v", req, err)
	}
	return view
}

func workRevision(t *testing.T, s *Service, id string) int64 {
	t.Helper()
	w, err := s.GetWork(context.Background(), id)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	return w.Revision
}

func editionRevision(t *testing.T, s *Service, id string) int64 {
	t.Helper()
	e, err := s.GetEdition(context.Background(), id)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	return e.Revision
}

func canonicalFrom(a, b string) string {
	f, _ := domain.CanonicalEndpoints(a, b)
	return f
}

// TestRelationServiceLifecycle proves the service relation lifecycle: a
// proposed edge stays proposed, an exact retry replays without a second
// edge, the same key with a different payload conflicts, confirmation and
// revocation move the edge between view buckets, and a revoked edge stays
// visible in the view.
func TestRelationServiceLifecycle(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Relation Story")
	eA := addEdition(t, s, w.ID, domain.KindAnime, "Anime")
	eB := addEdition(t, s, w.ID, domain.KindManga, "Manga")

	req := AddRelationRequest{
		MutationBase: mustBase(t, "req-rel-1", eA.Revision),
		Scope:        domain.ScopeEdition,
		Type:         domain.RelationAdaptationOf,
		FromID:       eA.ID,
		ToID:         eB.ID,
		State:        domain.ClaimProposed,
		Provenance:   "manual",
	}
	view := mustAddRelationView(t, s, req)
	if len(view.Proposed) != 1 || view.Proposed[0].State != domain.ClaimProposed {
		t.Fatalf("view = %+v, want the proposed edge", view)
	}
	if editionRevision(t, s, eA.ID) != eA.Revision+1 {
		t.Fatalf("edition revision = %d, want %d (a relation bumps the from entity)", editionRevision(t, s, eA.ID), eA.Revision+1)
	}
	// The exact retry replays without a second edge.
	replayed := mustAddRelationView(t, s, req)
	if len(replayed.Proposed) != 1 || replayed.Proposed[0].ID != view.Proposed[0].ID {
		t.Fatalf("replayed = %+v, want the same proposed edge", replayed)
	}
	var rows int
	if err := s.db.DB().QueryRow(`SELECT COUNT(*) FROM media_relations`).Scan(&rows); err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if rows != 1 {
		t.Fatalf("edges = %d, want 1 after the replay", rows)
	}
	// The same key with a different payload conflicts.
	different := req
	different.Provenance = "other"
	if _, err := s.AddRelation(context.Background(), different); appError(t, err).Code != ErrConflict {
		t.Fatalf("same key different payload: want conflict, got %v", err)
	}
	// The explicit confirmation moves the edge to the confirmed bucket.
	view = mustConfirmRelationView(t, s, ConfirmRelationRequest{
		MutationBase: mustBase(t, "req-rel-2", editionRevision(t, s, eA.ID)),
		RelationID:   view.Proposed[0].ID,
	})
	if len(view.Confirmed) != 1 || view.Confirmed[0].ConfirmedBy == nil || *view.Confirmed[0].ConfirmedBy != "local" {
		t.Fatalf("view = %+v, want the confirmed edge with the confirmer", view)
	}
	// Re-confirming is a no-op without a revision bump.
	before := editionRevision(t, s, eA.ID)
	view = mustConfirmRelationView(t, s, ConfirmRelationRequest{
		MutationBase: mustBase(t, "req-rel-3", before),
		RelationID:   view.Confirmed[0].ID,
	})
	if len(view.Confirmed) != 1 || editionRevision(t, s, eA.ID) != before {
		t.Fatalf("re-confirmation bumped the edition: view = %+v, revision = %d", view, editionRevision(t, s, eA.ID))
	}
	// The revocation keeps the edge visible in the revoked bucket.
	view = mustRevokeRelationView(t, s, RevokeRelationRequest{
		MutationBase: mustBase(t, "req-rel-4", before),
		RelationID:   view.Confirmed[0].ID,
	})
	if len(view.Revoked) != 1 || view.Revoked[0].State != domain.ClaimRevoked {
		t.Fatalf("view = %+v, want the revoked edge in history", view)
	}
	// The revoked edge is absent from the traversal.
	related, err := s.Related(context.Background(), domain.ScopeEdition, eA.ID, 1)
	if err != nil {
		t.Fatalf("Related: %v", err)
	}
	if len(related.Edges) != 0 {
		t.Fatalf("related = %+v, want none (the edge is revoked)", related)
	}
}

// TestRelationServiceErrors proves the stable error mapping on the
// relation path: malformed requests are invalid arguments that reserve no
// key, missing endpoints are not-found, a stale revision conflicts, a
// cyclic confirmed source link is a cycle error with no partial mutation,
// and confirming a revoked edge conflicts.
func TestRelationServiceErrors(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	w := mustCreateWork(t, s, "Relation Errors")
	eA := addEdition(t, s, w.ID, domain.KindAnime, "Anime")
	eB := addEdition(t, s, w.ID, domain.KindManga, "Manga")

	base := AddRelationRequest{
		MutationBase: mustBase(t, "req-rel-err", eA.Revision),
		Scope:        domain.ScopeEdition,
		Type:         domain.RelationAdaptationOf,
		FromID:       eA.ID,
		ToID:         eB.ID,
		State:        domain.ClaimProposed,
		Provenance:   "manual",
	}
	bad := base
	bad.FromID = bad.ToID
	bad.RequestID = "req-rel-err-self"
	if _, err := s.AddRelation(ctx, bad); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("self link: want invalid_argument, got %v", err)
	}
	bad = base
	bad.RequestID = "req-rel-err-type"
	bad.Type = domain.RelationType("hologram")
	if _, err := s.AddRelation(ctx, bad); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("unknown type: want invalid_argument, got %v", err)
	}
	bad = base
	bad.RequestID = "req-rel-err-scope"
	bad.Type = domain.RelationSequelOf
	if _, err := s.AddRelation(ctx, bad); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("work type in edition scope: want invalid_argument, got %v", err)
	}
	bad = base
	bad.RequestID = "req-rel-err-empty"
	bad.ToID = "   "
	if _, err := s.AddRelation(ctx, bad); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("blank endpoint: want invalid_argument, got %v", err)
	}
	bad = base
	bad.RequestID = "req-rel-err-prov"
	bad.Provenance = "   "
	if _, err := s.AddRelation(ctx, bad); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("blank provenance: want invalid_argument, got %v", err)
	}
	noRev := base
	noRev.RequestID = "req-rel-err-norev"
	noRev.ExpectedRevision = nil
	if _, err := s.AddRelation(ctx, noRev); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("missing expected_revision: want invalid_argument, got %v", err)
	}

	// A missing endpoint is a not-found.
	bad = base
	bad.RequestID = "req-rel-err-missing"
	bad.ToID = "00000000-0000-4000-8000-000000000000"
	if _, err := s.AddRelation(ctx, bad); appError(t, err).Code != ErrNotFound {
		t.Fatalf("missing endpoint: want not_found, got %v", err)
	}
	// A stale revision conflicts.
	if _, err := s.AddRelation(ctx, AddRelationRequest{
		MutationBase: mustBase(t, "req-rel-err-stale", 99),
		Scope:        domain.ScopeEdition,
		Type:         domain.RelationAdaptationOf,
		FromID:       eA.ID,
		ToID:         eB.ID,
		State:        domain.ClaimProposed,
		Provenance:   "manual",
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("stale revision: want conflict, got %v", err)
	}

	// The failed validations reserved no key: the original key carries the
	// edge afterwards.
	view := mustAddRelationView(t, s, base)
	edgeID := view.Proposed[0].ID
	// Confirming the proposed edge adds it to the confirmed source graph.
	view = mustConfirmRelationView(t, s, ConfirmRelationRequest{
		MutationBase: mustBase(t, "req-rel-err-cycle2", editionRevision(t, s, eA.ID)),
		RelationID:   edgeID,
	})
	if len(view.Confirmed) != 1 {
		t.Fatalf("view = %+v, want the confirmed edge", view)
	}
	// A cyclic confirmed source link is a cycle error with no partial
	// mutation.
	if _, err := s.AddRelation(ctx, AddRelationRequest{
		MutationBase: mustBase(t, "req-rel-err-cycle", editionRevision(t, s, eB.ID)),
		Scope:        domain.ScopeEdition,
		Type:         domain.RelationAdaptationOf,
		FromID:       eB.ID,
		ToID:         eA.ID,
		State:        domain.ClaimConfirmed,
		Provenance:   "manual",
	}); appError(t, err).Code != ErrCycleDetected {
		t.Fatalf("cyclic confirmed edge: want cycle_detected, got %v", err)
	}
	var rows int
	if err := s.db.DB().QueryRow(`SELECT COUNT(*) FROM media_relations WHERE from_id = ? AND to_id = ?`, eB.ID, eA.ID).Scan(&rows); err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if rows != 0 {
		t.Fatal("the cyclic edge was written (partial mutation)")
	}
	// Revoking and then confirming the edge conflicts.
	mustRevokeRelationView(t, s, RevokeRelationRequest{
		MutationBase: mustBase(t, "req-rel-err-rev", editionRevision(t, s, eA.ID)),
		RelationID:   edgeID,
	})
	if _, err := s.ConfirmRelation(ctx, ConfirmRelationRequest{
		MutationBase: mustBase(t, "req-rel-err-revconf", editionRevision(t, s, eA.ID)),
		RelationID:   edgeID,
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("confirming a revoked edge: want conflict, got %v", err)
	}
	// A missing relation is a not-found.
	if _, err := s.ConfirmRelation(ctx, ConfirmRelationRequest{
		MutationBase: mustBase(t, "req-rel-err-unknown", 0),
		RelationID:   "00000000-0000-4000-8000-000000000000",
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("unknown relation: want not_found, got %v", err)
	}
}

// TestRelationServiceWorkScope proves work-scoped relations: the sequel
// lifecycle on works and the bounded both-directions traversal that shows
// proposals separately.
func TestRelationServiceWorkScope(t *testing.T) {
	s := newTestService(t)
	w1 := mustCreateWork(t, s, "First Story")
	w2 := mustCreateWork(t, s, "Second Story")
	w3 := mustCreateWork(t, s, "Third Story")

	// A proposed sequel: later -> earlier.
	view := mustAddRelationView(t, s, AddRelationRequest{
		MutationBase: mustBase(t, "req-wrel-1", w2.Revision),
		Scope:        domain.ScopeWork,
		Type:         domain.RelationSequelOf,
		FromID:       w2.ID,
		ToID:         w1.ID,
		State:        domain.ClaimProposed,
		Provenance:   "manual",
	})
	if len(view.Proposed) != 1 || view.Proposed[0].Scope != domain.ScopeWork {
		t.Fatalf("view = %+v, want the proposed work relation", view)
	}
	// The confirmation records the confirmer.
	view = mustConfirmRelationView(t, s, ConfirmRelationRequest{
		MutationBase: mustBase(t, "req-wrel-2", workRevision(t, s, w2.ID)),
		RelationID:   view.Proposed[0].ID,
	})
	if len(view.Confirmed) != 1 || view.Confirmed[0].ConfirmedBy == nil {
		t.Fatalf("view = %+v, want the confirmed sequel", view)
	}
	// A symmetric related edge returns the view of the canonical from work.
	view = mustAddRelationView(t, s, AddRelationRequest{
		MutationBase: mustBase(t, "req-wrel-3", workRevision(t, s, canonicalFrom(w1.ID, w3.ID))),
		Scope:        domain.ScopeWork,
		Type:         domain.RelationRelated,
		FromID:       w3.ID,
		ToID:         w1.ID,
		State:        domain.ClaimConfirmed,
		Provenance:   "manual",
	})
	if len(view.Confirmed) != 1 {
		t.Fatalf("view = %+v, want the related edge", view)
	}
	// The traversal from w1 reaches w2 (sequel) and w3 (related).
	related, err := s.Related(context.Background(), domain.ScopeWork, w1.ID, 2)
	if err != nil {
		t.Fatalf("Related: %v", err)
	}
	if len(related.Edges) != 2 {
		t.Fatalf("related = %+v, want the two neighbouring works", related)
	}
	types := map[domain.RelationType]bool{}
	for _, e := range related.Edges {
		types[e.Type] = true
	}
	if !types[domain.RelationSequelOf] || !types[domain.RelationRelated] {
		t.Fatalf("related types = %+v, want the sequel and the related edge", types)
	}
}

// TestOriginService proves the service origin lifecycle: explicit
// assertions with replay, the correction through a replacement that keeps
// both claims visible, and the stable error mapping.
func TestOriginService(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	w := mustCreateWork(t, s, "Origin Story")
	e := addEdition(t, s, w.ID, domain.KindWebNovel, "The WN")

	req := RecordOriginRequest{
		MutationBase: mustBase(t, "req-orig-1", e.Revision),
		EditionID:    e.ID,
		Kind:         domain.OriginAnimeOriginal,
		State:        domain.ClaimProposed,
		Provenance:   "manual",
	}
	view := mustRecordOriginView(t, s, req)
	if len(view.Claims) != 1 || view.Claims[0].State != domain.ClaimProposed {
		t.Fatalf("view = %+v, want the proposed claim", view)
	}
	claimID := view.Claims[0].ID
	// The exact retry replays without a second claim.
	replayed := mustRecordOriginView(t, s, req)
	if len(replayed.Claims) != 1 || replayed.Claims[0].ID != claimID {
		t.Fatalf("replayed = %+v, want the same claim", replayed)
	}
	// The same key with a different payload conflicts.
	different := req
	different.Provenance = "other"
	if _, err := s.RecordOrigin(ctx, different); appError(t, err).Code != ErrConflict {
		t.Fatalf("same key different payload: want conflict, got %v", err)
	}
	// A different assertion of the same kind conflicts without a
	// replacement.
	if _, err := s.RecordOrigin(ctx, RecordOriginRequest{
		MutationBase: mustBase(t, "req-orig-2", editionRevision(t, s, e.ID)),
		EditionID:    e.ID,
		Kind:         domain.OriginAnimeOriginal,
		State:        domain.ClaimProposed,
		Provenance:   "another source",
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("same kind different provenance: want conflict, got %v", err)
	}
	// The explicit confirmation records the confirmer.
	view = mustConfirmOriginView(t, s, ConfirmOriginRequest{
		MutationBase: mustBase(t, "req-orig-3", editionRevision(t, s, e.ID)),
		EditionID:    e.ID,
		ClaimID:      claimID,
	})
	if len(view.Claims) != 1 || view.Claims[0].State != domain.ClaimConfirmed || view.Claims[0].ConfirmedBy == nil || *view.Claims[0].ConfirmedBy != "local" {
		t.Fatalf("view = %+v, want the confirmed claim", view)
	}
	// The correction: a replacement revokes the confirmed claim and keeps
	// both rows visible with the replacement link.
	view = mustRecordOriginView(t, s, RecordOriginRequest{
		MutationBase: mustBase(t, "req-orig-4", editionRevision(t, s, e.ID)),
		EditionID:    e.ID,
		Kind:         domain.OriginOriginal,
		State:        domain.ClaimConfirmed,
		Provenance:   "corrected",
		ReplacesID:   strPtr(claimID),
	})
	if len(view.Claims) != 2 {
		t.Fatalf("view = %+v, want both claims (the corrected history stays visible)", view)
	}
	var replaced *OriginClaim
	var active *OriginClaim
	for i := range view.Claims {
		switch view.Claims[i].State {
		case domain.ClaimRevoked:
			replaced = &view.Claims[i]
		default:
			active = &view.Claims[i]
		}
	}
	if replaced == nil || active == nil || active.ReplacesID == nil || *active.ReplacesID != claimID {
		t.Fatalf("claims = %+v, want the replacement link to the revoked claim", view.Claims)
	}
	// A missing claim is a not-found; a claim of another edition is a
	// not-found on this one.
	w2 := mustCreateWork(t, s, "Other Story")
	e2 := addEdition(t, s, w2.ID, domain.KindAnime, "Other Anime")
	if _, err := s.ConfirmOrigin(ctx, ConfirmOriginRequest{
		MutationBase: mustBase(t, "req-orig-5", e2.Revision),
		EditionID:    e2.ID,
		ClaimID:      claimID,
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("foreign-edition claim: want not_found, got %v", err)
	}
	if _, err := s.RevokeOrigin(ctx, RevokeOriginRequest{
		MutationBase: mustBase(t, "req-orig-6", editionRevision(t, s, e.ID)),
		EditionID:    e.ID,
		ClaimID:      "00000000-0000-4000-8000-000000000000",
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("unknown claim: want not_found, got %v", err)
	}
	// Confirming the revoked claim conflicts; recording with a revoked
	// state or an unknown kind is an invalid argument.
	if _, err := s.ConfirmOrigin(ctx, ConfirmOriginRequest{
		MutationBase: mustBase(t, "req-orig-7", editionRevision(t, s, e.ID)),
		EditionID:    e.ID,
		ClaimID:      claimID,
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("confirming a revoked claim: want conflict, got %v", err)
	}
	if _, err := s.RecordOrigin(ctx, RecordOriginRequest{
		MutationBase: mustBase(t, "req-orig-8", editionRevision(t, s, e.ID)),
		EditionID:    e.ID,
		Kind:         domain.OriginUnknown,
		State:        domain.ClaimRevoked,
		Provenance:   "manual",
	}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("revoked creation state: want invalid_argument, got %v", err)
	}
	if _, err := s.RecordOrigin(ctx, RecordOriginRequest{
		MutationBase: mustBase(t, "req-orig-9", editionRevision(t, s, e.ID)),
		EditionID:    e.ID,
		Kind:         domain.OriginKind("hologram"),
		State:        domain.ClaimProposed,
		Provenance:   "manual",
	}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("unknown kind: want invalid_argument, got %v", err)
	}
	// Re-revoking the active claim after revoking it is a no-op without a
	// revision bump.
	view = mustRevokeOriginView(t, s, RevokeOriginRequest{
		MutationBase: mustBase(t, "req-orig-10", editionRevision(t, s, e.ID)),
		EditionID:    e.ID,
		ClaimID:      active.ID,
	})
	if len(view.Claims) != 2 {
		t.Fatalf("view = %+v, want both claims kept in history", view)
	}
	before := editionRevision(t, s, e.ID)
	mustRevokeOriginView(t, s, RevokeOriginRequest{
		MutationBase: mustBase(t, "req-orig-11", before),
		EditionID:    e.ID,
		ClaimID:      active.ID,
	})
	if editionRevision(t, s, e.ID) != before {
		t.Fatalf("re-revocation bumped the edition: %d, want %d", editionRevision(t, s, e.ID), before)
	}
}

// TestOriginNoInferenceService proves the service never infers an origin:
// an edition without claims lists none, whatever its media kind.
func TestOriginNoInferenceService(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Plain Story")
	e := addEdition(t, s, w.ID, domain.KindWebNovel, "Plain WN")
	view, err := s.ListOrigins(context.Background(), e.ID)
	if err != nil {
		t.Fatalf("ListOrigins: %v", err)
	}
	if len(view.Claims) != 0 {
		t.Fatalf("claims = %+v, want none (a kind never implies an origin)", view.Claims)
	}
}
