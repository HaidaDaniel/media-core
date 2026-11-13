package httpapi

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/config"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// startCatalogServer serves an initialized state directory with the catalog
// routes attached; seed (when non-nil) populates the library through the
// shared service before the server starts.
func startCatalogServer(t *testing.T, seed func(s *app.Service)) *Server {
	t.Helper()
	dir := t.TempDir()
	if _, err := GenerateOwnerSecret(dir); err != nil {
		t.Fatalf("generate owner secret: %v", err)
	}
	maint, err := store.OpenForMaintenance(dir)
	if err != nil {
		t.Fatalf("OpenForMaintenance: %v", err)
	}
	if _, err := store.RunMigrations(maint.DB()); err != nil {
		maint.Close()
		t.Fatalf("RunMigrations: %v", err)
	}
	if err := maint.Close(); err != nil {
		t.Fatalf("Close maintenance: %v", err)
	}
	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	svc := app.NewService(db)
	if seed != nil {
		seed(svc)
	}
	srv, err := New(&config.Config{StateDir: dir, ListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv.WithCatalog(svc)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()
	var addr net.Addr
	for addr == nil {
		select {
		case err := <-done:
			t.Fatalf("Serve exited early: %v", err)
		case <-time.After(5 * time.Millisecond):
			addr = srv.Addr()
		}
	}
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return srv
}

// testActor is the actor used by seeded mutations.
var testActor = app.Actor{Principal: "local", Origin: domain.OriginUser}

// seedWork creates one work (with an optional first-edition kind) through
// the shared service and returns its id and, when kind is set, the
// edition id.
func seedWork(t *testing.T, s *app.Service, requestID string, titles []store.TitleInput, kind *domain.EditionKind) (string, *string) {
	t.Helper()
	res, err := s.CreateLibraryEntry(context.Background(), app.AdditionRequest{
		MutationBase: app.MutationBase{RequestID: requestID, Actor: testActor},
		Titles:       titles,
		Kind:         kind,
	})
	if err != nil {
		t.Fatalf("seed %s: %v", requestID, err)
	}
	return res.WorkID, res.EditionID
}

func titleInput(text string, kind domain.TitleKind) store.TitleInput {
	return store.TitleInput{Text: text, Kind: kind, Provenance: "manual"}
}

// catalogGet performs an authenticated GET on a catalog route with a live
// session cookie.
func catalogGet(t *testing.T, srv *Server, cookie *http.Cookie, path string) (*http.Response, []byte) {
	t.Helper()
	return doJSON(t, srv, http.MethodGet, path, map[string]string{"Cookie": cookie.String()}, nil)
}

func TestHTTPLibraryList(t *testing.T) {
	var svcRef *app.Service
	srv := startCatalogServer(t, func(s *app.Service) {
		svcRef = s
		seedWork(t, s, "seed-a", []store.TitleInput{titleInput("Alpha Storm", domain.TitleOriginal)}, appKind(domain.KindBook))
		seedWork(t, s, "seed-b", []store.TitleInput{titleInput("Beta Tide", domain.TitleOriginal)}, appKind(domain.KindManga))
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, _ := login(t, srv, secret)

	resp, raw := catalogGet(t, srv, cookie, "/api/v1/works")
	data := expectSuccess(t, resp, raw)
	items, ok := data["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("items = %v, want 2 work cards", data["items"])
	}
	if got, _ := data["total"].(float64); got != 2 {
		t.Fatalf("total = %v, want 2", data["total"])
	}
	if data["next_cursor"] != nil {
		t.Fatalf("next_cursor = %v, want null for a single page", data["next_cursor"])
	}

	// Acceptance: the HTTP payload carries the same catalog IDs and
	// current fields as a direct shared-service call.
	svcPayload, err := svcRef.SearchLibrary(context.Background(), app.LibraryFilter{})
	if err != nil {
		t.Fatalf("service SearchLibrary: %v", err)
	}
	for i, item := range items {
		card, ok := svcPayload.Items[i].(app.WorkCard)
		if !ok {
			t.Fatalf("service item %d is %T, want WorkCard", i, svcPayload.Items[i])
		}
		want, got := semanticJSON(t, card), semanticJSON(t, item)
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("item %d differs from the service result:\nhttp   %v\nservice %v", i, got, want)
		}
	}
}

// appKind lifts a domain kind to the pointer form the addition request uses.
func appKind(k domain.EditionKind) *domain.EditionKind { return &k }

// semanticJSON round-trips a value through JSON so struct and decoded-map
// forms compare by content, not representation.
func semanticJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %v: %v", v, err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal %s: %v", b, err)
	}
	return m
}

func TestHTTPLibraryListAliasesAndFilters(t *testing.T) {
	srv := startCatalogServer(t, func(s *app.Service) {
		// Work A has an original and an alias title; work B has a
		// different kind.
		seedWork(t, s, "seed-a",
			[]store.TitleInput{
				titleInput("Neon Garden", domain.TitleOriginal),
				titleInput("Neon Garden: Blooms", domain.TitleAlias),
			}, appKind(domain.KindBook))
		seedWork(t, s, "seed-b", []store.TitleInput{titleInput("Static Bloom", domain.TitleOriginal)}, appKind(domain.KindManga))
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, _ := login(t, srv, secret)

	check := func(path string, wantTitles []string) {
		t.Helper()
		resp, raw := catalogGet(t, srv, cookie, path)
		data := expectSuccess(t, resp, raw)
		items, _ := data["items"].([]any)
		var titles []string
		for _, it := range items {
			card, ok := it.(map[string]any)
			if !ok {
				t.Fatalf("item is %T, want object", it)
			}
			for _, tt := range card["titles"].([]any) {
				title, _ := tt.(map[string]any)
				titles = append(titles, title["text"].(string))
			}
		}
		if len(titles) != len(wantTitles) {
			t.Fatalf("%s: titles = %v, want %v", path, titles, wantTitles)
		}
		for i := range titles {
			if titles[i] != wantTitles[i] {
				t.Fatalf("%s: titles = %v, want %v", path, titles, wantTitles)
			}
		}
	}

	// The alias title matches its work.
	check("/api/v1/works?query=blooms", []string{"Neon Garden", "Neon Garden: Blooms"})
	// Kind filters select by the edition's kind.
	check("/api/v1/works?kind=manga", []string{"Static Bloom"})
	check("/api/v1/works?kind=book", []string{"Neon Garden", "Neon Garden: Blooms"})
	// An unknown kind is an invalid argument from the shared service.
	resp, raw := catalogGet(t, srv, cookie, "/api/v1/works?kind=comic")
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
}

func TestHTTPLibraryCursor(t *testing.T) {
	srv := startCatalogServer(t, func(s *app.Service) {
		seedWork(t, s, "seed-1", []store.TitleInput{titleInput("AAA One", domain.TitleOriginal)}, nil)
		seedWork(t, s, "seed-2", []store.TitleInput{titleInput("BBB Two", domain.TitleOriginal)}, nil)
		seedWork(t, s, "seed-3", []store.TitleInput{titleInput("CCC Three", domain.TitleOriginal)}, nil)
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, _ := login(t, srv, secret)

	// Page 1 of 1-per-page.
	resp, raw := catalogGet(t, srv, cookie, "/api/v1/works?limit=1")
	data := expectSuccess(t, resp, raw)
	items, _ := data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("page 1 items = %d, want 1", len(items))
	}
	if got, _ := data["total"].(float64); got != 3 {
		t.Fatalf("page 1 total = %v, want 3", data["total"])
	}
	cursor, ok := data["next_cursor"].(string)
	if !ok || cursor == "" {
		t.Fatalf("page 1 next_cursor = %v, want a cursor", data["next_cursor"])
	}

	// Page 2 follows the cursor.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/works?limit=1&cursor="+cursor)
	data = expectSuccess(t, resp, raw)
	items, _ = data["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("page 2 items = %d, want 1", len(items))
	}
	card := items[0].(map[string]any)
	if card["titles"].([]any)[0].(map[string]any)["text"] != "BBB Two" {
		t.Fatalf("page 2 = %v, want BBB Two", card["titles"])
	}

	// A cursor issued for a different sort is rejected.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/works?limit=1&sort=created_at&cursor="+cursor)
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
	// A non-numeric numeric parameter is an invalid argument, never a
	// silent default.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/works?limit=abc")
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/works?min-rating=high")
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
	// A malformed cursor is rejected.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/works?cursor=garbage")
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
	// A limit outside the bound is rejected.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/works?limit=1000")
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
}

func TestHTTPLibraryDetail(t *testing.T) {
	srv := startCatalogServer(t, func(s *app.Service) {
		seedWork(t, s, "seed-a",
			[]store.TitleInput{titleInput("Alpha Storm", domain.TitleOriginal)}, appKind(domain.KindBook))
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, _ := login(t, srv, secret)

	// Discover the seeded ids through the list route.
	resp, raw := catalogGet(t, srv, cookie, "/api/v1/works")
	data := expectSuccess(t, resp, raw)
	items, _ := data["items"].([]any)
	workID := items[0].(map[string]any)["work_id"].(string)
	editions := items[0].(map[string]any)["editions"].([]any)
	editionID := editions[0].(map[string]any)["id"].(string)

	// Work detail carries the same id and its editions.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/works/"+workID)
	data = expectSuccess(t, resp, raw)
	if data["id"] != workID {
		t.Fatalf("work detail id = %v, want %s", data["id"], workID)
	}
	if ed, _ := data["editions"].([]any); len(ed) != 1 {
		t.Fatalf("work detail editions = %v, want 1", data["editions"])
	}

	// Edition detail carries the same id and its work.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+editionID)
	data = expectSuccess(t, resp, raw)
	if data["id"] != editionID || data["work_id"] != workID {
		t.Fatalf("edition detail id/work_id = %v/%v, want %s/%s", data["id"], data["work_id"], editionID, workID)
	}

	// Unknown ids are JSON 404s with the stable code.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/works/does-not-exist")
	expectFailure(t, resp, raw, http.StatusNotFound, string(app.ErrNotFound))
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/does-not-exist")
	expectFailure(t, resp, raw, http.StatusNotFound, string(app.ErrNotFound))

	// A wrong method on a known path is a JSON 404, never a 405 page.
	resp, raw = doJSON(t, srv, http.MethodPost, "/api/v1/works/"+workID, map[string]string{"Cookie": cookie.String()}, map[string]string{"title": "x"})
	expectFailure(t, resp, raw, http.StatusNotFound, string(app.ErrNotFound))
}

func TestHTTPLibraryAuth(t *testing.T) {
	srv := startCatalogServer(t, func(s *app.Service) {
		seedWork(t, s, "seed-a", []store.TitleInput{titleInput("Alpha Storm", domain.TitleOriginal)}, nil)
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, _ := login(t, srv, secret)

	// No cookie: unauthorized.
	resp, raw := doJSON(t, srv, http.MethodGet, "/api/v1/works", nil, nil)
	expectFailure(t, resp, raw, http.StatusUnauthorized, string(app.ErrUnauthorized))

	// A stale cookie: unauthorized.
	resp, raw = doJSON(t, srv, http.MethodGet, "/api/v1/works", map[string]string{"Cookie": SessionCookieName + "=stale"}, nil)
	expectFailure(t, resp, raw, http.StatusUnauthorized, string(app.ErrUnauthorized))

	// A live session: allowed.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/works")
	expectSuccess(t, resp, raw)
}

func TestHTTPLibraryNoCatalog(t *testing.T) {
	// A session-only server (no catalog attached) has no catalog routes:
	// the path is an unknown route, a JSON 404, even for a live session.
	dir := t.TempDir()
	if _, err := GenerateOwnerSecret(dir); err != nil {
		t.Fatalf("generate: %v", err)
	}
	srv := startTestServer(t, loopbackConfig(t, dir))
	secret, _ := ReadOwnerSecretHex(dir)
	cookie, _ := login(t, srv, secret)
	resp, raw := doJSON(t, srv, http.MethodGet, "/api/v1/works", map[string]string{"Cookie": cookie.String()}, nil)
	expectFailure(t, resp, raw, http.StatusNotFound, string(app.ErrNotFound))
}
