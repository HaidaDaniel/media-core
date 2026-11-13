package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// seedWorkOnly creates one work without an edition and returns its id; a
// work-only seed stays at revision 0.
func seedWorkOnly(t *testing.T, s *app.Service, requestID, title string) string {
	t.Helper()
	id, ed := seedWork(t, s, requestID, []store.TitleInput{titleInput(title, domain.TitleOriginal)}, nil)
	if ed != nil {
		t.Fatalf("seed %s created an edition", requestID)
	}
	return id
}

// workRevision reads the work detail view and returns its revision.
func workRevision(t *testing.T, srv *Server, cookie *http.Cookie, workID string) float64 {
	t.Helper()
	resp, raw := catalogGet(t, srv, cookie, "/api/v1/works/"+workID)
	data := expectSuccess(t, resp, raw)
	return data["revision"].(float64)
}

// relatedEdges returns the traversal edges of one relations read.
func relatedEdges(t *testing.T, srv *Server, cookie *http.Cookie, path string) []map[string]any {
	t.Helper()
	resp, raw := catalogGet(t, srv, cookie, path)
	data := expectSuccess(t, resp, raw)
	edges, _ := data["edges"].([]any)
	out := make([]map[string]any, 0, len(edges))
	for _, e := range edges {
		out = append(out, e.(map[string]any))
	}
	return out
}

func TestHTTPRelationRead(t *testing.T) {
	var a, b string
	srv := startCatalogServer(t, func(s *app.Service) {
		a = seedWorkOnly(t, s, "seed-rel-a", "Relation Alpha")
		b = seedWorkOnly(t, s, "seed-rel-b", "Relation Beta")
		zero := int64(0)
		if _, err := s.AddRelation(context.Background(), app.AddRelationRequest{
			MutationBase: app.MutationBase{RequestID: "seed-rel-1", Actor: testActor, ExpectedRevision: &zero},
			Scope:        domain.ScopeWork, Type: domain.RelationSequelOf,
			FromID: a, ToID: b, State: domain.ClaimConfirmed, Provenance: "manual",
		}); err != nil {
			t.Fatalf("seed relation: %v", err)
		}
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, _ := login(t, srv, secret)

	// The forward traversal shows the edge with its source direction.
	edges := relatedEdges(t, srv, cookie, "/api/v1/works/"+a+"/relations")
	if len(edges) != 1 {
		t.Fatalf("edges from %s = %v, want one", a, edges)
	}
	if edges[0]["from_id"] != a || edges[0]["to_id"] != b || edges[0]["state"] != "confirmed" || edges[0]["type"] != "sequel_of" {
		t.Fatalf("edge = %v, want the confirmed %s -> %s sequel", edges[0], a, b)
	}
	if got := edges[0]["depth"].(float64); got != 1 {
		t.Fatalf("depth = %v, want 1", got)
	}

	// The reverse navigation reaches the same edge without flipping its
	// direction.
	edges = relatedEdges(t, srv, cookie, "/api/v1/works/"+b+"/relations")
	if len(edges) != 1 {
		t.Fatalf("edges from %s = %v, want one", b, edges)
	}
	if edges[0]["from_id"] != a || edges[0]["to_id"] != b {
		t.Fatalf("reverse edge = %v, want the direction preserved", edges[0])
	}

	// A present but unparsable depth is an invalid argument.
	resp, raw := catalogGet(t, srv, cookie, "/api/v1/works/"+a+"/relations?depth=abc")
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// An absent or out-of-range depth falls back to the service bound.
	if edges := relatedEdges(t, srv, cookie, "/api/v1/works/"+a+"/relations?depth=99"); len(edges) != 1 {
		t.Fatalf("clamped-depth edges = %v, want one", edges)
	}

	// An unknown entity has an empty traversal, not an error.
	if edges := relatedEdges(t, srv, cookie, "/api/v1/works/no-such-work/relations"); len(edges) != 0 {
		t.Fatalf("edges for an unknown work = %v, want none", edges)
	}

	// The read boundary requires a live session.
	resp, raw = doJSON(t, srv, http.MethodGet, "/api/v1/works/"+a+"/relations", nil, nil)
	expectFailure(t, resp, raw, http.StatusUnauthorized, string(app.ErrUnauthorized))
}

func TestHTTPRelationWrite(t *testing.T) {
	var a, b, c string
	srv := startCatalogServer(t, func(s *app.Service) {
		a = seedWorkOnly(t, s, "seed-rw-a", "RW Alpha")
		b = seedWorkOnly(t, s, "seed-rw-b", "RW Beta")
		c = seedWorkOnly(t, s, "seed-rw-c", "RW Gamma")
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	// Propose one edge with an explicit confidence.
	resp, raw := catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/relations", map[string]any{
		"request_id": "rel-1", "expected_revision": 0,
		"scope": "work", "type": "sequel_of", "from_id": a, "to_id": b,
		"state": "proposed", "provenance": "manual", "confidence": 0.9,
	})
	data := expectSuccess(t, resp, raw)
	proposed, _ := data["proposed"].([]any)
	if len(proposed) != 1 {
		t.Fatalf("proposed = %v, want one", data["proposed"])
	}
	edge := proposed[0].(map[string]any)
	edgeID := edge["id"].(string)
	if edge["confidence"] != 0.9 || edge["state"] != "proposed" {
		t.Fatalf("edge = %v, want the proposed edge with its confidence", edge)
	}
	if got := workRevision(t, srv, cookie, a); got != 1 {
		t.Fatalf("from revision after the propose = %v, want 1", got)
	}

	// A duplicate active edge returns the existing edge without a bump.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/relations", map[string]any{
		"request_id": "rel-2", "expected_revision": 1,
		"scope": "work", "type": "sequel_of", "from_id": a, "to_id": b,
		"state": "proposed", "provenance": "manual", "confidence": 0.9,
	})
	data = expectSuccess(t, resp, raw)
	if got := data["proposed"].([]any)[0].(map[string]any)["id"]; got != edgeID {
		t.Fatalf("duplicate edge id = %v, want %s", got, edgeID)
	}
	if got := workRevision(t, srv, cookie, a); got != 1 {
		t.Fatalf("from revision after the duplicate no-op = %v, want 1", got)
	}

	// An exact retry — the same key and the same payload, including
	// its original revision — replays without writing.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/relations", map[string]any{
		"request_id": "rel-1", "expected_revision": 0,
		"scope": "work", "type": "sequel_of", "from_id": a, "to_id": b,
		"state": "proposed", "provenance": "manual", "confidence": 0.9,
	})
	expectSuccess(t, resp, raw)
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/relations", map[string]any{
		"request_id": "rel-1", "expected_revision": 1,
		"scope": "work", "type": "sequel_of", "from_id": a, "to_id": c,
		"state": "proposed", "provenance": "manual", "confidence": 0.9,
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))

	// Invalid assertions are rejected without a write.
	for _, body := range []map[string]any{
		{"request_id": "rel-bad-self", "expected_revision": 1, "scope": "work", "type": "sequel_of", "from_id": a, "to_id": a, "state": "proposed", "provenance": "manual"},
		{"request_id": "rel-bad-scope", "expected_revision": 1, "scope": "work", "type": "adaptation_of", "from_id": a, "to_id": b, "state": "proposed", "provenance": "manual"},
		{"request_id": "rel-bad-state", "expected_revision": 1, "scope": "work", "type": "sequel_of", "from_id": a, "to_id": b, "state": "revoked", "provenance": "manual"},
		{"request_id": "rel-bad-prov", "expected_revision": 1, "scope": "work", "type": "sequel_of", "from_id": a, "to_id": b, "state": "proposed"},
	} {
		resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/relations", body)
		expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
	}

	// An endpoint outside the scope is a not found.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/relations", map[string]any{
		"request_id": "rel-bad-endpoint", "expected_revision": 1,
		"scope": "work", "type": "sequel_of", "from_id": a, "to_id": "no-such-work",
		"state": "proposed", "provenance": "manual",
	})
	expectFailure(t, resp, raw, http.StatusNotFound, string(app.ErrNotFound))

	// Explicit confirmation moves the edge into the confirmed list.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/relations/"+edgeID+"/confirm", map[string]any{
		"request_id": "rel-3", "expected_revision": 1,
	})
	data = expectSuccess(t, resp, raw)
	if confirmed, _ := data["confirmed"].([]any); len(confirmed) != 1 {
		t.Fatalf("confirmed = %v, want one", data["confirmed"])
	}
	if proposed, _ := data["proposed"].([]any); len(proposed) != 0 {
		t.Fatalf("proposed after the confirm = %v, want none", data["proposed"])
	}
	if got := workRevision(t, srv, cookie, a); got != 2 {
		t.Fatalf("from revision after the confirm = %v, want 2", got)
	}

	// Confirming an already-confirmed edge is a no-op without a bump.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/relations/"+edgeID+"/confirm", map[string]any{
		"request_id": "rel-4", "expected_revision": 2,
	})
	expectSuccess(t, resp, raw)
	if got := workRevision(t, srv, cookie, a); got != 2 {
		t.Fatalf("from revision after the confirm no-op = %v, want 2", got)
	}

	// A cycle: b -> c confirmed, c -> a proposed, then confirming
	// c -> a would loop the confirmed sequel graph.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/relations", map[string]any{
		"request_id": "rel-5", "expected_revision": 0,
		"scope": "work", "type": "sequel_of", "from_id": b, "to_id": c,
		"state": "confirmed", "provenance": "manual",
	})
	expectSuccess(t, resp, raw)
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/relations", map[string]any{
		"request_id": "rel-6", "expected_revision": 0,
		"scope": "work", "type": "sequel_of", "from_id": c, "to_id": a,
		"state": "proposed", "provenance": "manual",
	})
	data = expectSuccess(t, resp, raw)
	loopID := data["proposed"].([]any)[0].(map[string]any)["id"].(string)
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/relations/"+loopID+"/confirm", map[string]any{
		"request_id": "rel-7", "expected_revision": 1,
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrCycleDetected))

	// The failed confirmation leaves the edge proposed and the graph
	// intact.
	edges := relatedEdges(t, srv, cookie, "/api/v1/works/"+c+"/relations")
	stillProposed := false
	for _, e := range edges {
		if e["from_id"] == c && e["to_id"] == a && e["state"] == "proposed" {
			stillProposed = true
		}
	}
	if !stillProposed {
		t.Fatalf("c -> a edge after the cycle rejection = %v, want still proposed", edges)
	}

	// Revoke the confirmed edge; it leaves the traversal but stays in
	// the revoked list.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/relations/"+edgeID+"/revoke", map[string]any{
		"request_id": "rel-8", "expected_revision": 2,
	})
	data = expectSuccess(t, resp, raw)
	if revoked, _ := data["revoked"].([]any); len(revoked) != 1 {
		t.Fatalf("revoked = %v, want one", data["revoked"])
	}
	// The revoked edge leaves the traversal; the still-proposed c -> a
	// edge stays.
	for _, e := range relatedEdges(t, srv, cookie, "/api/v1/works/"+a+"/relations") {
		if e["relation_id"] == edgeID {
			t.Fatalf("revoked edge still traversed: %v", e)
		}
	}

	// Confirming a revoked edge is a distinguishable conflict.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/relations/"+edgeID+"/confirm", map[string]any{
		"request_id": "rel-9", "expected_revision": 3,
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))

	// A stale from revision conflicts.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/relations", map[string]any{
		"request_id": "rel-10", "expected_revision": 0,
		"scope": "work", "type": "sequel_of", "from_id": a, "to_id": c,
		"state": "proposed", "provenance": "manual",
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))

	// An unknown relation id is a not found.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/relations/no-such-relation/confirm", map[string]any{
		"request_id": "rel-11", "expected_revision": 3,
	})
	expectFailure(t, resp, raw, http.StatusNotFound, string(app.ErrNotFound))
}

func TestHTTPRelationAuth(t *testing.T) {
	var a, b string
	srv := startCatalogServer(t, func(s *app.Service) {
		a = seedWorkOnly(t, s, "seed-ra-a", "Auth Alpha")
		b = seedWorkOnly(t, s, "seed-ra-b", "Auth Beta")
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, _ := login(t, srv, secret)

	body := map[string]any{
		"request_id": "rel-auth", "expected_revision": 0,
		"scope": "work", "type": "sequel_of", "from_id": a, "to_id": b,
		"state": "proposed", "provenance": "manual",
	}

	// No session: unauthorized.
	resp, raw := doJSON(t, srv, http.MethodPost, "/api/v1/relations", nil, body)
	expectFailure(t, resp, raw, http.StatusUnauthorized, string(app.ErrUnauthorized))

	// A live session without the CSRF token: forbidden.
	resp, raw = doJSON(t, srv, http.MethodPost, "/api/v1/origins", map[string]string{"Cookie": cookie.String()}, map[string]any{
		"request_id": "org-auth", "expected_revision": 0, "edition_id": "no-such-edition",
		"kind": "original", "state": "proposed", "provenance": "manual",
	})
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// Nothing was written.
	if edges := relatedEdges(t, srv, cookie, "/api/v1/works/"+a+"/relations"); len(edges) != 0 {
		t.Fatalf("edges after the rejected mutations = %v, want none", edges)
	}
}

func TestHTTPOrigin(t *testing.T) {
	var editionID string
	srv := startCatalogServer(t, func(s *app.Service) {
		_, ed := seedWork(t, s, "seed-origin", []store.TitleInput{titleInput("Origin Story", domain.TitleOriginal)}, appKind(domain.KindManga))
		if ed == nil {
			t.Fatal("seed created no edition")
		}
		editionID = *ed
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	// Assert one proposed origin claim; no originality is inferred.
	resp, raw := catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/origins", map[string]any{
		"request_id": "org-1", "expected_revision": 0,
		"edition_id": editionID, "kind": "original", "state": "proposed", "provenance": "manual",
	})
	data := expectSuccess(t, resp, raw)
	claims, _ := data["claims"].([]any)
	if len(claims) != 1 {
		t.Fatalf("claims = %v, want one", data["claims"])
	}
	claim := claims[0].(map[string]any)
	claimID := claim["id"].(string)
	if claim["state"] != "proposed" || claim["kind"] != "original" {
		t.Fatalf("claim = %v, want the proposed original claim", claim)
	}
	if claim["confirmed_at"] != nil {
		t.Fatalf("proposed claim confirmed_at = %v, want absent", claim["confirmed_at"])
	}

	// An exact re-assertion is a no-op without a revision bump.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/origins", map[string]any{
		"request_id": "org-2", "expected_revision": 1,
		"edition_id": editionID, "kind": "original", "state": "proposed", "provenance": "manual",
	})
	data = expectSuccess(t, resp, raw)
	if claims, _ := data["claims"].([]any); len(claims) != 1 {
		t.Fatalf("claims after the re-assertion = %v, want one", data["claims"])
	}

	// A different assertion of the same kind without a replacement
	// conflicts.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/origins", map[string]any{
		"request_id": "org-3", "expected_revision": 1,
		"edition_id": editionID, "kind": "original", "state": "confirmed", "provenance": "publisher",
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))

	// The correction replaces the named claim; the old one stays
	// revoked with its audit.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/origins", map[string]any{
		"request_id": "org-4", "expected_revision": 1,
		"edition_id": editionID, "kind": "original", "state": "confirmed", "provenance": "publisher",
		"replaces_id": claimID,
	})
	data = expectSuccess(t, resp, raw)
	claims, _ = data["claims"].([]any)
	if len(claims) != 2 {
		t.Fatalf("claims after the correction = %v, want two", data["claims"])
	}
	var old, fresh map[string]any
	for _, it := range claims {
		c := it.(map[string]any)
		if c["id"] == claimID {
			old = c
		} else {
			fresh = c
		}
	}
	if old["state"] != "revoked" || old["revoked_at"] == nil {
		t.Fatalf("replaced claim = %v, want revoked with its instant", old)
	}
	if fresh["state"] != "confirmed" || fresh["confirmed_at"] == nil {
		t.Fatalf("replacement claim = %v, want confirmed with its instant", fresh)
	}

	// Invalid claims are rejected without a write.
	for _, body := range []map[string]any{
		{"request_id": "org-bad-kind", "expected_revision": 2, "edition_id": editionID, "kind": "derivative", "state": "proposed", "provenance": "manual"},
		{"request_id": "org-bad-state", "expected_revision": 2, "edition_id": editionID, "kind": "original", "state": "revoked", "provenance": "manual"},
		{"request_id": "org-bad-prov", "expected_revision": 2, "edition_id": editionID, "kind": "original", "state": "proposed"},
	} {
		resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/origins", body)
		expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
	}

	// An unknown edition is a not found.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/origins", map[string]any{
		"request_id": "org-5", "expected_revision": 0,
		"edition_id": "no-such-edition", "kind": "original", "state": "proposed", "provenance": "manual",
	})
	expectFailure(t, resp, raw, http.StatusNotFound, string(app.ErrNotFound))

	// A stale edition revision conflicts.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/origins", map[string]any{
		"request_id": "org-6", "expected_revision": 0,
		"edition_id": editionID, "kind": "anime_original", "state": "proposed", "provenance": "manual",
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))
}
