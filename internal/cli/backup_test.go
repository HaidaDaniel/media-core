package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type backupData struct {
	Archive string `json:"archive"`
	Format  string `json:"format"`
	Schema  int    `json:"schema"`
	DB      struct {
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
	} `json:"db"`
}

func backupDataFrom(t *testing.T, out string) backupData {
	t.Helper()
	env := libEnv(t, out)
	if !env.OK || env.Error != nil {
		t.Fatalf("envelope = %+v, want ok", env)
	}
	b, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var data backupData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("data is not a backup payload: %v\n%s", err, out)
	}
	return data
}

// TestBackupCLI exercises the backup command surface: a successful publish
// prints the archive envelope, a second publish without force is a
// conflict, force replaces, and argument errors keep the usage exit code
// with the stable error codes.
func TestBackupCLI(t *testing.T) {
	dir := initLibState(t)
	addWorkID(t, dir, "Backup Fixture")
	dest := filepath.Join(t.TempDir(), "backups")

	code, out, errw := runCLI(t, "backup", "create", "--state-dir", dir, "--dest", dest)
	if code != ExitOK {
		t.Fatalf("create: exit = %d, stderr = %q", code, errw)
	}
	data := backupDataFrom(t, out)
	if data.Format != "media-core-snapshot-v1" {
		t.Fatalf("format = %q", data.Format)
	}
	if data.Schema <= 0 || data.DB.Name != "media-core.db" || data.DB.Size <= 0 {
		t.Fatalf("payload = %+v, want schema and measured db", data)
	}
	if ok, _ := regexp.MatchString(`^[0-9a-f]{64}$`, data.DB.SHA256); !ok {
		t.Fatalf("sha256 = %q, want 64 hex chars", data.DB.SHA256)
	}
	if fi, err := filepath.Abs(data.Archive); err != nil || fi != filepath.Join(dest, filepath.Base(data.Archive)) {
		t.Fatalf("archive = %q, want under %q", data.Archive, dest)
	}
	entries, err := os.ReadDir(data.Archive)
	if err != nil || len(entries) != 2 {
		t.Fatalf("published archive entries = %v (%v), want the database and the manifest", entries, err)
	}

	code, out, errw = runCLI(t, "backup", "create", "--state-dir", dir, "--dest", dest, "--name", "again")
	if code != ExitOK {
		t.Fatalf("named create: exit = %d, stderr = %q", code, errw)
	}
	code, out, errw = runCLI(t, "backup", "create", "--state-dir", dir, "--dest", dest, "--name", "again")
	if code != ExitConflict {
		t.Fatalf("repeat create: exit = %d, want %d, stderr = %q", code, ExitConflict, errw)
	}
	env := libEnv(t, out)
	if env.Error == nil || env.Error.Code != "conflict" {
		t.Fatalf("repeat create envelope = %+v, want code conflict", env)
	}
	code, out, errw = runCLI(t, "backup", "create", "--state-dir", dir, "--dest", dest, "--name", "again", "--force")
	if code != ExitOK {
		t.Fatalf("force create: exit = %d, stderr = %q", code, errw)
	}

	usageCases := [][]string{
		{"backup", "--state-dir", dir},
		{"backup", "restore", "--state-dir", dir},
		{"backup", "create", "--state-dir", dir},
		{"backup", "create", "--state-dir", dir, "--dest", dest, "--bogus"},
	}
	for _, args := range usageCases {
		code, _, errw := runCLI(t, args...)
		if code != ExitUsage {
			t.Fatalf("%v: exit = %d, want %d, stderr = %q", args, code, ExitUsage, errw)
		}
		if !strings.Contains(errw, "media-core backup") {
			t.Fatalf("%v: stderr = %q, want the backup usage hint", args, errw)
		}
	}

	code, out, errw = runCLI(t, "backup", "create", "--state-dir", dir, "--dest", "relative")
	if code != ExitUsage {
		t.Fatalf("relative dest: exit = %d, want %d, stderr = %q", code, ExitUsage, errw)
	}
	env = libEnv(t, out)
	if env.Error == nil || env.Error.Code != "invalid_argument" {
		t.Fatalf("relative dest envelope = %+v, want code invalid_argument", env)
	}

	// The destructive combination (overwrite a colliding state file from
	// inside the state dir) is refused and leaves the state usable.
	code, out, errw = runCLI(t, "backup", "create", "--state-dir", dir, "--dest", dir, "--name", "media-core.db", "--force")
	if code != ExitUsage {
		t.Fatalf("in-place create: exit = %d, want %d, stderr = %q", code, ExitUsage, errw)
	}
	env = libEnv(t, out)
	if env.Error == nil || env.Error.Code != "invalid_argument" {
		t.Fatalf("in-place create envelope = %+v, want code invalid_argument", env)
	}
	code, out, errw = runCLI(t, "list", "--state-dir", dir)
	if code != ExitOK {
		t.Fatalf("state unusable after refused in-place backup: exit = %d, stderr = %q", code, errw)
	}
}
