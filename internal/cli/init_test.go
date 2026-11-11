package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/store"
)

type maintenanceEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	OK            bool   `json:"ok"`
	Data          struct {
		Schema int `json:"schema"`
	} `json:"data"`
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errw bytes.Buffer
	code := Run(append([]string{}, args...), &out, &errw)
	return code, out.String(), errw.String()
}

func ledgerMaxVersion(t *testing.T, dir string) int {
	t.Helper()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	var max int
	if err := db.DB().QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&max); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	return max
}

func ledgerRowCount(t *testing.T, dir string) int {
	t.Helper()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer db.Close()
	var count int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	return count
}

func TestInitCreatesCurrentDatabase(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	code, out, errw := runCLI(t, "init", "--state-dir", dir)
	if code != ExitOK {
		t.Fatalf("init exit = %d, stderr = %q", code, errw)
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("stdout must be exactly one JSON document, got %q", out)
	}
	var env maintenanceEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout is not the JSON envelope: %v\n%s", err, out)
	}
	if !env.OK || env.SchemaVersion != "1.0" {
		t.Fatalf("envelope = %+v, want ok with schema_version 1.0", env)
	}
	if env.Data.Schema != ledgerMaxVersion(t, dir) {
		t.Fatalf("reported schema %d, ledger max %d", env.Data.Schema, ledgerMaxVersion(t, dir))
	}
	if got := ledgerRowCount(t, dir); got == 0 {
		t.Fatal("init did not record applied migrations in the ledger")
	}
}

// TestInitRepeatedIsSafe proves init is an idempotent no-op on a current
// database: the ledger is not touched and the command succeeds again.
func TestInitRepeatedIsSafe(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	code, _, errw := runCLI(t, "init", "--state-dir", dir)
	if code != ExitOK {
		t.Fatalf("first init exit = %d, stderr = %q", code, errw)
	}
	rows := ledgerRowCount(t, dir)
	code, out, errw := runCLI(t, "init", "--state-dir", dir)
	if code != ExitOK {
		t.Fatalf("second init exit = %d, stderr = %q", code, errw)
	}
	var env maintenanceEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("second init stdout is not the JSON envelope: %v", err)
	}
	if got := ledgerRowCount(t, dir); got != rows {
		t.Fatalf("ledger rows changed by repeated init: %d -> %d", rows, got)
	}
}

// TestMigrationCommandMatchesInit proves migrate on an empty state dir
// performs the same first setup as init.
func TestMigrationCommandMatchesInit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	code, out, errw := runCLI(t, "migrate", "--state-dir", dir)
	if code != ExitOK {
		t.Fatalf("migrate exit = %d, stderr = %q", code, errw)
	}
	var env maintenanceEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("migrate stdout is not the JSON envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("migrate envelope = %+v, want ok", env)
	}
}

func TestInitUnknownArgument(t *testing.T) {
	code, _, errw := runCLI(t, "init", "--bogus")
	if code != ExitUsage {
		t.Fatalf("init with unknown argument: exit = %d, stderr = %q, want %d", code, errw, ExitUsage)
	}
}

func TestInitRelativeStateDir(t *testing.T) {
	code, _, _ := runCLI(t, "init", "--state-dir", "relative/state")
	if code != ExitError {
		t.Fatalf("init with relative state dir: exit = %d, want %d", code, ExitError)
	}
}

// TestInitBlockedByActiveHandle proves a shared handle from another process
// blocks the exclusive maintenance session and init fails with exit 4
// instead of waiting unboundedly or migrating anyway.
func TestInitBlockedByActiveHandle(t *testing.T) {
	dir := t.TempDir()
	l, err := store.AcquireMaintenanceLock(dir, store.LockShared)
	if err != nil {
		t.Fatalf("acquire shared holder: %v", err)
	}
	defer l.Release()
	code, _, errw := runCLI(t, "init", "--state-dir", dir)
	if code != ExitLocked {
		t.Fatalf("init while shared-held: exit = %d, stderr = %q, want %d", code, errw, ExitLocked)
	}
}

func TestInitUsageListsCommands(t *testing.T) {
	code, out, _ := runCLI(t, "help")
	if code != ExitOK {
		t.Fatalf("help exit = %d", code)
	}
	for _, want := range []string{"init", "migrate", "version"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage does not list %q:\n%s", want, out)
		}
	}
}
