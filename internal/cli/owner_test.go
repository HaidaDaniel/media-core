package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnerGenerateAndShow(t *testing.T) {
	stateDir := t.TempDir()
	var out, errw bytes.Buffer
	code := Run([]string{"owner", "generate", "--state-dir", stateDir}, &out, &errw)
	if code != ExitOK {
		t.Fatalf("generate: exit = %d, want 0 (stderr %q)", code, errw.String())
	}
	secret := strings.TrimSpace(out.String())
	if len(secret) != 64 {
		t.Fatalf("generate output = %q, want 64 hex chars (stderr %q)", secret, errw.String())
	}
	info, err := os.Stat(filepath.Join(stateDir, "owner-secret"))
	if err != nil {
		t.Fatalf("owner secret file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("owner secret mode = %o, want 0600", perm)
	}

	var showOut, showErr bytes.Buffer
	code = Run([]string{"owner", "show", "--state-dir", stateDir}, &showOut, &showErr)
	if code != ExitOK {
		t.Fatalf("show: exit = %d, want 0 (stderr %q)", code, showErr.String())
	}
	if got := strings.TrimSpace(showOut.String()); got != secret {
		t.Fatalf("show = %q, want %q", got, secret)
	}

	// Generation is idempotent: an existing secret is never replaced.
	var again, againErr bytes.Buffer
	code = Run([]string{"owner", "generate", "--state-dir", stateDir}, &again, &againErr)
	if code != ExitOK {
		t.Fatalf("regenerate: exit = %d, want 0 (stderr %q)", code, againErr.String())
	}
	if got := strings.TrimSpace(again.String()); got != secret {
		t.Fatalf("regenerate = %q, want the existing secret %q", got, secret)
	}
}

func TestOwnerShowMissing(t *testing.T) {
	stateDir := t.TempDir()
	var out, errw bytes.Buffer
	code := Run([]string{"owner", "show", "--state-dir", stateDir}, &out, &errw)
	if code != ExitError {
		t.Fatalf("show without secret: exit = %d, want %d (stderr %q)", code, ExitError, errw.String())
	}
	if !strings.Contains(errw.String(), "not configured") {
		t.Fatalf("show without secret must point at the missing secret, got %q", errw.String())
	}
	if out.Len() != 0 {
		t.Fatalf("show without secret printed to stdout: %q", out.String())
	}
}

func TestOwnerUsage(t *testing.T) {
	cases := []struct {
		args      []string
		wantUsage bool
	}{
		{[]string{"owner"}, true},
		{[]string{"owner", "rotate"}, true},
		{[]string{"owner", "generate", "show"}, true},
		{[]string{"owner", "generate", "--state-dir"}, false},
	}
	for _, tc := range cases {
		var out, errw bytes.Buffer
		code := Run(tc.args, &out, &errw)
		if code != ExitUsage {
			t.Fatalf("%v: exit = %d, want %d (stderr %q)", tc.args, code, ExitUsage, errw.String())
		}
		if tc.wantUsage && !strings.Contains(errw.String(), "usage:") {
			t.Fatalf("%v: stderr %q must carry the usage line", tc.args, errw.String())
		}
	}
}
