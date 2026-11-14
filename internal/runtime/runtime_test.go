package runtime

import (
	"bytes"
	"errors"
	"strings"
	"testing"
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
