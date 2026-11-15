package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/store"
)

type doctorDoc struct {
	OK       bool `json:"ok"`
	StateDir string `json:"state_dir"`
	Checks   []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Detail string `json:"detail"`
	} `json:"checks"`
}

func doctorDataFrom(t *testing.T, out string) doctorDoc {
	t.Helper()
	env := libEnv(t, out)
	if !env.OK || env.Error != nil {
		t.Fatalf("envelope = %+v, want ok", env)
	}
	b, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var doc doctorDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("data is not a doctor report: %v\n%s", err, out)
	}
	return doc
}

func findCheck(t *testing.T, doc doctorDoc, name string) (string, string) {
	t.Helper()
	for _, c := range doc.Checks {
		if c.Name == name {
			return c.Status, c.Detail
		}
	}
	t.Fatalf("check %q missing from the report: %+v", name, doc.Checks)
	return "", ""
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestDoctorMissingState proves the diagnosis of an absent state: the
// report says so with init guidance, and nothing is created — no state
// directory, no database, no sidecar files.
func TestDoctorMissingState(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "state")

	code, out, errw := runCLI(t, "doctor", "--state-dir", dir)
	if code != ExitOK {
		t.Fatalf("doctor: exit = %d, stderr = %q", code, errw)
	}
	doc := doctorDataFrom(t, out)
	if doc.OK {
		t.Fatalf("report = %+v, want not ok", doc)
	}
	if status, detail := findCheck(t, doc, "state_dir"); status != "fail" || !strings.Contains(detail, "media-core init") {
		t.Fatalf("state_dir = %q %q, want a fail with init guidance", status, detail)
	}
	if status, _ := findCheck(t, doc, "schema"); status != "fail" {
		t.Fatalf("schema = %q, want fail", status)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("doctor created the state directory: %v", err)
	}
	if entries := dirEntries(t, parent); len(entries) != 0 {
		t.Fatalf("parent gained entries: %v", entries)
	}
}

// TestDoctorActiveState proves the active verdict and the read-only
// guarantee: every check passes, no file appears or disappears, and the
// database file itself is untouched.
func TestDoctorActiveState(t *testing.T) {
	dir := initLibState(t)
	addWorkID(t, dir, "Doctor Story")
	before := dirEntries(t, dir)
	dbPath := filepath.Join(dir, store.DBFileName)
	beforeInfo, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat db: %v", err)
	}

	code, out, errw := runCLI(t, "doctor", "--state-dir", dir)
	if code != ExitOK {
		t.Fatalf("doctor: exit = %d, stderr = %q", code, errw)
	}
	doc := doctorDataFrom(t, out)
	if !doc.OK {
		t.Fatalf("report = %+v, want ok", doc)
	}
	for _, name := range []string{"config", "state_dir", "database", "schema", "permissions"} {
		if status, detail := findCheck(t, doc, name); status != "pass" {
			t.Fatalf("check %s = %q %q, want pass", name, status, detail)
		}
	}
	if after := dirEntries(t, dir); len(after) != len(before) {
		t.Fatalf("state dir entries changed: %v -> %v", before, after)
	}
	afterInfo, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat db after: %v", err)
	}
	if !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Fatalf("database mtime changed: %v -> %v", beforeInfo.ModTime(), afterInfo.ModTime())
	}
}

// TestDoctorSchemaStates proves the unsupported-state diagnosis: a ledger
// behind the build asks for migrate, a ledger ahead of it asks for a newer
// build, and in both cases the verdict is a report, not a crash.
func TestDoctorSchemaStates(t *testing.T) {
	t.Run("behind", func(t *testing.T) {
		dir := initLibState(t)
		maint, err := store.OpenForMaintenance(dir)
		if err != nil {
			t.Fatalf("OpenForMaintenance: %v", err)
		}
		if _, err := maint.DB().Exec(`DELETE FROM schema_migrations WHERE version = (SELECT MAX(version) FROM schema_migrations)`); err != nil {
			t.Fatalf("drop ledger row: %v", err)
		}
		if err := maint.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		code, out, errw := runCLI(t, "doctor", "--state-dir", dir)
		if code != ExitOK {
			t.Fatalf("doctor: exit = %d, stderr = %q", code, errw)
		}
		doc := doctorDataFrom(t, out)
		if doc.OK {
			t.Fatalf("report = %+v, want not ok", doc)
		}
		status, detail := findCheck(t, doc, "schema")
		if status != "fail" || !strings.Contains(detail, "media-core migrate") {
			t.Fatalf("schema = %q %q, want a fail with migrate guidance", status, detail)
		}
	})

	t.Run("ahead", func(t *testing.T) {
		dir := initLibState(t)
		maint, err := store.OpenForMaintenance(dir)
		if err != nil {
			t.Fatalf("OpenForMaintenance: %v", err)
		}
		var current int
		if err := maint.DB().QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&current); err != nil {
			t.Fatalf("read ledger: %v", err)
		}
		if _, err := maint.DB().Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
			current+1, "9999_future.sql", "2026-11-15T00:00:00Z"); err != nil {
			t.Fatalf("insert future ledger row: %v", err)
		}
		if err := maint.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		code, out, errw := runCLI(t, "doctor", "--state-dir", dir)
		if code != ExitOK {
			t.Fatalf("doctor: exit = %d, stderr = %q", code, errw)
		}
		doc := doctorDataFrom(t, out)
		if doc.OK {
			t.Fatalf("report = %+v, want not ok", doc)
		}
		status, detail := findCheck(t, doc, "schema")
		if status != "fail" || !strings.Contains(detail, "newer version") {
			t.Fatalf("schema = %q %q, want a fail asking for a newer build", status, detail)
		}
	})
}

// TestDoctorPermissions proves loose modes are diagnosed as warnings, not
// faults: the report stays ok and the next normal open would repair them.
func TestDoctorPermissions(t *testing.T) {
	dir := initLibState(t)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	code, out, errw := runCLI(t, "doctor", "--state-dir", dir)
	if code != ExitOK {
		t.Fatalf("doctor: exit = %d, stderr = %q", code, errw)
	}
	doc := doctorDataFrom(t, out)
	if !doc.OK {
		t.Fatalf("report = %+v, want ok with warnings", doc)
	}
	status, detail := findCheck(t, doc, "permissions")
	if status != "warn" || !strings.Contains(detail, "755") {
		t.Fatalf("permissions = %q %q, want a warn naming the mode", status, detail)
	}
}

// TestDoctorUsage pins the argument handling: extra arguments and a
// relative state dir fail before any probe runs.
func TestDoctorUsage(t *testing.T) {
	dir := initLibState(t)
	code, _, _ := runCLI(t, "doctor", "--state-dir", dir, "extra")
	if code != ExitUsage {
		t.Fatalf("extra argument: exit = %d, want %d", code, ExitUsage)
	}
	code, _, errw := runCLI(t, "doctor", "--state-dir", "relative/state")
	if code != ExitError || !strings.Contains(errw, "absolute") {
		t.Fatalf("relative state dir: exit = %d, stderr = %q, want the absolute-path rejection", code, errw)
	}
}
