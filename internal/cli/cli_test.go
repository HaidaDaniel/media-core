package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := Run([]string{"version"}, &out, &errBuf); code != ExitOK {
		t.Fatalf("Run(version) exit = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(out.String(), "media-core") {
		t.Errorf("version output %q does not contain the build identity", out.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errBuf.String())
	}
}

func TestRunHelp(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		var out, errBuf bytes.Buffer
		if code := Run([]string{arg}, &out, &errBuf); code != ExitOK {
			t.Fatalf("Run(%s) exit = %d, want %d", arg, code, ExitOK)
		}
		if !strings.Contains(out.String(), "usage:") {
			t.Errorf("Run(%s) output %q does not contain usage", arg, out.String())
		}
		if errBuf.Len() != 0 {
			t.Errorf("Run(%s) stderr = %q, want empty", arg, errBuf.String())
		}
	}
}

func TestRunMissing(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := Run(nil, &out, &errBuf); code != ExitUsage {
		t.Fatalf("Run() exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errBuf.String(), "missing command") {
		t.Errorf("stderr %q does not contain the missing command message", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "usage:") {
		t.Errorf("stderr %q does not contain usage", errBuf.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout %q should be empty", out.String())
	}
}

func TestRunUnsupported(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := Run([]string{"frobnicate"}, &out, &errBuf); code != ExitUsage {
		t.Fatalf("Run(frobnicate) exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errBuf.String(), `unsupported command "frobnicate"`) {
		t.Errorf("stderr %q does not contain the unsupported command message", errBuf.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout %q should be empty", out.String())
	}
}

func TestRunReserved(t *testing.T) {
	for _, command := range []string{"mcp"} {
		var out, errBuf bytes.Buffer
		if code := Run([]string{command}, &out, &errBuf); code != ExitError {
			t.Fatalf("Run(%s) exit = %d, want %d", command, code, ExitError)
		}
		if !strings.Contains(errBuf.String(), "reserved") {
			t.Errorf("Run(%s) stderr %q does not contain the reserved message", command, errBuf.String())
		}
		if out.Len() != 0 {
			t.Errorf("Run(%s) stdout %q should be empty", command, out.String())
		}
	}
}

func TestRunServeFailsBeforeListen(t *testing.T) {
	// Unknown serve flags fail with usage before any listener is created.
	var out, errBuf bytes.Buffer
	if code := Run([]string{"serve", "--bogus"}, &out, &errBuf); code != ExitUsage {
		t.Fatalf("Run(serve --bogus) exit = %d, want %d (stderr %q)", code, ExitUsage, errBuf.String())
	}
	// A non-loopback bind without the remote trio fails with a config
	// error before any listener is created.
	out.Reset()
	errBuf.Reset()
	if code := Run([]string{"serve", "--listen", "0.0.0.0:18787"}, &out, &errBuf); code != ExitError {
		t.Fatalf("Run(serve --listen 0.0.0.0:18787) exit = %d, want %d (stderr %q)", code, ExitError, errBuf.String())
	}
	if out.Len() != 0 {
		t.Errorf("Run(serve --listen 0.0.0.0:18787) stdout %q should be empty", out.String())
	}
}

func TestVersionLine(t *testing.T) {
	line := VersionLine()
	if !strings.Contains(line, "media-core") {
		t.Errorf("VersionLine() = %q, want the rendered identity", line)
	}
}
