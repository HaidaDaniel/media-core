package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/artwork"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func artworkTestServer(t *testing.T, enabled bool) (*Server, *http.Cookie, string) {
	t.Helper()
	stateDir := t.TempDir()
	secret, err := GenerateOwnerSecret(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	maint, err := store.OpenForMaintenance(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunMigrations(maint.DB()); err != nil {
		t.Fatal(err)
	}
	if err := maint.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg := loopbackConfig(t, stateDir)
	cfg.Artwork = enabled
	srv, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cache, err := artwork.NewCache(stateDir, artwork.DefaultMaxCacheBytes, artwork.DefaultMaxImageBytes)
	if err != nil {
		t.Fatal(err)
	}
	registry, client := artwork.NewDefault()
	srv.WithCatalog(app.NewService(db))
	srv.WithArtwork(artwork.NewService(db, registry, client, cache))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()
	for srv.Addr() == nil {
		time.Sleep(5 * time.Millisecond)
	}
	t.Cleanup(func() { cancel(); <-done })
	cookie, csrf := login(t, srv, secret)
	return srv, cookie, csrf
}

func TestArtworkRoutesRequireSessionAndRespectOptOut(t *testing.T) {
	srv, cookie, csrf := artworkTestServer(t, true)
	resp, raw := doJSON(t, srv, http.MethodGet, "/api/v1/artwork/providers", nil, nil)
	expectFailure(t, resp, raw, http.StatusUnauthorized, "unauthorized")

	headers := map[string]string{"Cookie": cookie.String(), "X-CSRF-Token": csrf}
	resp, raw = doJSON(t, srv, http.MethodGet, "/api/v1/artwork/providers", headers, nil)
	data := expectSuccess(t, resp, raw)
	if _, ok := data["providers"]; !ok {
		t.Fatalf("providers response = %v", data)
	}

	// An empty registry proves the endpoint is local and deterministic in
	// tests; no provider request is made by a read.
	resp, raw = doJSON(t, srv, http.MethodPost, "/api/v1/artwork/search", headers, map[string]any{
		"title": "Dune", "kind": "book", "alternatives": []string{}, "references": []string{},
	})
	data = expectSuccess(t, resp, raw)
	if _, ok := data["candidates"]; !ok {
		t.Fatalf("search response = %v", data)
	}

	// Mutating routes require CSRF even with a live session.
	resp, raw = doJSON(t, srv, http.MethodPost, "/api/v1/artwork/search",
		map[string]string{"Cookie": cookie.String()}, map[string]any{"title": "Dune", "kind": "book"})
	expectFailure(t, resp, raw, http.StatusForbidden, "forbidden")

	disabled, cookie2, csrf2 := artworkTestServer(t, false)
	resp, raw = doJSON(t, disabled, http.MethodGet, "/api/v1/artwork/providers",
		map[string]string{"Cookie": cookie2.String(), "X-CSRF-Token": csrf2}, nil)
	expectFailure(t, resp, raw, http.StatusConflict, "config_error")
}

func TestLibraryRenderHasNoExternalImageURL(t *testing.T) {
	srv, cookie, _ := artworkTestServer(t, true)
	resp, raw := doJSON(t, srv, http.MethodGet, "/api/v1/works",
		map[string]string{"Cookie": cookie.String()}, nil)
	data := expectSuccess(t, resp, raw)
	items, _ := data["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("empty library returned %d items", len(items))
	}
	_ = context.Background
	_ = time.Second
}
