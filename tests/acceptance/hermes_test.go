package acceptance

// TestHermes* verify the any-agent connection contract: the built binary
// serves MCP the way an external client (Hermes, Claude, opencode, or a
// plain JSON-RPC client) would consume it — over stdio on the same host
// and, opt-in, over authenticated Streamable HTTP. The stdio path needs
// no HTTP, no Node, and no provider; the HTTP path authenticates with the
// state directory's owner secret and reuses the same in-process service.

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- process helpers (same conventions as tests/packaging) ---

func hermesRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, "..", "..")
}

// hermesNodelessEnv is the child environment: the default environment
// minus PATH and Node variables, with a PATH that has no Node
// installation, so a binary that shelled out to Node would fail loudly.
func hermesNodelessEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		eq := strings.Index(kv, "=")
		if eq < 0 {
			continue
		}
		key := kv[:eq]
		if key == "PATH" || strings.HasPrefix(key, "NODE") || strings.HasPrefix(key, "npm_config_") || strings.HasPrefix(key, "pnpm_") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
}

func hermesBuild(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "media-core")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/media-core")
	cmd.Dir = hermesRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func hermesFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func hermesRunCLI(t *testing.T, bin string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = hermesRoot(t)
	cmd.Env = hermesNodelessEnv()
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return code, out.String()
}

func hermesMustRunCLI(t *testing.T, bin string, args ...string) string {
	t.Helper()
	code, out := hermesRunCLI(t, bin, args...)
	if code != 0 {
		t.Fatalf("%v: exit=%d out=%q", args, code, out)
	}
	return strings.TrimSpace(out)
}

// hermesWaitTCP dials addr until it accepts, so the test talks to a live
// listener instead of racing the process start.
func hermesWaitTCP(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no listener on %s within 30s", addr)
}

// --- stdio session: one external MCP client over the process pipes ---

type hermesStdio struct {
	t        *testing.T
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	scanner  *bufio.Scanner
	diag     *bytes.Buffer
	nextID   int
	waited   bool
	deadline time.Duration
}

func hermesOpenStdio(t *testing.T, bin, state string) *hermesStdio {
	t.Helper()
	cmd := exec.Command(bin, "mcp", "--state-dir", state)
	cmd.Env = hermesNodelessEnv()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	var diag bytes.Buffer
	cmd.Stderr = &diag
	if err := cmd.Start(); err != nil {
		t.Fatalf("start mcp: %v", err)
	}
	s := &hermesStdio{t: t, cmd: cmd, stdin: stdin, diag: &diag, deadline: 30 * time.Second}
	s.scanner = bufio.NewScanner(stdout)
	s.scanner.Buffer(make([]byte, 0, 1<<20), 16<<20)
	t.Cleanup(func() {
		_ = stdin.Close()
		if !s.waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	return s
}

// call sends one JSON-RPC request and returns its result field, skipping
// frames that do not answer this id.
func (s *hermesStdio) call(method string, params map[string]any) json.RawMessage {
	s.t.Helper()
	s.nextID++
	id := s.nextID
	frame := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		frame["params"] = params
	}
	raw, _ := json.Marshal(frame)
	if _, err := s.stdin.Write(append(raw, '\n')); err != nil {
		s.t.Fatalf("write %s: %v", method, err)
	}
	deadline := time.Now().Add(s.deadline)
	for time.Now().Before(deadline) {
		if !s.scanner.Scan() {
			s.t.Fatalf("stdio closed while awaiting %s; diagnostics: %s", method, s.diag.String())
		}
		var msg struct {
			ID     any             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(s.scanner.Bytes(), &msg); err != nil {
			continue
		}
		if got, _ := msg.ID.(float64); got == float64(id) {
			if msg.Error != nil {
				s.t.Fatalf("mcp protocol error on %s: %s", method, msg.Error)
			}
			return msg.Result
		}
	}
	s.t.Fatalf("no response to %s within %s; diagnostics: %s", method, s.deadline, s.diag.String())
	return nil
}

// notify sends a JSON-RPC notification (no id, no response).
func (s *hermesStdio) notify(method string) {
	s.t.Helper()
	raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method})
	if _, err := s.stdin.Write(append(raw, '\n')); err != nil {
		s.t.Fatalf("write %s: %v", method, err)
	}
}

// tool calls one tool and returns the single text content as raw JSON.
func (s *hermesStdio) tool(name string, args map[string]any) json.RawMessage {
	s.t.Helper()
	res := s.call("tools/call", map[string]any{"name": name, "arguments": args})
	var out struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		s.t.Fatalf("malformed tool result for %s: %s", name, res)
	}
	if len(out.Content) != 1 {
		s.t.Fatalf("tool %s: expected one content item, got %d", name, len(out.Content))
	}
	if out.IsError {
		s.t.Fatalf("tool %s failed: %s", name, out.Content[0].Text)
	}
	return json.RawMessage(out.Content[0].Text)
}

// kill terminates the process hard and proves the exit was not clean:
// the state must survive this and reopen on the next process.
func (s *hermesStdio) kill() {
	s.t.Helper()
	if err := s.cmd.Process.Kill(); err != nil {
		s.t.Fatalf("kill: %v", err)
	}
	if err := s.cmd.Wait(); err == nil {
		s.t.Fatalf("killed process reported a clean exit")
	}
	s.waited = true
}

// closeClean closes the client stdin and requires a clean exit: a clean
// EOF ends the MCP session without error.
func (s *hermesStdio) closeClean() {
	s.t.Helper()
	_ = s.stdin.Close()
	if err := s.cmd.Wait(); err != nil {
		s.t.Fatalf("clean EOF must exit without error, got %v; diagnostics: %s", err, s.diag.String())
	}
	s.waited = true
}

func hermesHandshake(t *testing.T, s *hermesStdio) {
	t.Helper()
	res := s.call("initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "hermes-acceptance", "version": "0"},
	})
	if !strings.Contains(string(res), "media-core") {
		t.Fatalf("initialize is not the media-core server: %s", res)
	}
	s.notify("notifications/initialized")
}

func hermesToolNames(t *testing.T, s *hermesStdio) map[string]bool {
	t.Helper()
	res := s.call("tools/list", nil)
	var out struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(res, &out); err != nil || len(out.Tools) == 0 {
		t.Fatalf("tools/list is empty or malformed: %s", res)
	}
	names := map[string]bool{}
	for _, tool := range out.Tools {
		names[tool.Name] = true
	}
	return names
}

func hermesActor() map[string]any {
	return map[string]any{"principal": "hermes", "origin": "agent"}
}

func hermesCreateEntryArgs() map[string]any {
	return map[string]any{
		"request_id": "hermes-req-1",
		"actor":      hermesActor(),
		"kind":       "anime",
		"titles": []map[string]any{
			{"text": "Мусиши", "kind": "original", "language": "ja", "preferred": true, "provenance": "user"},
			{"text": "Mushishi", "kind": "alias", "language": "en", "provenance": "user"},
		},
		// A known identity creates the edition as well as the work, so
		// the progress step has something to point at.
		"references": []map[string]any{{"url": "https://myanimelist.net/anime/199/Mushishi"}},
	}
}

// hermesStdioLifecycle drives the full connection lifecycle over stdio in
// one session: create, search by the romaji alias, idempotent replay,
// progress, history.
func hermesStdioLifecycle(t *testing.T, s *hermesStdio) (workID string) {
	t.Helper()
	hermesHandshake(t, s)
	names := hermesToolNames(t, s)
	for _, want := range []string{"create_entry", "search_library", "record_progress", "get_history"} {
		if !names[want] {
			t.Fatalf("tools/list is missing %q: %v", want, names)
		}
	}

	created := s.tool("create_entry", hermesCreateEntryArgs())
	var add struct {
		Created   bool    `json:"created"`
		WorkID    string  `json:"work_id"`
		EditionID *string `json:"edition_id"`
	}
	if err := json.Unmarshal(created, &add); err != nil || !add.Created || add.WorkID == "" || add.EditionID == nil {
		t.Fatalf("create_entry did not create the entry: %s", created)
	}

	// The exact request id with the same payload replays the stored
	// result, not a conflict and not a duplicate.
	replayed := s.tool("create_entry", hermesCreateEntryArgs())
	var replay struct {
		Created bool   `json:"created"`
		WorkID  string `json:"work_id"`
	}
	if err := json.Unmarshal(replayed, &replay); err != nil || !replay.Created || replay.WorkID != add.WorkID {
		t.Fatalf("idempotent replay must return the stored result: %s", replayed)
	}

	found := s.tool("search_library", map[string]any{"query": "Mushishi", "limit": 10})
	if !strings.Contains(string(found), add.WorkID) {
		t.Fatalf("search_library by the romaji alias did not find the work: %s", found)
	}

	// The write tools take the expected revision from the current view,
	// so the client reads the edition view first.
	edition := s.tool("get_edition", map[string]any{"edition_id": *add.EditionID})
	var view struct {
		Revision int64 `json:"revision"`
	}
	if err := json.Unmarshal(edition, &view); err != nil {
		t.Fatalf("get_edition did not return the edition view: %s", edition)
	}
	s.tool("record_progress", map[string]any{
		"request_id":        "hermes-req-2",
		"actor":             hermesActor(),
		"edition_id":        *add.EditionID,
		"unit":              "chapter",
		"position":          "3",
		"expected_revision": view.Revision,
	})
	history := s.tool("get_history", map[string]any{"work_id": add.WorkID})
	if !strings.Contains(string(history), "chapter") {
		t.Fatalf("get_history does not carry the recorded progress: %s", history)
	}
	return add.WorkID
}

// TestHermesStdioLifecycle proves the same-host agent path from the built
// binary: no HTTP, no Node, no provider. A hard kill must release the
// database handle, and a restarted client must see the same state.
func TestHermesStdioLifecycle(t *testing.T) {
	bin := hermesBuild(t, t.TempDir())
	state := t.TempDir()
	hermesMustRunCLI(t, bin, "init", "--state-dir", state)

	s1 := hermesOpenStdio(t, bin, state)
	workID := hermesStdioLifecycle(t, s1)
	s1.kill()

	// Session two opens the same state: the killed process must have
	// released the handle, and the state must be intact.
	s2 := hermesOpenStdio(t, bin, state)
	hermesHandshake(t, s2)
	history := s2.tool("get_history", map[string]any{"work_id": workID})
	if !strings.Contains(string(history), "chapter") {
		t.Fatalf("after restart the progress is gone: %s", history)
	}
	found := s2.tool("search_library", map[string]any{"query": "Mushishi", "limit": 10})
	if !strings.Contains(string(found), workID) {
		t.Fatalf("after restart the work is gone: %s", found)
	}
	s2.closeClean()
}

// --- Streamable HTTP: one external client over the wire ---

func hermesMCPOverHTTP(t *testing.T, client *http.Client, url, token string, frame map[string]any) (int, json.RawMessage, string) {
	t.Helper()
	body, _ := json.Marshal(frame)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// The Streamable HTTP spec requires both media types in Accept.
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil, string(data)
	}
	var msg struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("malformed MCP response: %s", data)
	}
	if msg.Error != nil {
		t.Fatalf("mcp protocol error: %s", msg.Error)
	}
	return http.StatusOK, msg.Result, string(data)
}

// hermesHTTPLifecycle drives the same lifecycle as the stdio smoke over
// Streamable HTTP: initialize, the tool surface, create, alias search,
// progress, history.
// httpTool calls one tool over Streamable HTTP and returns the single
// text content as raw JSON, mirroring the stdio tool helper.
func httpTool(t *testing.T, client *http.Client, url, token string, id int, name string, args map[string]any) json.RawMessage {
	t.Helper()
	code, res, body := hermesMCPOverHTTP(t, client, url, token, map[string]any{
		"jsonrpc": "2.0", "id": id, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args},
	})
	if code != http.StatusOK {
		t.Fatalf("id=%d tools/call %s over HTTP: code=%d body=%s", id, name, code, body)
	}
	var out struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(res, &out); err != nil || len(out.Content) != 1 {
		t.Fatalf("id=%d tools/call %s over HTTP: malformed result: %s", id, name, res)
	}
	if out.IsError {
		t.Fatalf("id=%d tool %s over HTTP failed: %s", id, name, out.Content[0].Text)
	}
	return json.RawMessage(out.Content[0].Text)
}

func hermesHTTPLifecycle(t *testing.T, client *http.Client, url, token string) (workID string) {
	t.Helper()
	// post wraps one Streamable HTTP exchange and fails with the full body
	// when the exchange does not return 200.
	post := func(id int, frame map[string]any) json.RawMessage {
		t.Helper()
		code, res, body := hermesMCPOverHTTP(t, client, url, token, frame)
		if code != http.StatusOK {
			t.Fatalf("id=%d %v over HTTP: code=%d body=%s", id, frame["method"], code, body)
		}
		return res
	}
	res := post(1, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "hermes-acceptance", "version": "0"},
		},
	})
	if !strings.Contains(string(res), "media-core") {
		t.Fatalf("initialize is not the media-core server: %s", res)
	}
	if res := post(2, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}); !strings.Contains(string(res), "create_entry") {
		t.Fatalf("tools/list over HTTP is missing the surface: %s", res)
	}
	res = httpTool(t, client, url, token, 3, "create_entry", hermesCreateEntryArgs())
	var add struct {
		Created   bool    `json:"created"`
		WorkID    string  `json:"work_id"`
		EditionID *string `json:"edition_id"`
	}
	if json.Unmarshal(res, &add) != nil || !add.Created || add.WorkID == "" || add.EditionID == nil {
		t.Fatalf("create_entry over HTTP did not create the entry: %s", res)
	}
	res = httpTool(t, client, url, token, 4, "search_library", map[string]any{"query": "Mushishi", "limit": 10})
	if !strings.Contains(string(res), add.WorkID) {
		t.Fatalf("search_library over HTTP did not find the work: %s", res)
	}
	// Same view-read before the write as the stdio lifecycle.
	res = httpTool(t, client, url, token, 5, "get_edition", map[string]any{"edition_id": *add.EditionID})
	var view struct {
		Revision int64 `json:"revision"`
	}
	if json.Unmarshal(res, &view) != nil {
		t.Fatalf("get_edition over HTTP did not return the edition view: %s", res)
	}
	res = httpTool(t, client, url, token, 6, "record_progress", map[string]any{
		"request_id":        "hermes-http-req-1",
		"actor":             hermesActor(),
		"edition_id":        *add.EditionID,
		"unit":              "chapter",
		"position":          "3",
		"expected_revision": view.Revision,
	})
	res = httpTool(t, client, url, token, 7, "get_history", map[string]any{"work_id": add.WorkID})
	if !strings.Contains(string(res), "chapter") {
		t.Fatalf("get_history over HTTP does not carry the progress: %s", res)
	}
	return add.WorkID
}

func hermesStartHTTP(t *testing.T, bin, state, addr string, extra ...string) {
	t.Helper()
	args := append([]string{"mcp", "--http", "--state-dir", state, "--listen", addr}, extra...)
	cmd := exec.Command(bin, args...)
	cmd.Env = hermesNodelessEnv()
	var diag bytes.Buffer
	cmd.Stderr = &diag
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %v: %v", args, err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			t.Logf("server diagnostics: %s", diag.String())
		}
	})
	hermesWaitTCP(t, addr)
}

func TestHermesStreamableHTTP(t *testing.T) {
	bin := hermesBuild(t, t.TempDir())
	client := http.DefaultClient

	t.Run("missing secret is a distinct 401", func(t *testing.T) {
		state := t.TempDir()
		hermesMustRunCLI(t, bin, "init", "--state-dir", state)
		addr := fmt.Sprintf("127.0.0.1:%d", hermesFreePort(t))
		hermesStartHTTP(t, bin, state, addr)
		code, _, body := hermesMCPOverHTTP(t, client, "http://"+addr+"/mcp", "", map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "initialize",
			"params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "hermes", "version": "0"}},
		})
		if code != http.StatusUnauthorized || !strings.Contains(body, "not configured") {
			t.Fatalf("want the distinct not-configured 401, got code=%d body=%s", code, body)
		}
	})

	t.Run("missing or wrong secret is rejected without touching state", func(t *testing.T) {
		state := t.TempDir()
		hermesMustRunCLI(t, bin, "init", "--state-dir", state)
		secret := hermesMustRunCLI(t, bin, "owner", "generate", "--state-dir", state)
		addr := fmt.Sprintf("127.0.0.1:%d", hermesFreePort(t))
		hermesStartHTTP(t, bin, state, addr)
		url := "http://" + addr + "/mcp"
		frame := map[string]any{
			"jsonrpc": "2.0", "id": 1, "method": "tools/call",
			"params": map[string]any{"name": "create_entry", "arguments": hermesCreateEntryArgs()},
		}
		for _, token := range []string{"", "deadbeef"} {
			code, _, body := hermesMCPOverHTTP(t, client, url, token, frame)
			if code != http.StatusUnauthorized || !strings.Contains(body, "does not match") {
				t.Fatalf("token=%q: want the 401 mismatch, got code=%d body=%s", token, code, body)
			}
		}
		// The rejected writes must have created nothing: the library is
		// still empty.
		res := httpTool(t, client, url, secret, 2, "search_library", map[string]any{"query": "Mushishi", "limit": 10})
		if !strings.Contains(string(res), `"total":0`) {
			t.Fatalf("rejected requests must not touch the state: %s", res)
		}
	})

	t.Run("non-loopback without the trio fails before any listener", func(t *testing.T) {
		state := t.TempDir()
		hermesMustRunCLI(t, bin, "init", "--state-dir", state)
		addr := fmt.Sprintf("0.0.0.0:%d", hermesFreePort(t))
		code, out := hermesRunCLI(t, bin, "mcp", "--http", "--state-dir", state, "--listen", addr)
		if code != 1 || !strings.Contains(out, "allow-remote") {
			t.Fatalf("want the config failure naming allow-remote, got exit=%d out=%q", code, out)
		}
	})

	t.Run("authenticated lifecycle", func(t *testing.T) {
		state := t.TempDir()
		hermesMustRunCLI(t, bin, "init", "--state-dir", state)
		secret := hermesMustRunCLI(t, bin, "owner", "generate", "--state-dir", state)
		addr := fmt.Sprintf("127.0.0.1:%d", hermesFreePort(t))
		hermesStartHTTP(t, bin, state, addr)
		if workID := hermesHTTPLifecycle(t, client, "http://"+addr+"/mcp", secret); workID == "" {
			t.Fatal("the HTTP lifecycle returned no work")
		}
	})

	t.Run("tls listener", func(t *testing.T) {
		dir := t.TempDir()
		certFile, keyFile := hermesWriteSelfSignedCert(t, dir)
		state := t.TempDir()
		hermesMustRunCLI(t, bin, "init", "--state-dir", state)
		secret := hermesMustRunCLI(t, bin, "owner", "generate", "--state-dir", state)
		addr := fmt.Sprintf("127.0.0.1:%d", hermesFreePort(t))
		hermesStartHTTP(t, bin, state, addr, "--tls-cert", certFile, "--tls-key", keyFile)
		tlsClient := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
		if workID := hermesHTTPLifecycle(t, tlsClient, "https://"+addr+"/mcp", secret); workID == "" {
			t.Fatal("the TLS HTTP lifecycle returned no work")
		}
	})
}

// TestHermesServeHasNoMCP pins the invariant: the web serve mode never
// exposes an MCP endpoint on its listener.
func TestHermesServeHasNoMCP(t *testing.T) {
	bin := hermesBuild(t, t.TempDir())
	state := t.TempDir()
	hermesMustRunCLI(t, bin, "init", "--state-dir", state)
	hermesMustRunCLI(t, bin, "owner", "generate", "--state-dir", state)
	addr := fmt.Sprintf("127.0.0.1:%d", hermesFreePort(t))
	cmd := exec.Command(bin, "serve", "--state-dir", state, "--listen", addr)
	cmd.Env = hermesNodelessEnv()
	var diag bytes.Buffer
	cmd.Stderr = &diag
	if err := cmd.Start(); err != nil {
		t.Fatalf("start serve: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			t.Logf("serve diagnostics: %s", diag.String())
		}
	})
	hermesWaitTCP(t, addr)

	// The web security middleware demands a same-origin Origin header for
	// unsafe methods; with it, a real MCP request would be distinguishable
	// from the plain JSON 404 the asset catch-all returns for POSTs.
	req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/mcp", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+addr)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp on serve: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotFound || strings.Contains(string(body), "jsonrpc") {
		t.Fatalf("serve must not expose an MCP endpoint: code=%d body=%s", resp.StatusCode, body)
	}
}

// hermesWriteSelfSignedCert creates a throwaway server certificate for the
// TLS listener check; it is valid only for this test's lifetime.
func hermesWriteSelfSignedCert(t *testing.T, dir string) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "media-core-acceptance"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certFile, keyFile
}
