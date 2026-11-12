package httpapi

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
)

func TestSecurityHostRejected(t *testing.T) {
	stateDir := t.TempDir()
	secret, err := GenerateOwnerSecret(stateDir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	srv := startTestServer(t, loopbackConfig(t, stateDir))
	_ = secret

	// A hostile Host on a safe request is rejected before routing.
	resp, raw := hostProbe(t, srv, http.MethodGet)
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// A hostile Host on an unsafe request is rejected as well.
	resp, raw = hostProbe(t, srv, http.MethodPost)
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))
}

// hostProbe sends one request with a hostile Host header, which the Go
// client only honors through Request.Host, not through the header map.
func hostProbe(t *testing.T, srv *Server, method string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, "http://"+srv.Addr().String()+"/api/v1/session", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "attacker.example.com"
	if method == http.MethodPost {
		req.Header.Set("Origin", "http://"+srv.Addr().String())
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("host probe: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, raw
}

func TestSecurityOriginMissingNotBypass(t *testing.T) {
	stateDir := t.TempDir()
	secret, err := GenerateOwnerSecret(stateDir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	srv := startTestServer(t, loopbackConfig(t, stateDir))

	// The invariant: an unsafe request with no Origin header must not be
	// treated as a non-browser request and let through. Login and logout
	// are both refused without an exact Origin.
	req, err := http.NewRequest(http.MethodPost, "http://"+srv.Addr().String()+"/api/v1/session", strings.NewReader(`{"owner_secret":"`+secret+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("login without Origin: status = %d, want 403 (body %s)", resp.StatusCode, raw)
	}
	env := decodeEnvelope(t, raw)
	if env.Error == nil || env.Error.Code != string(app.ErrForbidden) {
		t.Fatalf("login without Origin: error = %+v, want forbidden", env.Error)
	}

	// Same for logout on an unknown session: Origin is checked before
	// authentication, so it cannot be skipped by presenting no Origin.
	req, err = http.NewRequest(http.MethodDelete, "http://"+srv.Addr().String()+"/api/v1/session", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("logout without Origin: status = %d, want 403 (body %s)", resp.StatusCode, raw)
	}
}

func TestSecurityOriginMismatch(t *testing.T) {
	stateDir := t.TempDir()
	srv := startTestServer(t, loopbackConfig(t, stateDir))

	// A cross-site Origin is rejected even with a well-formed body.
	resp, raw := doJSON(t, srv, http.MethodPost, "/api/v1/session", map[string]string{"Origin": "http://evil.example.com"}, map[string]string{"owner_secret": "x"})
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// The https variant of the loopback origin is not the loopback origin.
	resp, raw = doJSON(t, srv, http.MethodPost, "/api/v1/session", map[string]string{"Origin": "https://" + srv.Addr().String()}, map[string]string{"owner_secret": "x"})
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))
}

func TestSecurityResponseHeaders(t *testing.T) {
	stateDir := t.TempDir()
	srv := startTestServer(t, loopbackConfig(t, stateDir))

	// Headers are set on every response, including failures.
	resp, _ := doJSON(t, srv, http.MethodGet, "/api/v1/library", nil, nil)
	h := resp.Header
	csp := h.Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") ||
		!strings.Contains(csp, "object-src 'none'") ||
		!strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP = %q, want self-only with object-src none and frame-ancestors none", csp)
	}
	if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
		t.Errorf("CSP = %q must not allow inline or eval", csp)
	}
	if h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", h.Get("X-Content-Type-Options"))
	}
	if h.Get("X-Frame-Options") != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", h.Get("X-Frame-Options"))
	}
	if h.Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("Referrer-Policy = %q, want no-referrer", h.Get("Referrer-Policy"))
	}
	if got := h.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want none (no permissive CORS)", got)
	}
}

func TestSecurityBodyLimit(t *testing.T) {
	stateDir := t.TempDir()
	secret, err := GenerateOwnerSecret(stateDir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	srv := startTestServer(t, loopbackConfig(t, stateDir))
	_ = secret

	// A body just over the 1 MiB limit is rejected with invalid_argument
	// before the handler processes it.
	oversized := map[string]string{"owner_secret": strings.Repeat("a", MaxRequestBody)}
	resp, raw := doJSON(t, srv, http.MethodPost, "/api/v1/session", nil, oversized)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized body: status = %d, want 400 (body %s)", resp.StatusCode, raw)
	}
	env := decodeEnvelope(t, raw)
	if env.Error == nil || env.Error.Code != string(app.ErrInvalidArgument) {
		t.Fatalf("oversized body: error = %+v, want invalid_argument", env.Error)
	}

	// A small valid body still passes the limit.
	resp, raw = doJSON(t, srv, http.MethodPost, "/api/v1/session", nil, map[string]string{"owner_secret": "wrong-secret"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("small body: status = %d, want 401 (body %s)", resp.StatusCode, raw)
	}
}

func TestSecurityCSRFRequiredOnMutations(t *testing.T) {
	stateDir := t.TempDir()
	secret, err := GenerateOwnerSecret(stateDir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	srv := startTestServer(t, loopbackConfig(t, stateDir))
	cookie, csrf := login(t, srv, secret)

	// An authenticated mutation without the CSRF header is forbidden.
	resp, raw := doJSON(t, srv, http.MethodDelete, "/api/v1/session", map[string]string{"Cookie": cookie.String()}, nil)
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// A mismatched CSRF token is forbidden.
	resp, raw = doJSON(t, srv, http.MethodDelete, "/api/v1/session", map[string]string{"Cookie": cookie.String(), CSRFHeader: strings.Repeat("0", len(csrf))}, nil)
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// The matching CSRF token succeeds and the session is invalidated.
	resp, raw = doJSON(t, srv, http.MethodDelete, "/api/v1/session", map[string]string{"Cookie": cookie.String(), CSRFHeader: csrf}, nil)
	expectSuccess(t, resp, raw)
}
