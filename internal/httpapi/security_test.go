package httpapi

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/config"
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

// privateLANIP returns one up RFC1918 IPv4 interface address. The test that
// uses it skips when the host has no private interface: a loopback substitute
// would not exercise a real private-network HTTP session.
func privateLANIP(t *testing.T) string {
	t.Helper()
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("list interfaces: %v", err)
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
				continue
			}
			private := ip[0] == 10 || (ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) || (ip[0] == 192 && ip[1] == 168)
			if private {
				return ip.String()
			}
		}
	}
	t.Skip("no RFC1918 IPv4 interface available; private-network HTTP session is unverified here")
	return ""
}

// freePortOnIP reserves an ephemeral port on one interface and releases it
// so a trusted LAN server can be started there with an explicit port.
func freePortOnIP(t *testing.T, ip string) string {
	t.Helper()
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		t.Fatalf("reserve port on %s: %v", ip, err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

// lanRequest sends one request to the bound private listener while
// overriding the Host header, so the exact configured spellings are tested.
func lanRequest(t *testing.T, srv *Server, method, host, origin string, headers map[string]string, payload any) (*http.Response, []byte) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = strings.NewReader(string(b))
	}
	req, err := http.NewRequest(method, "http://"+srv.Addr().String()+"/api/v1/session", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("%s host %q: %v", method, host, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, raw
}

func TestTrustedLANPrivateInterfaceHTTPSession(t *testing.T) {
	stateDir := t.TempDir()
	secret, err := GenerateOwnerSecret(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	ip := privateLANIP(t)
	srv := startTestServer(t, &config.Config{
		StateDir:   stateDir,
		ListenAddr: net.JoinHostPort(ip, freePortOnIP(t, ip)),
		TrustedLAN: true,
		LANHost:    "media.home.arpa",
	})
	_, port, err := net.SplitHostPort(srv.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	ipHost := net.JoinHostPort(ip, port)
	dnsHost := net.JoinHostPort("media.home.arpa", port)
	foreignHost := net.JoinHostPort("attacker.example.com", port)
	dnsOrigin := "http://" + dnsHost

	// Both configured spellings work on a real private interface; a foreign
	// Host is refused even without any Origin requirement.
	for _, host := range []string{ipHost, dnsHost} {
		resp, raw := lanRequest(t, srv, http.MethodGet, host, "", nil, nil)
		if resp.StatusCode == http.StatusForbidden {
			t.Fatalf("GET Host %q: forbidden (body %s)", host, raw)
		}
		expectFailure(t, resp, raw, http.StatusUnauthorized, string(app.ErrUnauthorized))
	}
	resp, raw := lanRequest(t, srv, http.MethodGet, foreignHost, "", nil, nil)
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// Login through the DNS Host with the exact http Origin succeeds; the
	// cookie is not Secure because HTTP is not a secure context.
	resp, raw = lanRequest(t, srv, http.MethodPost, dnsHost, dnsOrigin, nil, map[string]string{"owner_secret": secret})
	data := expectSuccess(t, resp, raw)
	csrf, _ := data["csrf_token"].(string)
	if csrf == "" {
		t.Fatalf("login data missing csrf_token: %v", data)
	}
	cookie := sessionCookie(t, resp)
	if cookie.Secure {
		t.Error("trusted LAN HTTP session cookie must not be Secure")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("trusted LAN cookie flags = %+v", cookie)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("trusted LAN response has CORS header %q", got)
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("trusted LAN response CSP = %q", csp)
	}

	// Foreign and missing Origins are refused before authentication.
	resp, raw = lanRequest(t, srv, http.MethodPost, dnsHost, "http://attacker.example.com", nil, map[string]string{"owner_secret": secret})
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))
	resp, raw = lanRequest(t, srv, http.MethodPost, dnsHost, "", nil, map[string]string{"owner_secret": secret})
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))
	resp, raw = lanRequest(t, srv, http.MethodPost, foreignHost, "http://"+foreignHost, nil, map[string]string{"owner_secret": secret})
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// CSRF remains mandatory on the authenticated mutation.
	resp, raw = lanRequest(t, srv, http.MethodDelete, dnsHost, dnsOrigin, map[string]string{"Cookie": cookie.String()}, nil)
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))
	resp, raw = lanRequest(t, srv, http.MethodDelete, dnsHost, dnsOrigin, map[string]string{"Cookie": cookie.String(), CSRFHeader: csrf}, nil)
	expectSuccess(t, resp, raw)
}

func TestTrustedLANServerRejectsUnsafeConfiguration(t *testing.T) {
	stateDir := t.TempDir()
	cases := map[string]*config.Config{
		"loopback": {
			StateDir: stateDir, ListenAddr: "127.0.0.1:0", TrustedLAN: true, LANHost: "media.home.arpa",
		},
		"wildcard": {
			StateDir: stateDir, ListenAddr: "0.0.0.0:8787", TrustedLAN: true, LANHost: "media.home.arpa",
		},
		"bad host": {
			StateDir: stateDir, ListenAddr: "192.168.1.100:8787", TrustedLAN: true, LANHost: "http://media.home.arpa",
		},
		"remote trio": {
			StateDir: stateDir, ListenAddr: "192.168.1.100:8787", TrustedLAN: true,
			LANHost: "media.home.arpa", AllowRemote: true, PublicOrigin: "https://media.example.org",
		},
	}
	for name, cfg := range cases {
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: New succeeded, want failure before listen", name)
		}
	}
}
