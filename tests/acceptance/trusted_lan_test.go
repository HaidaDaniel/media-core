// Trusted LAN process-level acceptance: a real release-style binary serves
// the embedded UI and the authenticated API over plain HTTP on a private
// network interface, using both the configured IP and home.arpa hostname.
// The test skips when no RFC1918 interface exists: a loopback substitute
// would not prove private-network access.
package acceptance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func lanAcceptanceIP(t *testing.T) string {
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
			if ip[0] == 10 || (ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) || (ip[0] == 192 && ip[1] == 168) {
				return ip.String()
			}
		}
	}
	t.Skip("no RFC1918 IPv4 interface available; private-network HTTP session is unverified here")
	return ""
}

func lanAcceptancePort(t *testing.T, ip string) string {
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

type lanCall struct {
	method string
	host   string
	origin string
	path   string
	cookie string
	csrf   string
	body   any
}

type lanResult struct {
	status int
	header http.Header
	data   map[string]any
	body   string
}

func callLAN(t *testing.T, ip, port string, call lanCall) lanResult {
	t.Helper()
	var body io.Reader
	if call.body != nil {
		encoded, err := json.Marshal(call.body)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(call.method, "http://"+net.JoinHostPort(ip, port)+call.path, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = call.host
	if call.origin != "" {
		req.Header.Set("Origin", call.origin)
	}
	if call.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if call.cookie != "" {
		req.Header.Set("Cookie", call.cookie)
	}
	if call.csrf != "" {
		req.Header.Set("X-CSRF-Token", call.csrf)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("%s %s (Host %s): %v", call.method, call.path, call.host, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	result := lanResult{status: resp.StatusCode, header: resp.Header, body: string(raw)}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		var env struct {
			OK    bool           `json:"ok"`
			Data  map[string]any `json:"data"`
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decode %s: %v (%s)", call.path, err, raw)
		}
		result.data = env.Data
	}
	return result
}

func waitForLAN(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Trusted LAN server did not start at %s", url)
}

func TestTrustedLANPrivateInterfaceSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("process-level HTTP smoke in long mode only")
	}
	ip := lanAcceptanceIP(t)
	port := lanAcceptancePort(t, ip)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	binary := filepath.Join(workDir, "media-core")
	build := exec.Command("go", "build", "-o", binary, "./cmd/media-core")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build media-core: %v (%s)", err, out)
	}
	stateDir := filepath.Join(workDir, "state")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "--state-dir", stateDir},
		{"owner", "generate", "--state-dir", stateDir},
	} {
		if out, err := exec.Command(binary, args...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v (%s)", args, err, out)
		}
	}
	secretBytes, err := os.ReadFile(filepath.Join(stateDir, "owner-secret"))
	if err != nil {
		t.Fatal(err)
	}
	secret := fmt.Sprintf("%x", secretBytes)

	logFile, err := os.Create(filepath.Join(workDir, "serve.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	proc := exec.Command(binary, "serve", "--state-dir", stateDir, "--trusted-lan",
		"--listen", net.JoinHostPort(ip, port), "--lan-host", "media.home.arpa")
	proc.Stdout = logFile
	proc.Stderr = logFile
	if err := proc.Start(); err != nil {
		t.Fatalf("start serve: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- proc.Wait() }()
	t.Cleanup(func() {
		if proc.Process != nil {
			_ = proc.Process.Signal(syscall.SIGTERM)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = proc.Process.Kill()
			<-done
		}
	})

	base := "http://" + net.JoinHostPort(ip, port)
	waitForLAN(t, base+"/")
	ipHost := net.JoinHostPort(ip, port)
	dnsHost := net.JoinHostPort("media.home.arpa", port)
	dnsOrigin := "http://" + dnsHost

	login := callLAN(t, ip, port, lanCall{
		method: http.MethodPost, host: dnsHost, origin: dnsOrigin,
		path: "/api/v1/session", body: map[string]string{"owner_secret": secret},
	})
	if login.status != http.StatusOK || login.data == nil {
		t.Fatalf("DNS host login: status = %d, body = %s", login.status, login.body)
	}
	csrf, _ := login.data["csrf_token"].(string)
	if csrf == "" {
		t.Fatalf("login response missing csrf_token: %s", login.body)
	}
	var cookie *http.Cookie
	for _, c := range (&http.Response{Header: login.header}).Cookies() {
		if c.Name == "mc_session" {
			cookie = c
		}
	}
	if cookie == nil || cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("Trusted LAN cookie = %+v, want HttpOnly/Strict/non-Secure", cookie)
	}

	created := callLAN(t, ip, port, lanCall{
		method: http.MethodPost, host: dnsHost, origin: dnsOrigin,
		path: "/api/v1/works", cookie: cookie.String(), csrf: csrf,
		body: map[string]any{
			"request_id": "lan-acceptance-create",
			"titles": []map[string]any{{
				"text": "Trusted LAN Acceptance", "language": "en", "kind": "original",
				"preferred": true, "provenance": "manual",
			}},
			"kind": "book", "language": "en",
		},
	})
	if created.status != http.StatusOK || created.data == nil || created.data["work_id"] == nil {
		t.Fatalf("DNS host create: status = %d, body = %s", created.status, created.body)
	}
	workID, _ := created.data["work_id"].(string)

	// The Go binary itself serves the SPA shell and a hard refresh on a
	// client route; there is no Node server or proxy in this topology.
	for _, path := range []string{"/", "/works/" + workID, "/add"} {
		page := callLAN(t, ip, port, lanCall{method: http.MethodGet, host: dnsHost, path: path})
		if page.status != http.StatusOK || !strings.Contains(page.header.Get("Content-Type"), "text/html") {
			t.Fatalf("SPA %s: status = %d, content-type = %q", path, page.status, page.header.Get("Content-Type"))
		}
	}

	// The IP spelling reads the same library; unknown Host and Origin and
	// missing session/CSRF are still refused in LAN mode.
	list := callLAN(t, ip, port, lanCall{method: http.MethodGet, host: ipHost, cookie: cookie.String(), path: "/api/v1/works"})
	if list.status != http.StatusOK {
		t.Fatalf("IP host library read: status = %d, body = %s", list.status, list.body)
	}
	for _, tc := range []lanCall{
		{method: http.MethodGet, host: net.JoinHostPort("attacker.example.com", port), path: "/api/v1/works"},
		{method: http.MethodPost, host: dnsHost, origin: "http://attacker.example.com", path: "/api/v1/session", body: map[string]string{"owner_secret": secret}},
		{method: http.MethodPost, host: dnsHost, path: "/api/v1/session", body: map[string]string{"owner_secret": secret}},
		{method: http.MethodPost, host: dnsHost, origin: dnsOrigin, path: "/api/v1/works", body: map[string]any{"request_id": "lan-acceptance-unauthorized"}},
		{method: http.MethodPost, host: dnsHost, origin: dnsOrigin, cookie: cookie.String(), path: "/api/v1/works", body: map[string]any{"request_id": "lan-acceptance-no-csrf"}},
	} {
		got := callLAN(t, ip, port, tc)
		if got.status != http.StatusForbidden && got.status != http.StatusUnauthorized {
			t.Fatalf("%s %s Host %s Origin %q: status = %d, body = %s", tc.method, tc.path, tc.host, tc.origin, got.status, got.body)
		}
	}
}
