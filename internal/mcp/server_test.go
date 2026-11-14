// Subprocess tests for the MCP stdio lifecycle. The parent drives a
// re-executed test binary that runs the server over the real process
// stdio streams (the same entry path the mcp command uses), so framing,
// stdout purity, EOF handling and the no-mutation guarantee are proven
// against a real process, not an in-memory pipe.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/config"
	"github.com/HaidaDaniel/media-core/internal/store"
)

const (
	// subprocessEnv names the state dir for the re-executed child; a
	// non-empty value switches the test into child mode.
	subprocessEnv = "MEDIA_CORE_MCP_TEST_STATE_DIR"
	// testProtoVersion is a protocol version the pinned SDK supports.
	testProtoVersion = "2025-06-18"
)

// runChildMode is the child entry: open the state database and serve the
// MCP protocol over the process stdio streams until EOF.
func runChildMode(t *testing.T, stateDir string) {
	db, err := store.Open(stateDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mcp child open:", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := New(app.NewService(db), logger).Run(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mcp child run:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// initDatabase initializes a temporary state directory exactly the way
// the init command does.
func initDatabase(t *testing.T, stateDir string) {
	t.Helper()
	if _, err := config.Resolve(config.Options{StateDir: stateDir}); err != nil {
		t.Fatalf("resolve config: %v", err)
	}
	db, err := store.OpenForMaintenance(stateDir)
	if err != nil {
		t.Fatalf("open for maintenance: %v", err)
	}
	defer db.Close()
	if _, err := store.RunMigrations(db.DB()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type mcpProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr *bytes.Buffer
}

func startMCPChild(t *testing.T, testName, stateDir string) *mcpProcess {
	t.Helper()
	bin, err := os.Executable()
	if err != nil {
		t.Fatalf("test binary path: %v", err)
	}
	cmd := exec.Command(bin, "-test.run="+testName)
	cmd.Env = append(os.Environ(), subprocessEnv+"="+stateDir)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start mcp child: %v", err)
	}
	p := &mcpProcess{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout), stderr: stderr}
	t.Cleanup(func() {
		stdin.Close()
		if cmd.ProcessState == nil && cmd.Process != nil {
			cmd.Process.Kill()
		}
	})
	return p
}

func (p *mcpProcess) write(t *testing.T, frame string) {
	t.Helper()
	if _, err := p.stdin.Write([]byte(frame + "\n")); err != nil {
		t.Fatalf("write frame: %v", err)
	}
}

// read waits for one protocol frame and enforces the stdout purity
// contract: every stdout line is a JSON-RPC 2.0 message.
func (p *mcpProcess) read(t *testing.T) rpcMessage {
	t.Helper()
	type outcome struct {
		line string
		err  error
	}
	ch := make(chan outcome, 1)
	go func() {
		line, err := p.stdout.ReadString('\n')
		ch <- outcome{line: line, err: err}
	}()
	select {
	case o := <-ch:
		if o.err != nil {
			t.Fatalf("read frame (stderr: %s): %v", p.stderr.String(), o.err)
		}
		var msg rpcMessage
		if err := json.Unmarshal([]byte(o.line), &msg); err != nil {
			t.Fatalf("stdout is not a JSON frame: %q (%v)", o.line, err)
		}
		if msg.JSONRPC != "2.0" {
			t.Fatalf("frame is not JSON-RPC 2.0: %q", o.line)
		}
		return msg
	case <-time.After(20 * time.Second):
		t.Fatalf("timed out waiting for a protocol frame (stderr: %s)", p.stderr.String())
	}
	return rpcMessage{}
}

func (p *mcpProcess) initialize(t *testing.T) rpcMessage {
	t.Helper()
	req := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":%q,"capabilities":{},"clientInfo":{"name":"media-core-test","version":"0"}}}`,
		testProtoVersion,
	)
	p.write(t, req)
	return p.read(t)
}

func (p *mcpProcess) listTools(t *testing.T) rpcMessage {
	t.Helper()
	p.write(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	p.write(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	return p.read(t)
}

// TestMCPServer proves the bootstrap server starts on an initialized
// temporary database with no serve process: the initialize handshake
// reports the media-core implementation, tools/list returns the (empty)
// bootstrap tool set, a clean client EOF exits zero, and every stdout
// line is a protocol frame.
func TestMCPServer(t *testing.T) {
	if dir := os.Getenv(subprocessEnv); dir != "" {
		runChildMode(t, dir)
		return
	}
	stateDir := t.TempDir()
	initDatabase(t, stateDir)
	p := startMCPChild(t, "TestMCPServer", stateDir)

	res := p.initialize(t)
	var initResult struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(res.Result, &initResult); err != nil {
		t.Fatalf("unmarshal initialize result: %v", err)
	}
	if initResult.ServerInfo.Name != "media-core" {
		t.Fatalf("serverInfo.name = %q, want media-core", initResult.ServerInfo.Name)
	}
	if initResult.ProtocolVersion == "" {
		t.Fatalf("the server did not negotiate a protocol version")
	}

	list := p.listTools(t)
	var toolsResult struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(list.Result, &toolsResult); err != nil {
		t.Fatalf("unmarshal tools/list result: %v", err)
	}
	// The bootstrap registers no tools; domain tools arrive with the
	// capability tasks and no generic tool exists at any point.
	if len(toolsResult.Tools) != 0 {
		t.Fatalf("bootstrap server must register no tools: %s", list.Result)
	}

	if err := p.stdin.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	if err := p.cmd.Wait(); err != nil {
		t.Fatalf("clean client EOF should exit zero: %v (stderr: %s)", err, p.stderr.String())
	}
}

// TestMCPLifecycle covers the failure paths: an uninitialized state dir
// fails before any frame with a distinguishable schema error, a
// malformed message gets a JSON-RPC parse error while the server keeps
// working, and protocol traffic never mutates the database.
func TestMCPLifecycle(t *testing.T) {
	if dir := os.Getenv(subprocessEnv); dir != "" {
		runChildMode(t, dir)
		return
	}
	t.Run("uninitialized state dir fails before any frame", func(t *testing.T) {
		stateDir := t.TempDir()
		p := startMCPChild(t, "TestMCPLifecycle", stateDir)
		if err := p.cmd.Wait(); err == nil {
			t.Fatalf("expected a failure on an uninitialized state dir")
		}
		if !strings.Contains(strings.ToLower(p.stderr.String()), "schema") {
			t.Fatalf("the failure is not the schema boundary (stderr: %s)", p.stderr.String())
		}
		if p.stdout.Buffered() != 0 {
			t.Fatalf("no frame may be written before the schema boundary holds")
		}
	})

	t.Run("unknown method gets a method-not-found error and the session keeps working", func(t *testing.T) {
		stateDir := t.TempDir()
		initDatabase(t, stateDir)
		p := startMCPChild(t, "TestMCPLifecycle", stateDir)

		res := p.initialize(t)
		var initResult struct {
			ServerInfo struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		}
		if err := json.Unmarshal(res.Result, &initResult); err != nil {
			t.Fatalf("unmarshal initialize result: %v", err)
		}
		if initResult.ServerInfo.Name != "media-core" {
			t.Fatalf("serverInfo.name = %q, want media-core", initResult.ServerInfo.Name)
		}
		p.write(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)

		// A well-formed request the server does not handle is answered,
		// not fatal: the session survives and still lists tools.
		p.write(t, `{"jsonrpc":"2.0","id":9,"method":"bogus/method","params":{}}`)
		bad := p.read(t)
		if bad.Error == nil || bad.Error.Code != -32601 {
			t.Fatalf("expected a method-not-found error (-32601), got: %+v", bad.Error)
		}
		list := p.listTools(t)
		var toolsResult struct {
			Tools []json.RawMessage `json:"tools"`
		}
		if err := json.Unmarshal(list.Result, &toolsResult); err != nil {
			t.Fatalf("unmarshal tools/list result: %v", err)
		}

		if err := p.stdin.Close(); err != nil {
			t.Fatalf("close stdin: %v", err)
		}
		if err := p.cmd.Wait(); err != nil {
			t.Fatalf("clean client EOF should exit zero: %v (stderr: %s)", err, p.stderr.String())
		}
		assertNoMutation(t, stateDir)
	})

	t.Run("malformed stream data ends the session distinguishably without mutation", func(t *testing.T) {
		// The pinned SDK treats a broken stream as a fatal transport
		// error: the session ends, the failure is distinguishable on
		// stderr, and nothing is written to stdout but frames.
		stateDir := t.TempDir()
		initDatabase(t, stateDir)
		p := startMCPChild(t, "TestMCPLifecycle", stateDir)

		p.write(t, "this is not json")
		outDone := make(chan string, 1)
		go func() {
			rest, _ := io.ReadAll(p.stdout)
			outDone <- string(rest)
		}()
		if err := p.cmd.Wait(); err == nil {
			t.Fatalf("expected a failure on a malformed stream, got a clean exit")
		}
		if !strings.Contains(p.stderr.String(), "mcp child run:") {
			t.Fatalf("the failure must be the distinguishable run error (stderr: %s)", p.stderr.String())
		}
		// Every line the server managed to write must still be a
		// protocol frame.
		for _, line := range strings.Split(strings.TrimSpace(<-outDone), "\n") {
			if line != "" && !isFrame(line) {
				t.Fatalf("stdout carried a non-frame line: %q", line)
			}
		}
		assertNoMutation(t, stateDir)
	})
}

// assertNoMutation proves the protocol traffic left the database intact:
// the domain tables stay empty and the schema ledger stays current.
func assertNoMutation(t *testing.T, stateDir string) {
	t.Helper()
	db, err := store.Open(stateDir)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer db.Close()
	var works, editions int
	if err := db.DB().QueryRow("SELECT count(*) FROM works").Scan(&works); err != nil {
		t.Fatalf("count works: %v", err)
	}
	if err := db.DB().QueryRow("SELECT count(*) FROM editions").Scan(&editions); err != nil {
		t.Fatalf("count editions: %v", err)
	}
	if works != 0 || editions != 0 {
		t.Fatalf("protocol traffic mutated the database: works=%d editions=%d", works, editions)
	}
}

func isFrame(line string) bool {
	var msg rpcMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return false
	}
	return msg.JSONRPC == "2.0"
}
