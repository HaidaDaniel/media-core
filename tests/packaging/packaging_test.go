// Package packaging proves the release-artifact contract: one binary
// carries the API, the embedded UI and the CLI/MCP modes, and it runs
// without Node at runtime. The build itself (frontend, staging, cross
// links) is scripts/build-release.sh; these tests consume what it stages.
package packaging

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// repoRoot is the repository root: the tests live in tests/packaging.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, "..", "..")
}

// nodelessEnv is the child environment for every runtime check: the
// default environment minus PATH and Node variables, with a PATH that has
// no Node installation, so a binary that shelled out to Node would fail
// loudly instead of passing quietly.
func nodelessEnv() []string {
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

// buildBinary links the current source into dir; it uses only the Go
// toolchain and the staged embedded assets, never Node.
func buildBinary(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "media-core")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/media-core")
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// freePort returns an unbound loopback TCP port for a smoke server.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// runCLI runs one CLI command and returns the exit code and combined
// output.
func runCLI(t *testing.T, bin string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = repoRoot(t)
	cmd.Env = nodelessEnv()
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

// TestPackageCLIWithoutNode proves the built binary runs the CLI surface
// with no Node in the environment: identity, first-time init and a read
// against the fresh state.
func TestPackageCLIWithoutNode(t *testing.T) {
	bin := buildBinary(t, t.TempDir())

	code, out := runCLI(t, bin, "version")
	if code != 0 || !strings.Contains(out, "media-core") {
		t.Fatalf("version: exit=%d out=%q, want the identity line", code, out)
	}

	state := t.TempDir()
	code, out = runCLI(t, bin, "init", "--state-dir", state)
	if code != 0 {
		t.Fatalf("init: exit=%d out=%q", code, out)
	}
	code, out = runCLI(t, bin, "list", "--state-dir", state)
	if code != 0 || !strings.Contains(out, `"ok":true`) {
		t.Fatalf("list: exit=%d out=%q, want the ok envelope", code, out)
	}
}

// TestPackageMCPSmokeWithoutNode proves the MCP stdio lifecycle works from
// the built binary without Node: initialize, the initialized
// notification, and a tools/list that carries the real tool surface.
func TestPackageMCPSmokeWithoutNode(t *testing.T) {
	bin := buildBinary(t, t.TempDir())
	state := t.TempDir()
	if code, out := runCLI(t, bin, "init", "--state-dir", state); code != 0 {
		t.Fatalf("init: exit=%d out=%q", code, out)
	}

	cmd := exec.Command(bin, "mcp", "--state-dir", state)
	cmd.Env = nodelessEnv()
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
	defer func() {
		stdin.Close()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	frames := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"packaging-smoke","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}
	go func() {
		for _, f := range frames {
			_, _ = io.WriteString(stdin, f+"\n")
		}
	}()

	deadline := time.Now().Add(30 * time.Second)
	var initResult, toolsList []byte
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for time.Now().Before(deadline) && (initResult == nil || toolsList == nil) {
		if !scanner.Scan() {
			break
		}
		line := scanner.Bytes()
		var head struct {
			ID     any     `json:"id"`
			Result any     `json:"result"`
			Error  *string `json:"error"`
		}
		if err := json.Unmarshal(line, &head); err != nil {
			continue
		}
		if head.Error != nil {
			t.Fatalf("mcp protocol error: %s", *head.Error)
		}
		// Copy: the scanner reuses its buffer on the next Scan.
		if id, _ := head.ID.(float64); id == 1 && head.Result != nil {
			initResult = append([]byte(nil), line...)
		}
		if id, _ := head.ID.(float64); id == 2 && head.Result != nil {
			toolsList = append([]byte(nil), line...)
		}
	}
	if initResult == nil {
		t.Fatalf("no initialize response; diagnostics: %s", diag.String())
	}
	if toolsList == nil {
		t.Fatalf("no tools/list response; diagnostics: %s", diag.String())
	}
	if !strings.Contains(string(initResult), "media-core") {
		t.Fatalf("initialize result is not the media-core server: %s", initResult)
	}
	var toolsResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(toolsList, &toolsResp); err != nil || len(toolsResp.Result.Tools) == 0 {
		t.Fatalf("tools/list is empty or malformed: %s", toolsList)
	}
	found := false
	for _, tool := range toolsResp.Result.Tools {
		if tool.Name == "search_library" {
			found = true
		}
	}
	if !found {
		t.Fatalf("search_library is missing from the live tool surface: %s", toolsList)
	}
}

// TestPackageServeServesEmbeddedUIWithoutNode proves the resident serve
// mode from the built binary serves the embedded UI same-origin with the
// API on the same listener, with no Node in the environment, and stops
// cleanly on SIGINT.
func TestPackageServeServesEmbeddedUIWithoutNode(t *testing.T) {
	bin := buildBinary(t, t.TempDir())
	state := t.TempDir()
	if code, out := runCLI(t, bin, "init", "--state-dir", state); code != 0 {
		t.Fatalf("init: exit=%d out=%q", code, out)
	}
	port := freePort(t)
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	cmd := exec.Command(bin, "serve", "--state-dir", state, "--listen", fmt.Sprintf("127.0.0.1:%d", port))
	cmd.Env = nodelessEnv()
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start serve: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	client := &http.Client{Timeout: 5 * time.Second}
	up := false
	for i := 0; i < 100; i++ {
		resp, err := client.Get(base + "/")
		if err == nil {
			resp.Body.Close()
			up = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !up {
		t.Fatalf("serve never came up: %s", out.String())
	}

	get := func(url string) (*http.Response, []byte) {
		t.Helper()
		resp, err := client.Get(url)
		if err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatalf("read %s: %v", url, err)
		}
		return resp, body
	}

	// The embedded UI answers the root and a client-side route.
	resp, body := get(base + "/")
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Media Core") {
		t.Fatalf("GET /: status=%d body=%q, want the embedded shell", resp.StatusCode, body)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("shell Cache-Control = %q, want no-cache", cc)
	}
	resp, body = get(base + "/library")
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Media Core") {
		t.Fatalf("GET /library: status=%d body=%q, want the SPA fallback", resp.StatusCode, body)
	}
	// The API stays JSON: an unauthenticated session read is the typed
	// 401, an unknown API path the typed 404, never the shell.
	resp, body = get(base + "/api/v1/session")
	if resp.StatusCode != http.StatusUnauthorized || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("GET /api/v1/session: status=%d ct=%q, want the typed 401", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	resp, body = get(base + "/api/v9/nope")
	if resp.StatusCode != http.StatusNotFound || strings.Contains(string(body), "Media Core") {
		t.Fatalf("GET /api/v9/nope: status=%d body=%q, want the JSON 404, not the shell", resp.StatusCode, body)
	}

	// SIGINT: clean stop with the stop line on stdout.
	_ = cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatalf("serve did not stop on SIGINT: %s", out.String())
	}
	if !strings.Contains(out.String(), "serve: stopped") {
		t.Fatalf("the clean-stop line is missing: %s", out.String())
	}
}

// TestPackageReleaseArtifacts checks the cross-built release binaries when
// scripts/build-release.sh has staged them: both targets exist as native
// ELFs for their architectures, the host-matching one runs the identity
// smoke, and the other is reported as smoke-limited on this host rather
// than silently assumed.
func TestPackageReleaseArtifacts(t *testing.T) {
	root := repoRoot(t)
	release := filepath.Join(root, "release")
	amd64 := filepath.Join(release, "media-core-linux-amd64")
	arm64 := filepath.Join(release, "media-core-linux-arm64")
	if _, err := os.Stat(amd64); err != nil {
		if os.Getenv(releaseGateEnv) == "1" {
			t.Fatalf("release gate requires %s; run scripts/build-release.sh first", amd64)
		}
		t.Skip("release artifacts are not staged; run scripts/build-release.sh first")
	}
	if _, err := os.Stat(arm64); err != nil {
		t.Fatalf("amd64 artifact exists but arm64 is missing")
	}

	for path, want := range map[string]uint16{amd64: 62, arm64: 183} { // EM_X86_64, EM_AARCH64
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		var magic [4]byte
		if _, err := f.Read(magic[:]); err != nil || string(magic[:]) != "\x7fELF" {
			t.Fatalf("%s is not an ELF binary", path)
		}
		if _, err := f.Seek(18, io.SeekStart); err != nil {
			t.Fatalf("seek %s: %v", path, err)
		}
		var machine [2]byte
		if _, err := f.Read(machine[:]); err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		f.Close()
		if got := binary.LittleEndian.Uint16(machine[:]); got != want {
			t.Fatalf("%s: e_machine = %d, want %d", path, got, want)
		}
	}

	arch := osArch(t)
	hostArtifact, ok := map[string]string{
		"x86_64":  amd64,
		"aarch64": arm64,
	}[arch]
	if !ok {
		t.Logf("host arch %s is not a release target; both artifacts are limited to format checks on this host", arch)
		return
	}
	code, out := runCLI(t, hostArtifact, "version")
	if code != 0 || !strings.Contains(out, "media-core") {
		t.Fatalf("host artifact version: exit=%d out=%q", code, out)
	}
	t.Logf("host-matching artifact smoke passed: %s", hostArtifact)
}

// osArch is the uname-style host architecture name.
func osArch(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("uname", "-m").CombinedOutput()
	if err != nil {
		t.Fatalf("uname: %v", err)
	}
	return strings.TrimSpace(string(out))
}

const releaseGateEnv = "MEDIA_CORE_RELEASE_GATE"
const stageDirEnv = "MEDIA_CORE_STAGE_DIR"

type webAssetEntry struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type webAssetsManifest struct {
	Format string          `json:"format"`
	Files  []webAssetEntry `json:"files"`
}

// TestPackageReleaseArtifactsManifest verifies a real manifest against real
// staged files. The build script sets the staging directory while the files
// still exist; a missing, malformed or self-referential manifest fails there.
func TestPackageReleaseArtifactsManifest(t *testing.T) {
	stage := os.Getenv(stageDirEnv)
	if stage == "" {
		t.Skip("staging directory is not set; run scripts/build-release.sh")
	}
	raw, err := os.ReadFile(filepath.Join(stage, "manifest.json"))
	if err != nil {
		t.Fatalf("read web asset manifest: %v", err)
	}
	var manifest webAssetsManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse web asset manifest: %v", err)
	}
	if manifest.Format != "media-core-webassets-v1" {
		t.Fatalf("manifest format = %q, want media-core-webassets-v1", manifest.Format)
	}
	listed := map[string]bool{}
	for _, entry := range manifest.Files {
		if entry.Name == "manifest.json" {
			t.Fatal("manifest contains a self-entry")
		}
		if entry.Name == "" || filepath.Clean(entry.Name) != entry.Name || filepath.IsAbs(entry.Name) {
			t.Fatalf("unsafe manifest entry name %q", entry.Name)
		}
		if listed[entry.Name] {
			t.Fatalf("duplicate manifest entry %q", entry.Name)
		}
		listed[entry.Name] = true
		path := filepath.Join(stage, entry.Name)
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("manifest entry %s is absent: %v", entry.Name, err)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("manifest entry %s is not a regular file", entry.Name)
		}
		if info.Size() != entry.Size {
			t.Fatalf("manifest entry %s size = %d, want %d", entry.Name, info.Size(), entry.Size)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read manifest entry %s: %v", entry.Name, err)
		}
		sum := sha256.Sum256(contents)
		if fmt.Sprintf("%x", sum) != entry.SHA256 {
			t.Fatalf("manifest entry %s SHA-256 mismatch", entry.Name)
		}
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		t.Fatalf("read staging directory: %v", err)
	}
	for _, e := range entries {
		if e.Name() == "manifest.json" || e.IsDir() {
			continue
		}
		if !listed[e.Name()] {
			t.Fatalf("staged file %s is absent from the manifest", e.Name())
		}
	}
}
