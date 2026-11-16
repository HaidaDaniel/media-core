package httpapi

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/HaidaDaniel/media-core/internal/webassets"
)

// synthAssets is a synthetic staged build: a shell, two hashed assets and
// a non-hashed root file, enough to pin the serving contract without a
// real Vite build.
var synthAssets = fstest.MapFS{
	"index.html":          &fstest.MapFile{Data: []byte("<!doctype html><title>synth</title>")},
	"assets/app-a1.js":    &fstest.MapFile{Data: []byte("export{}")},
	"assets/style-b2.css": &fstest.MapFile{Data: []byte("body{}")},
	"robots.txt":          &fstest.MapFile{Data: []byte("User-agent: *")},
	"secret.txt":          &fstest.MapFile{Data: []byte("root:topsecret")},
}

// startAssetServer builds a server with the given assets FS attached
// before the first Serve, so the route table and the handler agree on the
// FS, and stops it when the test ends.
func startAssetServer(t *testing.T, fsys fs.FS) *Server {
	t.Helper()
	srv, err := New(loopbackConfig(t, t.TempDir()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv.WithAssets(fsys)
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

// getAsset performs one GET against the test server and returns the
// response, the (possibly decompressed) body and any failure.
func getAsset(t *testing.T, srv *Server, path string) (*http.Response, []byte) {
	t.Helper()
	resp, err := http.Get("http://" + srv.Addr().String() + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			t.Fatalf("gzip reader: %v", err)
		}
		defer gz.Close()
		reader = gz
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return resp, body
}

// TestAssetsShellServed proves the root serves the embedded shell with the
// revalidating cache policy and the full security header set.
func TestAssetsShellServed(t *testing.T) {
	srv := startAssetServer(t, synthAssets)

	resp, body := getAsset(t, srv, "/")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /: status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != cacheNoCache {
		t.Fatalf("Cache-Control = %q, want %q", cc, cacheNoCache)
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Fatalf("CSP = %q, want the self-only policy", csp)
	}
	if !strings.Contains(string(body), "synth") {
		t.Fatalf("body = %q, want the synthetic shell", body)
	}
}

// TestAssetsCachePolicies pins the split: hashed assets under assets/ are
// immutable, everything else revalidates.
func TestAssetsCachePolicies(t *testing.T) {
	srv := startAssetServer(t, synthAssets)

	if resp, _ := getAsset(t, srv, "/assets/app-a1.js"); resp.Header.Get("Cache-Control") != cacheImmutable {
		t.Fatalf("hashed asset Cache-Control = %q, want %q", resp.Header.Get("Cache-Control"), cacheImmutable)
	}
	if _, body := getAsset(t, srv, "/assets/app-a1.js"); string(body) != "export{}" {
		t.Fatalf("asset body = %q", body)
	}
	if resp, _ := getAsset(t, srv, "/robots.txt"); resp.Header.Get("Cache-Control") != cacheNoCache {
		t.Fatalf("root file Cache-Control = %q, want %q", resp.Header.Get("Cache-Control"), cacheNoCache)
	}
	if resp, _ := getAsset(t, srv, "/"); resp.Header.Get("Cache-Control") != cacheNoCache {
		t.Fatalf("shell Cache-Control = %q, want %q", resp.Header.Get("Cache-Control"), cacheNoCache)
	}
}

// TestAssetsSPAFallback proves client-side routes fall back to the shell
// while real files keep their own content.
func TestAssetsSPAFallback(t *testing.T) {
	srv := startAssetServer(t, synthAssets)

	for _, p := range []string{"/library", "/works/abc-123", "/library?filter=d"} {
		resp, body := getAsset(t, srv, p)
		if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "synth") {
			t.Fatalf("GET %s: status=%d body=%q, want the shell", p, resp.StatusCode, body)
		}
	}
}

// TestAssetsAPIMissIsJSON proves an unregistered API path is the JSON 404
// wire error, never index.html.
func TestAssetsAPIMissIsJSON(t *testing.T) {
	srv := startAssetServer(t, synthAssets)

	resp, body := getAsset(t, srv, "/api/v9/nope")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /api/v9/nope: status = %d, want 404", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	if strings.Contains(string(body), "synth") || strings.Contains(string(body), "<") {
		t.Fatalf("the API miss leaked the shell: %q", body)
	}
	var env struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.OK {
		t.Fatalf("the API miss is not the JSON error envelope: %q", body)
	}
}

// TestAssetsTraversalAndListing proves the handler never serves a dot-
// segment path and never lists a directory. The standard transport redirect
// for a literal ".." request is asserted separately with redirect-following
// disabled: it always lands on the cleaned in-webroot path, so it cannot
// reach outside the staged build.
func TestAssetsTraversalAndListing(t *testing.T) {
	srv := startAssetServer(t, synthAssets)

	for _, p := range []string{"/assets/", "/%2e%2e/secret.txt", "/%2e%2e/%2e%2e/etc/passwd"} {
		resp, body := getAsset(t, srv, p)
		if strings.Contains(string(body), "topsecret") || strings.Contains(string(body), "root:") {
			t.Fatalf("GET %s leaked a file through the handler: %q", p, body)
		}
		if strings.Contains(string(body), "secret.txt") || strings.Contains(string(body), "app-a1.js") {
			t.Fatalf("GET %s produced a directory listing: %q", p, body)
		}
		if resp.StatusCode == http.StatusInternalServerError {
			t.Fatalf("GET %s: status 500, want a clean miss or the shell", p)
		}
	}

	// A literal dot-segment request must not be served directly; the
	// transport may redirect to the cleaned path, which stays in-webroot.
	// (The mux uses a 307 temporary redirect for the cleaned path.)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get("http://" + srv.Addr().String() + "/../secret.txt")
	if err != nil {
		t.Fatalf("GET /../secret.txt: %v", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusMovedPermanently, http.StatusTemporaryRedirect, http.StatusBadRequest, http.StatusNotFound:
	default:
		t.Fatalf("GET /../secret.txt: status = %d, want a redirect/400/404 (never a direct serve)", resp.StatusCode)
	}
}

// TestAssetsUnsafeMethodsKeepJSONContract proves a non-GET/HEAD request on
// an unregistered path keeps the JSON 404 the old catch-all served.
func TestAssetsUnsafeMethodsKeepJSONContract(t *testing.T) {
	srv := startAssetServer(t, synthAssets)

	req, err := http.NewRequest(http.MethodPut, "http://"+srv.Addr().String()+"/library", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Origin", "http://"+srv.Addr().String())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /library: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("PUT /library: status = %d, want 404", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("PUT /library Content-Type = %q, want application/json", ct)
	}
	var env struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.OK {
		t.Fatalf("the unsafe-method miss is not the JSON error envelope: %q", body)
	}
}

// TestAssetsServesEmbeddedPlaceholder proves the embedded staging is what
// the runtime attaches: a server built over webassets.FS answers the root
// with the shell it embeds, so a binary always serves something coherent.
func TestAssetsServesEmbeddedPlaceholder(t *testing.T) {
	srv := startAssetServer(t, webassets.FS)
	resp, body := getAsset(t, srv, "/")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /: status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(body), "Media Core") {
		t.Fatalf("the embedded shell is missing: %q", body)
	}
}
