package httpapi

import (
	"net/http"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// releaseEnvelope builds one Release Watcher version 1.0 envelope carrying a
// single event for the given publication id.
func releaseEnvelope(eventID, publicationID string, overrides map[string]any) map[string]any {
	ev := map[string]any{
		"schema_version": "1.0",
		"event_id":       eventID,
		"event_type":     "release.added",
		"publication_id": publicationID,
		"release_number": "1",
		"release_title":  "Chapter 1",
	}
	for k, v := range overrides {
		ev[k] = v
	}
	return map[string]any{
		"schema_version": "1.0",
		"ok":             true,
		"data":           map[string]any{"events": []map[string]any{ev}},
	}
}

// TestReleaseImport proves the bounded JSON batch adapter: a valid envelope
// stages one pending record, a canonical-equal replay is a no-op, and an
// unsupported schema major is an invalid argument that writes nothing.
func TestReleaseImport(t *testing.T) {
	srv := startCatalogServer(t, nil)
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	resp, raw := catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/imports/releases",
		releaseEnvelope("evt_http_1", "pub_http_1", nil))
	data := expectSuccess(t, resp, raw)
	if got := data["imported"].(float64); got != 1 {
		t.Fatalf("imported = %v, want 1 (body %s)", got, raw)
	}

	// Replay the same event: a no-op, reported as replayed.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/imports/releases",
		releaseEnvelope("evt_http_1", "pub_http_1", nil))
	data = expectSuccess(t, resp, raw)
	if got := data["replayed"].(float64); got != 1 {
		t.Fatalf("replayed = %v, want 1 (body %s)", got, raw)
	}

	// An unsupported schema major writes nothing and is an invalid argument.
	bad := releaseEnvelope("evt_http_2", "pub_http_1", nil)
	bad["schema_version"] = "2.0"
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/imports/releases", bad)
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// The pending queue holds exactly the one imported event (the replay and
	// the rejected major added nothing).
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/pending-imports")
	data = expectSuccess(t, resp, raw)
	if got := data["total"].(float64); got != 1 {
		t.Fatalf("pending total = %v, want 1 (body %s)", got, raw)
	}
}

// TestReleaseBinding proves the binding vertical slice over HTTP: an
// explicit bind projects the event and clears the pending queue, a rebind
// moves the projection without a duplicate, a revoke un-projects it, and a
// conflicting bind is a conflict.
func TestReleaseBinding(t *testing.T) {
	var e1ID, e2ID string
	srv := startCatalogServer(t, func(s *app.Service) {
		_, e1 := seedWork(t, s, "seed-rel-1", []store.TitleInput{titleInput("Release One", domain.TitleOriginal)}, appKind(domain.KindManga))
		e1ID = *e1
		_, e2 := seedWork(t, s, "seed-rel-2", []store.TitleInput{titleInput("Release Two", domain.TitleOriginal)}, appKind(domain.KindWebNovel))
		e2ID = *e2
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	// Import one event, then bind it to the first edition.
	catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/imports/releases",
		releaseEnvelope("evt_rb", "pub_rb", nil))
	resp, raw := catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/releases/bindings", map[string]any{
		"request_id": "bind-1", "publication_id": "pub_rb", "edition_id": e1ID,
	})
	data := expectSuccess(t, resp, raw)
	if data["edition_id"] != e1ID || data["state"] != "active" {
		t.Fatalf("bind = %v, want active on e1 (body %s)", data, raw)
	}

	// The event is no longer pending and projects for e1 only.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/pending-imports")
	data = expectSuccess(t, resp, raw)
	if got := data["total"].(float64); got != 0 {
		t.Fatalf("pending total after bind = %v, want 0 (body %s)", got, raw)
	}
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+e1ID+"/releases")
	data = expectSuccess(t, resp, raw)
	if got := data["total"].(float64); got != 1 {
		t.Fatalf("e1 releases = %v, want 1 (body %s)", got, raw)
	}
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+e2ID+"/releases")
	data = expectSuccess(t, resp, raw)
	if got := data["total"].(float64); got != 0 {
		t.Fatalf("e2 releases before rebind = %v, want 0 (body %s)", got, raw)
	}

	// A conflicting bind to a different edition is a conflict.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/releases/bindings", map[string]any{
		"request_id": "bind-conflict", "publication_id": "pub_rb", "edition_id": e2ID,
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))

	// The explicit rebind moves the projection to e2 with no duplicate.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/releases/bindings/rebind", map[string]any{
		"request_id": "rebind-1", "publication_id": "pub_rb", "edition_id": e2ID,
	})
	data = expectSuccess(t, resp, raw)
	if data["edition_id"] != e2ID || data["replaces_id"] == nil {
		t.Fatalf("rebind = %v, want e2 with a replaces link (body %s)", data, raw)
	}
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+e2ID+"/releases")
	data = expectSuccess(t, resp, raw)
	if got := data["total"].(float64); got != 1 {
		t.Fatalf("e2 releases after rebind = %v, want 1 (body %s)", got, raw)
	}
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+e1ID+"/releases")
	data = expectSuccess(t, resp, raw)
	if got := data["total"].(float64); got != 0 {
		t.Fatalf("e1 releases after rebind = %v, want 0 (no duplicate active projection, body %s)", got, raw)
	}

	// The active binding read reports the replacement.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/releases/bindings?publication=pub_rb")
	data = expectSuccess(t, resp, raw)
	active := data["active"].(map[string]any)
	if active["edition_id"] != e2ID {
		t.Fatalf("active binding = %v, want e2 (body %s)", active, raw)
	}

	// Revoking un-projects the event from every edition.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodDelete, "/api/v1/releases/bindings", map[string]any{
		"request_id": "revoke-1", "publication_id": "pub_rb",
	})
	data = expectSuccess(t, resp, raw)
	if data["status"] != "revoked" {
		t.Fatalf("revoke = %v, want revoked (body %s)", data, raw)
	}
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+e2ID+"/releases")
	data = expectSuccess(t, resp, raw)
	if got := data["total"].(float64); got != 0 {
		t.Fatalf("e2 releases after revoke = %v, want 0 (body %s)", got, raw)
	}
}
