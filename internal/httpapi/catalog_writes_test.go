package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// catalogWrite performs one authenticated catalog mutation with a live
// session cookie and the matching CSRF token.
func catalogWrite(t *testing.T, srv *Server, cookie *http.Cookie, csrf, method, path string, payload any) (*http.Response, []byte) {
	t.Helper()
	return doJSON(t, srv, method, path, map[string]string{"Cookie": cookie.String(), CSRFHeader: csrf}, payload)
}

// workCount reads the current work total through the shared service.
func workCount(t *testing.T, s *app.Service) int64 {
	t.Helper()
	payload, err := s.SearchLibrary(context.Background(), app.LibraryFilter{})
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	return payload.Total
}

// strPtrVal lifts a string to its pointer form for seed identities.
func strPtrVal(v string) *string { return &v }

// resolveCandidates resolves a title through the HTTP route and returns the
// candidate token; it fails the test unless the outcome is candidates.
func resolveCandidates(t *testing.T, srv *Server, cookie *http.Cookie, csrf string, payload map[string]any) string {
	t.Helper()
	resp, raw := catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/additions/resolve", payload)
	data := expectSuccess(t, resp, raw)
	if data["outcome"] != "candidates" {
		t.Fatalf("resolve outcome = %v, want candidates (body %s)", data["outcome"], raw)
	}
	token, _ := data["candidate_token"].(string)
	if token == "" {
		t.Fatalf("resolve candidates without a token (body %s)", raw)
	}
	return token
}

func TestHTTPCatalogWriteCreate(t *testing.T) {
	var svcRef *app.Service
	srv := startCatalogServer(t, func(s *app.Service) { svcRef = s })
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	body := map[string]any{
		"request_id": "req-create-1",
		"titles":     []map[string]any{{"text": "First Entry", "kind": "original", "provenance": "manual"}},
		"kind":       "book",
	}
	resp, raw := catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/works", body)
	data := expectSuccess(t, resp, raw)
	if data["created"] != true || data["work_id"] == "" || data["edition_id"] == "" {
		t.Fatalf("create = %v, want a created work with an edition (body %s)", data, raw)
	}
	workID := data["work_id"].(string)

	// A duplicate title without an explicit choice is a
	// duplicate-candidate conflict, never an automatic identity: no write.
	dup := map[string]any{
		"request_id": "req-create-2",
		"titles":     []map[string]any{{"text": "First Entry", "kind": "original", "provenance": "manual"}},
	}
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/works", dup)
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrDuplicateCandidate))
	if got := workCount(t, svcRef); got != 1 {
		t.Fatalf("works after the duplicate conflict = %d, want 1", got)
	}

	// An exact replay returns the stored result.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/works", body)
	replayed := expectSuccess(t, resp, raw)
	if replayed["created"] != true || replayed["work_id"] != workID {
		t.Fatalf("replay = %v, want the stored result for %s", replayed, workID)
	}

	// The same idempotency key with a different payload conflicts.
	clash := map[string]any{
		"request_id": "req-create-1",
		"titles":     []map[string]any{{"text": "Other Entry", "kind": "original", "provenance": "manual"}},
	}
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/works", clash)
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))

	// A missing idempotency key is an invalid argument.
	missing := map[string]any{
		"titles": []map[string]any{{"text": "No Key", "kind": "original", "provenance": "manual"}},
	}
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/works", missing)
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// Unknown mutation fields are rejected.
	unknown := map[string]any{
		"request_id": "req-create-3",
		"titles":     []map[string]any{{"text": "Surprise", "kind": "original", "provenance": "manual"}},
		"surprise":   true,
	}
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/works", unknown)
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
	if got := workCount(t, svcRef); got != 1 {
		t.Fatalf("works after the failed bodies = %d, want 1", got)
	}
}

func TestHTTPCatalogWriteResolve(t *testing.T) {
	seedWorkID := ""
	srv := startCatalogServer(t, func(s *app.Service) {
		// Seed a work carrying an external reference so the identity
		// path can be exercised.
		ref := store.ReferenceInput{Provider: strPtrVal("mal"), ExternalID: strPtrVal("424242")}
		kind := domain.KindBook
		res, err := s.CreateLibraryEntry(context.Background(), app.AdditionRequest{
			MutationBase: app.MutationBase{RequestID: "seed-ref", Actor: testActor},
			Titles:       []store.TitleInput{titleInput("Referenced Work", domain.TitleOriginal)},
			Kind:         &kind,
			References:   []store.ReferenceInput{ref},
		})
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
		seedWorkID = res.WorkID
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	// A title with no match is ready to create and carries a request
	// fingerprint.
	resp, raw := catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/additions/resolve", map[string]any{
		"titles": []map[string]any{{"text": "Brand New Story", "kind": "original", "provenance": "manual"}},
	})
	data := expectSuccess(t, resp, raw)
	if data["outcome"] != "ready" {
		t.Fatalf("resolve = %v, want ready (body %s)", data["outcome"], raw)
	}
	if data["request_fingerprint"] == nil {
		t.Fatalf("ready resolve without a request fingerprint (body %s)", raw)
	}
	if data["candidate_token"] != nil {
		t.Fatalf("ready resolve with a candidate token (body %s)", raw)
	}

	// An exact title match returns candidates with evidence and a token.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/additions/resolve", map[string]any{
		"titles": []map[string]any{{"text": "Referenced Work", "kind": "original", "provenance": "manual"}},
	})
	data = expectSuccess(t, resp, raw)
	if data["outcome"] != "candidates" {
		t.Fatalf("resolve = %v, want candidates (body %s)", data["outcome"], raw)
	}
	candidates, _ := data["candidates"].([]any)
	if len(candidates) == 0 || candidates[0].(map[string]any)["evidence"] == nil {
		t.Fatalf("candidates without evidence (body %s)", raw)
	}

	// A known external identity resolves to the existing work without a
	// candidate decision.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/additions/resolve", map[string]any{
		"references": []map[string]any{{"provider": "mal", "external_id": "424242"}},
	})
	data = expectSuccess(t, resp, raw)
	if data["outcome"] != "existing" || data["work_id"] != seedWorkID {
		t.Fatalf("identity resolve = %v, want existing for %s (body %s)", data["outcome"], seedWorkID, raw)
	}

	// No title, reference or work is an invalid argument.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/additions/resolve", map[string]any{})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// A candidate decision is not part of the resolve payload.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/additions/resolve", map[string]any{
		"titles": []map[string]any{{"text": "Referenced Work", "kind": "original", "provenance": "manual"}},
		"choice": "use_existing",
	})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// A work id is only meaningful with an explicit choice, so it is not
	// part of the resolve payload either.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/additions/resolve", map[string]any{
		"titles":  []map[string]any{{"text": "Referenced Work", "kind": "original", "provenance": "manual"}},
		"work_id": "some-work",
	})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// Resolve is still behind the session boundary.
	resp, raw = doJSON(t, srv, http.MethodPost, "/api/v1/additions/resolve", nil, map[string]any{
		"titles": []map[string]any{{"text": "Referenced Work", "kind": "original", "provenance": "manual"}},
	})
	expectFailure(t, resp, raw, http.StatusUnauthorized, string(app.ErrUnauthorized))
}

func TestHTTPCatalogWriteCandidateFlow(t *testing.T) {
	var svcRef *app.Service
	seedWorkID := ""
	srv := startCatalogServer(t, func(s *app.Service) {
		svcRef = s
		seedWorkID, _ = seedWork(t, s, "seed-same", []store.TitleInput{titleInput("Same Title", domain.TitleOriginal)}, appKind(domain.KindBook))
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	resolve := func(payload map[string]any) string {
		t.Helper()
		base := map[string]any{"titles": []map[string]any{{"text": "Same Title", "kind": "original", "provenance": "manual"}}}
		for k, v := range payload {
			base[k] = v
		}
		return resolveCandidates(t, srv, cookie, csrf, base)
	}

	// use_existing returns the seeded work and writes nothing.
	token := resolve(nil)
	resp, raw := catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/works", map[string]any{
		"request_id":      "req-use-existing",
		"titles":          []map[string]any{{"text": "Same Title", "kind": "original", "provenance": "manual"}},
		"choice":          "use_existing",
		"candidate_token": token,
	})
	data := expectSuccess(t, resp, raw)
	if data["created"] != false || data["work_id"] != seedWorkID {
		t.Fatalf("use_existing = %v, want created false for %s (body %s)", data, seedWorkID, raw)
	}
	if got := workCount(t, svcRef); got != 1 {
		t.Fatalf("works after use_existing = %d, want 1", got)
	}

	// add_edition adds a distinct edition under the seeded work.
	addToken := resolve(map[string]any{"kind": "manga"})
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/works/"+seedWorkID+"/editions", map[string]any{
		"request_id":      "req-add-edition",
		"titles":          []map[string]any{{"text": "Same Title", "kind": "original", "provenance": "manual"}},
		"kind":            "manga",
		"candidate_token": addToken,
	})
	data = expectSuccess(t, resp, raw)
	if data["created"] != true || data["work_id"] != seedWorkID || data["edition_id"] == "" {
		t.Fatalf("add_edition = %v, want a new edition under %s (body %s)", data, seedWorkID, raw)
	}
	if got := workCount(t, svcRef); got != 1 {
		t.Fatalf("works after add_edition = %d, want 1", got)
	}

	// create_distinct makes a new work.
	distinctToken := resolve(map[string]any{"kind": "anime"})
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/works", map[string]any{
		"request_id":      "req-create-distinct",
		"titles":          []map[string]any{{"text": "Same Title", "kind": "original", "provenance": "manual"}},
		"kind":            "anime",
		"choice":          "create_distinct",
		"candidate_token": distinctToken,
	})
	data = expectSuccess(t, resp, raw)
	if data["created"] != true || data["work_id"] == seedWorkID {
		t.Fatalf("create_distinct = %v, want a new work (body %s)", data, raw)
	}
	if got := workCount(t, svcRef); got != 2 {
		t.Fatalf("works after create_distinct = %d, want 2", got)
	}

	// A stale token: the candidate set changed after the resolve, so the
	// decision conflicts without writing.
	staleToken := resolve(nil)
	_, _ = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/works", map[string]any{
		"request_id":      "req-stale-setup",
		"titles":          []map[string]any{{"text": "Same Title", "kind": "original", "provenance": "manual"}},
		"kind":            "light_novel",
		"choice":          "create_distinct",
		"candidate_token": resolve(map[string]any{"kind": "light_novel"}),
	})
	before := workCount(t, svcRef)
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/works", map[string]any{
		"request_id":      "req-stale-use",
		"titles":          []map[string]any{{"text": "Same Title", "kind": "original", "provenance": "manual"}},
		"work_id":         seedWorkID,
		"choice":          "use_existing",
		"candidate_token": staleToken,
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))
	if got := workCount(t, svcRef); got != before {
		t.Fatalf("works after the stale decision = %d, want %d", got, before)
	}

	// A forged token is an invalid argument.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/works", map[string]any{
		"request_id":      "req-forged-token",
		"titles":          []map[string]any{{"text": "Same Title", "kind": "original", "provenance": "manual"}},
		"choice":          "use_existing",
		"candidate_token": "not-a-token",
	})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
}

func TestHTTPCatalogWriteCorrections(t *testing.T) {
	seedWorkID, seedEditionID := "", (*string)(nil)
	srv := startCatalogServer(t, func(s *app.Service) {
		seedWorkID, seedEditionID = seedWork(t, s, "seed-correct", []store.TitleInput{titleInput("Alpha Storm", domain.TitleOriginal)}, appKind(domain.KindBook))
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	// Discover the current work and edition revisions through the read
	// routes.
	resp, raw := catalogGet(t, srv, cookie, "/api/v1/works/"+seedWorkID)
	work := expectSuccess(t, resp, raw)
	workRevision := int64(work["revision"].(float64))
	editionID := seedEditionID
	if editionID == nil {
		t.Fatal("seed created no edition")
	}
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+*editionID)
	edition := expectSuccess(t, resp, raw)
	editionRevision := int64(edition["revision"].(float64))

	// Add an alias title at the current revision.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPatch, "/api/v1/works/"+seedWorkID+"/titles", map[string]any{
		"request_id":        "req-title-add",
		"expected_revision": workRevision,
		"add":               []map[string]any{{"text": "Alpha Storm: Remaster", "kind": "alias", "provenance": "manual"}},
	})
	work = expectSuccess(t, resp, raw)
	if got := int64(work["revision"].(float64)); got != workRevision+1 {
		t.Fatalf("work revision after add = %d, want %d", got, workRevision+1)
	}

	// A stale expected revision conflicts.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPatch, "/api/v1/works/"+seedWorkID+"/titles", map[string]any{
		"request_id":        "req-title-stale",
		"expected_revision": workRevision,
		"add":               []map[string]any{{"text": "Another Alias", "kind": "alias", "provenance": "manual"}},
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))

	// Retire the added alias; find its id in the updated view.
	var aliasID string
	for _, title := range work["titles"].([]any) {
		if title.(map[string]any)["text"] == "Alpha Storm: Remaster" {
			aliasID = title.(map[string]any)["id"].(string)
		}
	}
	if aliasID == "" {
		t.Fatalf("added alias not found in the work view: %v", work["titles"])
	}
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPatch, "/api/v1/works/"+seedWorkID+"/titles", map[string]any{
		"request_id":        "req-title-retire",
		"expected_revision": workRevision + 1,
		"retire":            []string{aliasID},
	})
	work = expectSuccess(t, resp, raw)
	for _, title := range work["titles"].([]any) {
		if title.(map[string]any)["text"] == "Alpha Storm: Remaster" {
			t.Fatalf("retired title still active: %v", work["titles"])
		}
	}

	// Update the edition metadata.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPatch, "/api/v1/editions/"+*editionID, map[string]any{
		"request_id":        "req-edition-update",
		"expected_revision": editionRevision,
		"language":          "ja",
	})
	edition = expectSuccess(t, resp, raw)
	if got := int64(edition["revision"].(float64)); got != editionRevision+1 {
		t.Fatalf("edition revision after update = %d, want %d", got, editionRevision+1)
	}
	if edition["language"] != "ja" {
		t.Fatalf("edition language = %v, want ja", edition["language"])
	}

	// A same-value no-op succeeds without incrementing the revision.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPatch, "/api/v1/editions/"+*editionID, map[string]any{
		"request_id":        "req-edition-noop",
		"expected_revision": editionRevision + 1,
		"language":          "ja",
	})
	edition = expectSuccess(t, resp, raw)
	if got := int64(edition["revision"].(float64)); got != editionRevision+1 {
		t.Fatalf("edition revision after no-op = %d, want %d", got, editionRevision+1)
	}

	// A stale edition revision conflicts.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPatch, "/api/v1/editions/"+*editionID, map[string]any{
		"request_id":        "req-edition-stale",
		"expected_revision": editionRevision,
		"language":          "en",
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))

	// No metadata fields is an invalid argument; unknown fields too.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPatch, "/api/v1/editions/"+*editionID, map[string]any{
		"request_id":        "req-edition-empty",
		"expected_revision": editionRevision + 1,
	})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPatch, "/api/v1/editions/"+*editionID, map[string]any{
		"request_id":        "req-edition-unknown",
		"expected_revision": editionRevision + 1,
		"language":          "en",
		"surprise":          true,
	})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
}

func TestHTTPCatalogWriteAuth(t *testing.T) {
	var svcRef *app.Service
	srv := startCatalogServer(t, func(s *app.Service) { svcRef = s })
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	body := map[string]any{
		"request_id": "req-auth",
		"titles":     []map[string]any{{"text": "Auth Probe", "kind": "original", "provenance": "manual"}},
	}

	// No session: unauthorized, nothing written.
	resp, raw := doJSON(t, srv, http.MethodPost, "/api/v1/works", nil, body)
	expectFailure(t, resp, raw, http.StatusUnauthorized, string(app.ErrUnauthorized))

	// A stale session: unauthorized.
	resp, raw = doJSON(t, srv, http.MethodPost, "/api/v1/works", map[string]string{"Cookie": SessionCookieName + "=stale"}, body)
	expectFailure(t, resp, raw, http.StatusUnauthorized, string(app.ErrUnauthorized))

	// A live session without the CSRF token: forbidden.
	resp, raw = doJSON(t, srv, http.MethodPost, "/api/v1/works", map[string]string{"Cookie": cookie.String()}, body)
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// A live session with the wrong CSRF token: forbidden.
	resp, raw = doJSON(t, srv, http.MethodPost, "/api/v1/works", map[string]string{"Cookie": cookie.String(), CSRFHeader: "wrong-token"}, body)
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// A live session with the CSRF token but a foreign origin: forbidden
	// by the security boundary before the session is consulted.
	resp, raw = doJSON(t, srv, http.MethodPost, "/api/v1/works", map[string]string{
		"Cookie":   cookie.String(),
		CSRFHeader: csrf,
		"Origin":   "http://evil.example",
	}, body)
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// None of the failed mutations wrote anything.
	if got := workCount(t, svcRef); got != 0 {
		t.Fatalf("works after the rejected mutations = %d, want 0", got)
	}

	// Reads still need only the session, not the CSRF token.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/works")
	expectSuccess(t, resp, raw)

	// A wrong method on a write route is a JSON 404 like any unknown
	// route.
	resp, raw = doJSON(t, srv, http.MethodGet, "/api/v1/works/some-work/titles", map[string]string{"Cookie": cookie.String()}, nil)
	expectFailure(t, resp, raw, http.StatusNotFound, string(app.ErrNotFound))
	resp, raw = doJSON(t, srv, http.MethodPut, "/api/v1/works", map[string]string{"Cookie": cookie.String(), CSRFHeader: csrf}, body)
	expectFailure(t, resp, raw, http.StatusNotFound, string(app.ErrNotFound))
}
