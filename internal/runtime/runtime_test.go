package runtime

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/config"
	"github.com/HaidaDaniel/media-core/internal/mcp"
)

func TestDispatchVersion(t *testing.T) {
	var out bytes.Buffer
	if err := Dispatch([]string{"version"}, &out); err != nil {
		t.Fatalf("Dispatch(version) error: %v", err)
	}
	if !strings.Contains(out.String(), "media-core") {
		t.Errorf("version output %q does not contain the build identity", out.String())
	}
}

func TestDispatchMissing(t *testing.T) {
	if err := Dispatch(nil, &bytes.Buffer{}); !errors.Is(err, ErrMissingCommand) {
		t.Errorf("Dispatch(nil) = %v, want ErrMissingCommand", err)
	}
}

func TestDispatchMCPFailsBeforeAnyFrame(t *testing.T) {
	// Unknown mcp flags fail fast; there is no listener flag because the
	// transport is stdio and no native remote listener exists.
	var out bytes.Buffer
	err := Dispatch([]string{"mcp", "--bogus"}, &out)
	if err == nil {
		t.Fatal("Dispatch(mcp --bogus) = nil, want a flag error")
	}
	var exitErr *ExitCodeError
	if !errors.As(err, &exitErr) || exitErr.Exit != 2 {
		t.Errorf("Dispatch(mcp --bogus) = %v, want exit code 2", err)
	}
	if out.Len() != 0 {
		t.Errorf("Dispatch(mcp --bogus) wrote %q to out", out.String())
	}

	// A state directory without an initialized database fails with the
	// schema guidance before any protocol frame is written and no serve
	// process is needed.
	dir := t.TempDir()
	out.Reset()
	err = Dispatch([]string{"mcp", "--state-dir", dir}, &out)
	if err == nil {
		t.Fatal("Dispatch(mcp on an empty state dir) = nil, want the schema error")
	}
	exitErr = nil
	if !errors.As(err, &exitErr) || exitErr.Exit != 1 {
		t.Fatalf("Dispatch(mcp on an empty state dir) = %v, want exit code 1", err)
	}
	if !strings.Contains(err.Error(), "run media-core init") {
		t.Errorf("error %q must carry the init guidance", err)
	}
	if out.Len() != 0 {
		t.Errorf("Dispatch(mcp on an empty state dir) wrote %q to out", out.String())
	}
}

func TestDispatchServeFailsBeforeListen(t *testing.T) {
	// Unknown serve flags fail fast without binding.
	err := Dispatch([]string{"serve", "--bogus"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Dispatch(serve --bogus) = nil, want a flag error")
	}
	// A non-loopback bind without the remote trio fails before any
	// listener is created.
	err = Dispatch([]string{"serve", "--listen", "0.0.0.0:18787"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Dispatch(serve --listen 0.0.0.0:18787) = nil, want the remote trio error")
	}
	var exitErr *ExitCodeError
	if !errors.As(err, &exitErr) || exitErr.Exit != 1 {
		t.Errorf("Dispatch(serve --listen 0.0.0.0:18787) = %v, want exit code 1", err)
	}
}

func TestParseServeTrustedLANFlags(t *testing.T) {
	opts, err := parseServeArgs([]string{
		"--state-dir", "/var/lib/media-core", "--trusted-lan",
		"--listen", "192.168.1.100:8787", "--lan-host", "media.home.arpa",
	})
	if err != nil {
		t.Fatalf("parseServeArgs: %v", err)
	}
	if !opts.TrustedLAN || opts.LANHost != "media.home.arpa" || opts.ListenAddr != "192.168.1.100:8787" || opts.StateDir != "/var/lib/media-core" {
		t.Fatalf("parsed options = %+v, want explicit trusted LAN values", opts)
	}

	plain, err := parseServeArgs([]string{"--state-dir", "/var/lib/media-core"})
	if err != nil || plain.TrustedLAN || plain.LANHost != "" {
		t.Fatalf("default serve flags = %+v, %v; want no trusted LAN", plain, err)
	}
	if _, err := parseServeArgs([]string{"--lan-host"}); err == nil {
		t.Fatal("--lan-host without a value must fail")
	}
}

func TestDispatchServeTrustedLANRequiresPrivateListen(t *testing.T) {
	err := Dispatch([]string{"serve", "--trusted-lan", "--lan-host", "media.home.arpa"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "trusted LAN listen") {
		t.Fatalf("trusted LAN without --listen: err = %v, want listener guidance", err)
	}
	err = Dispatch([]string{"serve", "--trusted-lan", "--listen", "8.8.8.8:8787", "--lan-host", "media.home.arpa"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "public IPv4") {
		t.Fatalf("trusted LAN on public IP: err = %v, want public IP rejection", err)
	}
	// A valid trusted LAN configuration reaches the schema boundary; an
	// uninitialized state directory still fails before binding.
	dir := t.TempDir()
	err = Dispatch([]string{"serve", "--state-dir", dir, "--trusted-lan", "--listen", "192.168.1.100:8787", "--lan-host", "media.home.arpa"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "run media-core init") {
		t.Fatalf("trusted LAN with empty state: err = %v, want schema guidance", err)
	}
}

func TestDispatchServeUninitializedStateDir(t *testing.T) {
	// A state directory without an initialized database fails with the
	// schema guidance before any listener is created.
	dir := t.TempDir()
	err := Dispatch([]string{"serve", "--state-dir", dir}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Dispatch(serve on an empty state dir) = nil, want the schema error")
	}
	var exitErr *ExitCodeError
	if !errors.As(err, &exitErr) || exitErr.Exit != 1 {
		t.Fatalf("Dispatch(serve on an empty state dir) = %v, want exit code 1", err)
	}
	if !strings.Contains(err.Error(), "run media-core init") {
		t.Errorf("error %q must carry the init guidance", err)
	}
}

func TestDispatchUnsupported(t *testing.T) {
	var out bytes.Buffer
	err := Dispatch([]string{"frobnicate"}, &out)
	var unsupported ErrUnsupported
	if !errors.As(err, &unsupported) || unsupported.Command != "frobnicate" {
		t.Errorf("Dispatch(frobnicate) = %v, want ErrUnsupported", err)
	}
	if out.Len() != 0 {
		t.Errorf("Dispatch(frobnicate) wrote %q to out", out.String())
	}
}

func TestShutdownMCPHTTPTimedOutDrainExitsCleanly(t *testing.T) {
	previous := mcpShutdownDrain
	mcpShutdownDrain = 100 * time.Millisecond
	t.Cleanup(func() { mcpShutdownDrain = previous })

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	addr := probe.Addr().String()
	probe.Close()
	cfg := &config.Config{StateDir: t.TempDir(), ListenAddr: addr}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveMCPHTTP(ctx, cfg, mcp.New(nil, nil)) }()

	var conn net.Conn
	for i := 0; i < 100 && conn == nil; i++ {
		conn, err = net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err != nil {
			time.Sleep(5 * time.Millisecond)
		}
	}
	if conn == nil {
		cancel()
		t.Fatalf("mcp http listener never accepted a connection on %s", addr)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("POST /mcp HTTP/1.1\r\nHost: " + addr + "\r\n")); err != nil {
		t.Fatalf("write partial request: %v", err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveMCPHTTP after timed-out drain: %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serveMCPHTTP did not force-close after the drain deadline")
	}
}
