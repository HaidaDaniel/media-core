package httpapi

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/config"
)

// startTestServer builds and serves a test server on an ephemeral port and
// stops it when the test ends.
func startTestServer(t *testing.T, cfg *config.Config) *Server {
	t.Helper()
	srv, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
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

// loopbackConfig is a loopback configuration on an ephemeral port.
func loopbackConfig(t *testing.T, stateDir string) *config.Config {
	t.Helper()
	return &config.Config{StateDir: stateDir, ListenAddr: "127.0.0.1:0"}
}

// doJSON performs one request and returns the response plus the raw body
// bytes. The Origin header, required on unsafe requests, defaults to the
// server's bound origin and may be overridden per test.
func doJSON(t *testing.T, srv *Server, method, path string, headers map[string]string, payload any) (*http.Response, []byte) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = strings.NewReader(string(b))
	}
	base := "http://" + srv.Addr().String()
	url := base + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if method != http.MethodGet && method != http.MethodHead {
		req.Header.Set("Origin", base)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("%s %s: content type = %q, want application/json", method, path, ct)
	}
	return resp, raw
}

// decodeEnvelope decodes one envelope body.
func decodeEnvelope(t *testing.T, raw []byte) app.Envelope {
	t.Helper()
	var env app.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode envelope %q: %v", raw, err)
	}
	return env
}

// expectFailure asserts a failure envelope with a status and error code.
func expectFailure(t *testing.T, resp *http.Response, raw []byte, wantStatus int, wantCode string) {
	t.Helper()
	if resp.StatusCode != wantStatus {
		t.Fatalf("status = %d, want %d (body %s)", resp.StatusCode, wantStatus, raw)
	}
	env := decodeEnvelope(t, raw)
	if env.OK {
		t.Fatalf("envelope ok = true, want failure (body %s)", raw)
	}
	if env.Error == nil || env.Error.Code != wantCode {
		t.Fatalf("error = %+v, want code %q (body %s)", env.Error, wantCode, raw)
	}
}

// expectSuccess asserts a success envelope and returns the decoded data
// (nil when the success carries no data).
func expectSuccess(t *testing.T, resp *http.Response, raw []byte) map[string]any {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, raw)
	}
	env := decodeEnvelope(t, raw)
	if !env.OK {
		t.Fatalf("envelope ok = false, want success (body %s)", raw)
	}
	if env.Data == nil {
		return nil
	}
	data, ok := (*env.Data).(map[string]any)
	if !ok {
		t.Fatalf("data is %T, want object", *env.Data)
	}
	return data
}

// sessionCookie extracts the session cookie from a response.
func sessionCookie(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == SessionCookieName {
			return c
		}
	}
	t.Fatalf("no %s cookie in response", SessionCookieName)
	return nil
}

// login performs a successful owner-secret login and returns the session
// cookie and CSRF token.
func login(t *testing.T, srv *Server, secret string) (*http.Cookie, string) {
	t.Helper()
	resp, raw := doJSON(t, srv, http.MethodPost, "/api/v1/session", nil, map[string]string{"owner_secret": secret})
	data := expectSuccess(t, resp, raw)
	csrf, _ := data["csrf_token"].(string)
	if csrf == "" {
		t.Fatalf("login data missing csrf_token: %v", data)
	}
	return sessionCookie(t, resp), csrf
}

func TestHTTPServerSessionRoundTrip(t *testing.T) {
	stateDir := t.TempDir()
	secret, err := GenerateOwnerSecret(stateDir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	srv := startTestServer(t, loopbackConfig(t, stateDir))

	cookie, csrf := login(t, srv, secret)

	// Cookie flags: HttpOnly, SameSite=Strict, Path=/, Secure off on
	// local loopback HTTP, bounded max age.
	if !cookie.HttpOnly {
		t.Error("session cookie is not HttpOnly")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("session cookie SameSite = %v, want Strict", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Errorf("session cookie Path = %q, want /", cookie.Path)
	}
	if cookie.Secure {
		t.Error("session cookie Secure on loopback HTTP, want false")
	}
	if cookie.MaxAge <= 0 || time.Duration(cookie.MaxAge)*time.Second != DefaultAbsoluteExpiry {
		t.Errorf("session cookie MaxAge = %d, want %d", cookie.MaxAge, int(DefaultAbsoluteExpiry.Seconds()))
	}

	// Authenticated session inspection returns the in-memory CSRF token.
	resp, raw := doJSON(t, srv, http.MethodGet, "/api/v1/session", map[string]string{"Cookie": cookie.String()}, nil)
	data := expectSuccess(t, resp, raw)
	if got, _ := data["csrf_token"].(string); got != csrf {
		t.Fatalf("inspection csrf = %q, want %q", got, csrf)
	}

	// Logout without the CSRF token is forbidden.
	resp, raw = doJSON(t, srv, http.MethodDelete, "/api/v1/session", map[string]string{"Cookie": cookie.String()}, nil)
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// Logout with a wrong CSRF token is forbidden as well.
	resp, raw = doJSON(t, srv, http.MethodDelete, "/api/v1/session", map[string]string{"Cookie": cookie.String(), CSRFHeader: "wrong"}, nil)
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// Logout with the CSRF token invalidates the session and clears the
	// cookie.
	resp, raw = doJSON(t, srv, http.MethodDelete, "/api/v1/session", map[string]string{"Cookie": cookie.String(), CSRFHeader: csrf}, nil)
	expectSuccess(t, resp, raw)
	cleared := sessionCookie(t, resp)
	if cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Fatalf("logout did not clear the cookie: %+v", cleared)
	}

	// The invalidated session no longer authenticates.
	resp, raw = doJSON(t, srv, http.MethodGet, "/api/v1/session", map[string]string{"Cookie": cookie.String()}, nil)
	expectFailure(t, resp, raw, http.StatusUnauthorized, string(app.ErrUnauthorized))
}

func TestHTTPServerMissingSecret(t *testing.T) {
	stateDir := t.TempDir()
	srv := startTestServer(t, loopbackConfig(t, stateDir))
	resp, raw := doJSON(t, srv, http.MethodPost, "/api/v1/session", nil, map[string]string{"owner_secret": "0000000000000000000000000000000000000000000000000000000000000000"})
	env := decodeEnvelope(t, raw)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if env.Error == nil || env.Error.Code != string(app.ErrUnauthorized) {
		t.Fatalf("error = %+v, want unauthorized", env.Error)
	}
	if !strings.Contains(env.Error.Message, "not configured") {
		t.Fatalf("missing-secret message %q must be distinguishable from a wrong secret", env.Error.Message)
	}
}

func TestHTTPServerUnauthenticated(t *testing.T) {
	stateDir := t.TempDir()
	srv := startTestServer(t, loopbackConfig(t, stateDir))

	cases := []struct {
		method string
		header map[string]string
	}{
		{http.MethodGet, nil},
		{http.MethodGet, map[string]string{"Cookie": SessionCookieName + "=stale"}},
		{http.MethodDelete, nil},
	}
	for _, tc := range cases {
		resp, raw := doJSON(t, srv, tc.method, "/api/v1/session", tc.header, nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s without a valid session: status = %d, want 401 (body %s)", tc.method, resp.StatusCode, raw)
		}
		env := decodeEnvelope(t, raw)
		if env.Error == nil || env.Error.Code != string(app.ErrUnauthorized) {
			t.Fatalf("%s without a valid session: error = %+v, want unauthorized", tc.method, env.Error)
		}
	}
}

func TestHTTPServerUnknownRoute(t *testing.T) {
	stateDir := t.TempDir()
	srv := startTestServer(t, loopbackConfig(t, stateDir))

	// Reads on library routes are 404 JSON, not HTML.
	resp, raw := doJSON(t, srv, http.MethodGet, "/api/v1/library", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /api/v1/library: status = %d, want 404", resp.StatusCode)
	}
	env := decodeEnvelope(t, raw)
	if env.Error == nil || env.Error.Code != string(app.ErrNotFound) {
		t.Fatalf("GET /api/v1/library: error = %+v, want not_found", env.Error)
	}
	// Writes on unknown routes are 404 JSON as well: there is no
	// unauthenticated writable endpoint and no 405 HTML page.
	resp, raw = doJSON(t, srv, http.MethodPut, "/api/v1/library/1", nil, map[string]string{"title": "x"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("PUT /api/v1/library/1: status = %d, want 404", resp.StatusCode)
	}
	env = decodeEnvelope(t, raw)
	if env.Error == nil || env.Error.Code != string(app.ErrNotFound) {
		t.Fatalf("PUT /api/v1/library/1: error = %+v, want not_found", env.Error)
	}
	// A wrong method on the session route is a JSON 404 too.
	resp, raw = doJSON(t, srv, http.MethodPatch, "/api/v1/session", nil, map[string]string{"a": "b"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("PATCH /api/v1/session: status = %d, want 404", resp.StatusCode)
	}
}

func TestHTTPServerRemoteBindGuard(t *testing.T) {
	stateDir := t.TempDir()
	cert, key := selfSignedTLS(t)
	cases := map[string]*config.Config{
		"non-loopback without allow-remote": {
			StateDir:   stateDir,
			ListenAddr: "0.0.0.0:18787",
		},
		"non-loopback without public origin": {
			StateDir:    stateDir,
			ListenAddr:  "0.0.0.0:18787",
			AllowRemote: true,
		},
		"non-loopback without TLS files": {
			StateDir:     stateDir,
			ListenAddr:   "0.0.0.0:18787",
			AllowRemote:  true,
			PublicOrigin: "https://nas.example.com",
		},
		"non-loopback with unreadable certificate": {
			StateDir:     stateDir,
			ListenAddr:   "0.0.0.0:18787",
			AllowRemote:  true,
			PublicOrigin: "https://nas.example.com",
			TLSCertFile:  filepath.Join(stateDir, "missing-cert.pem"),
			TLSKeyFile:   key,
		},
	}
	for name, cfg := range cases {
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: New succeeded, want failure before listen", name)
		}
	}
	// The full trio with a loadable certificate passes construction.
	cfg := &config.Config{
		StateDir:     stateDir,
		ListenAddr:   "0.0.0.0:18787",
		AllowRemote:  true,
		PublicOrigin: "https://nas.example.com",
		TLSCertFile:  cert,
		TLSKeyFile:   key,
	}
	if _, err := New(cfg); err != nil {
		t.Fatalf("valid remote configuration: New: %v", err)
	}
}

func TestHTTPServerTLSRoundTrip(t *testing.T) {
	stateDir := t.TempDir()
	secret, err := GenerateOwnerSecret(stateDir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	cert, key := selfSignedTLS(t)
	srv, err := New(&config.Config{
		StateDir:     stateDir,
		ListenAddr:   "0.0.0.0:0",
		AllowRemote:  true,
		PublicOrigin: "https://nas.example.com",
		TLSCertFile:  cert,
		TLSKeyFile:   key,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
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
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, "127.0.0.1:"+port)
			},
		},
	}

	// A wrong Host (the dial target) is rejected: only the public origin
	// host is accepted in remote mode.
	req, err := http.NewRequest(http.MethodGet, "https://nas.example.com/api/v1/session", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "127.0.0.1:" + port
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("host probe: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("remote host probe: status = %d, want 403", resp.StatusCode)
	}

	// Login through the public origin host.
	req, err = http.NewRequest(http.MethodPost, "https://nas.example.com/api/v1/session", strings.NewReader(`{"owner_secret":"`+secret+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://nas.example.com")
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	env := decodeEnvelope(t, raw)
	if resp.StatusCode != http.StatusOK || !env.OK || env.Error != nil {
		t.Fatalf("remote login: status = %d, envelope = %s", resp.StatusCode, raw)
	}
	cookie := sessionCookie(t, resp)
	if !cookie.Secure {
		t.Error("session cookie on TLS must be Secure")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("TLS session cookie flags: %+v", cookie)
	}

	// Inspect the session over TLS.
	req, err = http.NewRequest(http.MethodGet, "https://nas.example.com/api/v1/session", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Cookie", cookie.String())
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	raw, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), "csrf_token") {
		t.Fatalf("remote inspect: status = %d, body %s", resp.StatusCode, raw)
	}
}

func TestHTTPServerGracefulShutdown(t *testing.T) {
	stateDir := t.TempDir()
	srv, err := New(loopbackConfig(t, stateDir))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
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

	// Cancellation stops the server cleanly.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve after cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop within 5s of cancellation")
	}

	// The listener no longer accepts connections.
	conn, err := net.DialTimeout("tcp", addr.String(), 500*time.Millisecond)
	if err == nil {
		conn.Close()
		t.Fatal("listener still accepting after graceful shutdown")
	}
}

func TestHTTPServerRestartInvalidatesSessions(t *testing.T) {
	stateDir := t.TempDir()
	secret, err := GenerateOwnerSecret(stateDir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	first := startTestServer(t, loopbackConfig(t, stateDir))
	cookie, _ := login(t, first, secret)

	// A restart is a new in-memory session store: the old session id is
	// not active even though the owner secret is unchanged.
	second := startTestServer(t, loopbackConfig(t, stateDir))
	resp, raw := doJSON(t, second, http.MethodGet, "/api/v1/session", map[string]string{"Cookie": cookie.String()}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("stale session after restart: status = %d, want 401", resp.StatusCode)
	}
	env := decodeEnvelope(t, raw)
	if env.Error == nil || env.Error.Code != string(app.ErrUnauthorized) {
		t.Fatalf("stale session after restart: error = %+v, want unauthorized", env.Error)
	}
}

// selfSignedTLS writes a short-lived self-signed certificate and key for
// TLS tests and returns their paths.
func selfSignedTLS(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "media-core test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}
