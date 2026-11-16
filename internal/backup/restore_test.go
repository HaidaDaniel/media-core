package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/store"
)

// seedProbeState initializes one state directory with a probe table and
// rows, releases the lifetime maintenance lock, and returns the dir with a
// plain read handle that no longer blocks the exclusive lock.
func seedProbeState(t *testing.T, rows int) (string, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	maint, err := store.OpenForMaintenance(dir)
	if err != nil {
		t.Fatalf("OpenForMaintenance: %v", err)
	}
	if _, err := store.RunMigrations(maint.DB()); err != nil {
		maint.Close()
		t.Fatalf("RunMigrations: %v", err)
	}
	if err := maint.Close(); err != nil {
		t.Fatalf("Close maintenance: %v", err)
	}
	sd, err := store.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	db := sd.DB()
	mustExec(t, db, `CREATE TABLE backup_probe (id INTEGER PRIMARY KEY, n TEXT)`)
	for i := 1; i <= rows; i++ {
		mustExec(t, db, `INSERT INTO backup_probe (id, n) VALUES (?, ?)`, i, fmt.Sprintf("row-%d", i))
	}
	if err := sd.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return dir, db
}

// makeArchive publishes one archive of the source state and returns the
// archive path.
func makeArchive(t *testing.T, srcDir, name string) string {
	t.Helper()
	res, err := Create(context.Background(), Options{StateDir: srcDir, DestDir: t.TempDir(), Name: name})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return res.Path
}

// stateDBHash returns the hash of the state database, or "" when absent.
func stateDBHash(t *testing.T, stateDir string) string {
	t.Helper()
	path := filepath.Join(stateDir, store.DBFileName)
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return fileHash(t, path)
}

// restoreCount opens the state database read-only and counts probe rows.
func restoreCount(t *testing.T, stateDir string) int64 {
	t.Helper()
	db := openArchiveDB(t, filepath.Join(stateDir, store.DBFileName))
	raw := mustQuery(t, db, `SELECT COUNT(*) FROM backup_probe`)
	n, _ := raw[0].(int64)
	return n
}

// noRestoreDebris asserts the state directory holds no staging files.
func noRestoreDebris(t *testing.T, stateDir string) {
	t.Helper()
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == "" && len(e.Name()) > 9 && e.Name()[:9] == ".restore-" {
			t.Fatalf("staging debris %q in %s", e.Name(), stateDir)
		}
	}
}

// TestRestoreIsolatedDestination restores into a destination that does not
// exist yet, without force, and checks the published state.
func TestRestoreIsolatedDestination(t *testing.T) {
	src, _ := seedProbeState(t, 5)
	archive := makeArchive(t, src, "isolated")

	dest := filepath.Join(t.TempDir(), "fresh")
	res, err := Restore(context.Background(), RestoreOptions{Archive: archive, StateDir: dest})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if res.StateDir != dest || res.Schema <= 0 {
		t.Fatalf("result = %+v, want state dir and positive schema", res)
	}
	if got := restoreCount(t, dest); got != 5 {
		t.Fatalf("restored probe count = %d, want 5", got)
	}
	archHash := fileHash(t, filepath.Join(archive, DBFileName))
	if got := stateDBHash(t, dest); got != archHash {
		t.Fatalf("restored db hash = %q, want the archived hash %q", got, archHash)
	}
	if res.DB.SHA256 != archHash {
		t.Fatalf("result hash = %q, want %q", res.DB.SHA256, archHash)
	}
	noRestoreDebris(t, dest)
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 2 || (names[0] != store.DBFileName && names[0] != store.MaintenanceLockFileName) || (names[1] != store.DBFileName && names[1] != store.MaintenanceLockFileName) {
		t.Fatalf("destination entries = %v, want only the database and the lock file", names)
	}
}

// TestRestoreExistingDestination refuses replacement without force, leaves
// the old database byte-identical, and replaces it with the archived data
// when force is explicit.
func TestRestoreExistingDestination(t *testing.T) {
	src, srcDB := seedProbeState(t, 3)
	archive := makeArchive(t, src, "replace")
	srcDB.Close()
	dest, destDB := seedProbeState(t, 7)
	oldHash := stateDBHash(t, dest)
	if got := restoreCount(t, dest); got != 7 {
		t.Fatalf("destination count before restore = %d, want 7", got)
	}
	destDB.Close()

	_, err := Restore(context.Background(), RestoreOptions{Archive: archive, StateDir: dest})
	if !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("Restore without force err = %v, want ErrDestinationExists", err)
	}
	if got := stateDBHash(t, dest); got != oldHash {
		t.Fatalf("old database changed despite the refused restore")
	}
	if got := restoreCount(t, dest); got != 7 {
		t.Fatalf("destination count after refusal = %d, want 7", got)
	}

	res, err := Restore(context.Background(), RestoreOptions{Archive: archive, StateDir: dest, Force: true})
	if err != nil {
		t.Fatalf("forced Restore: %v", err)
	}
	if res.StateDir != dest {
		t.Fatalf("result state dir = %q, want %q", res.StateDir, dest)
	}
	if got := restoreCount(t, dest); got != 3 {
		t.Fatalf("destination count after replace = %d, want 3", got)
	}
	if got := stateDBHash(t, dest); got == oldHash {
		t.Fatalf("destination database was not replaced")
	}
	noRestoreDebris(t, dest)
}

// TestRestoreValidation exercises each invalid archive shape and checks the
// destination is untouched in every case.
func TestRestoreValidation(t *testing.T) {
	src, _ := seedProbeState(t, 2)
	archive := makeArchive(t, src, "invalid-shapes")

	tamper := func(name string, mutate func(dir string)) string {
		t.Helper()
		dir := t.TempDir()
		for _, entry := range []string{DBFileName, ManifestFileName} {
			data, err := os.ReadFile(filepath.Join(archive, entry))
			if err != nil {
				t.Fatalf("read %s: %v", entry, err)
			}
			if err := os.WriteFile(filepath.Join(dir, entry), data, 0o600); err != nil {
				t.Fatalf("write %s: %v", entry, err)
			}
		}
		mutate(dir)
		return dir
	}

	cases := []struct {
		name string
		arch string
	}{
		{"missing archive", filepath.Join(t.TempDir(), "nope")},
		{"truncated database", tamper("trunc", func(dir string) {
			p := filepath.Join(dir, DBFileName)
			data, _ := os.ReadFile(p)
			os.WriteFile(p, data[:len(data)/2], 0o600)
		})},
		{"tampered manifest schema", tamper("schema", func(dir string) {
			p := filepath.Join(dir, ManifestFileName)
			data, _ := os.ReadFile(p)
			var man Manifest
			if err := json.Unmarshal(data, &man); err != nil {
				t.Fatalf("unmarshal manifest: %v", err)
			}
			man.Schema = 9999
			raw, _ := json.Marshal(man)
			os.WriteFile(p, raw, 0o600)
		})},
		{"wrong format marker", tamper("format", func(dir string) {
			os.WriteFile(filepath.Join(dir, ManifestFileName), []byte(`{"format":"other-v1"}`), 0o600)
		})},
		{"manifest not json", tamper("json", func(dir string) {
			os.WriteFile(filepath.Join(dir, ManifestFileName), []byte("not json"), 0o600)
		})},
		{"extra entry", tamper("extra", func(dir string) {
			os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("x"), 0o600)
		})},
		{"missing manifest", tamper("nomanifest", func(dir string) {
			os.Remove(filepath.Join(dir, ManifestFileName))
		})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "dest")
			_, err := Restore(context.Background(), RestoreOptions{Archive: tc.arch, StateDir: dest, Force: true})
			if !errors.Is(err, ErrArchiveInvalid) {
				t.Fatalf("Restore err = %v, want ErrArchiveInvalid", err)
			}
			if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
				t.Fatalf("destination %s was created or touched", dest)
			}
		})
	}

	t.Run("archive path is a file", func(t *testing.T) {
		_, err := Restore(context.Background(), RestoreOptions{Archive: filepath.Join(archive, DBFileName), StateDir: t.TempDir()})
		if !errors.Is(err, ErrArchiveInvalid) {
			t.Fatalf("Restore err = %v, want ErrArchiveInvalid", err)
		}
	})
	t.Run("archive path is a symlink", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(archive, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		_, err := Restore(context.Background(), RestoreOptions{Archive: link, StateDir: t.TempDir()})
		if !errors.Is(err, ErrArchiveInvalid) {
			t.Fatalf("Restore err = %v, want ErrArchiveInvalid", err)
		}
	})
}

// TestRestoreOffBuildSchema rejects archives whose ledger is newer or
// older than the restoring build with the distinct schema boundary errors.
func TestRestoreOffBuildSchema(t *testing.T) {
	// The build's current schema, read from a freshly initialized state.
	_, db := initState(t)
	var current int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		t.Fatalf("read current schema: %v", err)
	}

	build := func(schema int) string {
		t.Helper()
		dir := t.TempDir()
		if _, err := SyntheticArchive(dir, schema, time.Now().UTC()); err != nil {
			t.Fatalf("SyntheticArchive: %v", err)
		}
		return dir
	}

	t.Run("newer schema", func(t *testing.T) {
		_, err := Restore(context.Background(), RestoreOptions{Archive: build(current + 5000), StateDir: t.TempDir()})
		if !errors.Is(err, store.ErrSchemaNewer) {
			t.Fatalf("Restore err = %v, want ErrSchemaNewer", err)
		}
	})
	t.Run("older schema", func(t *testing.T) {
		if current <= 1 {
			t.Skip("build schema too small for the older case")
		}
		_, err := Restore(context.Background(), RestoreOptions{Archive: build(1), StateDir: t.TempDir()})
		if !errors.Is(err, store.ErrSchemaPending) {
			t.Fatalf("Restore err = %v, want ErrSchemaPending", err)
		}
	})
	t.Run("current schema with an incomplete ledger", func(t *testing.T) {
		// SyntheticArchive writes only the named version. A matching maximum
		// alone must not make a ledger current: earlier migrations are absent.
		_, err := Restore(context.Background(), RestoreOptions{Archive: build(current), StateDir: t.TempDir()})
		if !errors.Is(err, store.ErrSchemaPending) {
			t.Fatalf("Restore err = %v, want ErrSchemaPending", err)
		}
	})
}

// TestRestoreRetainsOwnerSecret replaces only the database: other files in
// the destination state directory, including secret material, are retained
// byte-identical.
func TestRestoreRetainsOwnerSecret(t *testing.T) {
	src, srcDB := seedProbeState(t, 2)
	archive := makeArchive(t, src, "secrets")
	srcDB.Close()
	dest, destDB := seedProbeState(t, 4)
	destDB.Close()
	secret := "owner-secret-material"
	secretPath := filepath.Join(dest, "owner-secret")
	if err := os.WriteFile(secretPath, []byte(secret), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	if _, err := Restore(context.Background(), RestoreOptions{Archive: archive, StateDir: dest, Force: true}); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := restoreCount(t, dest); got != 2 {
		t.Fatalf("restored count = %d, want 2", got)
	}
	got, err := os.ReadFile(secretPath)
	if err != nil || string(got) != secret {
		t.Fatalf("owner secret = %q (%v), want %q retained", got, err, secret)
	}
}

// TestRestoreActiveHandleBlocks proves an active shared-lock holder — a
// running serve, MCP or CLI handle — blocks the replacement.
func TestRestoreActiveHandleBlocks(t *testing.T) {
	src, _ := seedProbeState(t, 2)
	archive := makeArchive(t, src, "blocked")
	dest, _ := seedProbeState(t, 4)
	oldHash := stateDBHash(t, dest)

	live, err := store.Open(dest)
	if err != nil {
		t.Fatalf("open active handle: %v", err)
	}
	defer live.Close()

	_, err = Restore(context.Background(), RestoreOptions{Archive: archive, StateDir: dest, Force: true})
	if !errors.Is(err, store.ErrLocked) {
		t.Fatalf("Restore with an active handle err = %v, want ErrLocked", err)
	}
	if got := stateDBHash(t, dest); got != oldHash {
		t.Fatalf("destination database changed despite the blocked restore")
	}
}

// TestRestoreInterruption stands in for a crash between the staged
// validation and the publish rename: the previous database must stay
// usable and no staging debris may remain.
func TestRestoreInterruption(t *testing.T) {
	src, srcDB := seedProbeState(t, 2)
	archive := makeArchive(t, src, "interrupted")
	srcDB.Close()
	dest, destDB := seedProbeState(t, 4)
	destDB.Close()
	oldHash := stateDBHash(t, dest)

	prePublishHook = func(staged string) { os.Rename(staged, staged+".gone") }
	defer func() { prePublishHook = nil }()

	_, err := Restore(context.Background(), RestoreOptions{Archive: archive, StateDir: dest, Force: true})
	if err == nil {
		t.Fatalf("interrupted Restore succeeded, want failure")
	}
	if got := stateDBHash(t, dest); got != oldHash {
		t.Fatalf("destination database changed despite the interrupted restore")
	}
	if got := restoreCount(t, dest); got != 4 {
		t.Fatalf("restored count = %d, want the old 4", got)
	}
	noRestoreDebris(t, dest)
}

// TestRestoreReadOnlyDestination fails the restore when the destination
// cannot be written and leaves the previous database usable.
func TestRestoreReadOnlyDestination(t *testing.T) {
	src, srcDB := seedProbeState(t, 2)
	archive := makeArchive(t, src, "readonly")
	srcDB.Close()
	dest, destDB := seedProbeState(t, 4)
	destDB.Close()
	oldHash := stateDBHash(t, dest)

	if err := os.Chmod(dest, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dest, 0o700) })

	_, err := Restore(context.Background(), RestoreOptions{Archive: archive, StateDir: dest, Force: true})
	if err == nil {
		t.Fatalf("Restore into a read-only destination succeeded, want failure")
	}
	if got := stateDBHash(t, dest); got != oldHash {
		t.Fatalf("destination database changed despite the failed restore")
	}
}

// TestRestoreRejectsSymlinkedParentBeforeCreation proves the destination
// path chain is verified before MkdirAll can create a directory through a
// symlinked parent.
func TestRestoreRejectsSymlinkedParentBeforeCreation(t *testing.T) {
	src, db := initState(t)
	archive := makeArchive(t, src, "symlink-parent")
	db.Close()

	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatalf("mkdir real: %v", err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	dest := filepath.Join(link, "dest")
	_, err := Restore(context.Background(), RestoreOptions{Archive: archive, StateDir: dest})
	if !errors.Is(err, store.ErrSymlink) {
		t.Fatalf("Restore err = %v, want ErrSymlink", err)
	}
	if _, statErr := os.Lstat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("destination %s was created through the symlink", dest)
	}
	entries, err := os.ReadDir(real)
	if err != nil {
		t.Fatalf("read real dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("symlink target gained %d entries", len(entries))
	}
}

const walCrashRoleEnv = "MC_BACKUP_WAL_CRASH_ROLE"
const walCrashDirEnv = "MC_BACKUP_WAL_CRASH_DIR"

// walCrashWriterRole commits one write and exits without closing SQLite, so
// the acknowledged row stays in the WAL and never reaches the main file.
func walCrashWriterRole(dir string) {
	db, err := store.Open(dir)
	if err != nil {
		os.Exit(11)
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if _, err := db.DB().Exec(`INSERT INTO works (id, revision, created_at, updated_at) VALUES ('work-wal-only', 0, ?, ?)`, now, now); err != nil {
		os.Exit(12)
	}
	if err := os.WriteFile(filepath.Join(dir, "committed"), []byte("ok"), 0o600); err != nil {
		os.Exit(13)
	}
	os.Exit(0)
}

func rowsInMainFileOnly(t *testing.T, stateDir string) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(stateDir, store.DBFileName))
	if err != nil {
		t.Fatalf("read destination database: %v", err)
	}
	path := filepath.Join(t.TempDir(), "main-only.db")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("stage main-only copy: %v", err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open main-only copy: %v", err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM works WHERE id = 'work-wal-only'`).Scan(&count); err != nil {
		t.Fatalf("query main-only copy: %v", err)
	}
	return count
}

func TestRestoreInterruptedAfterCheckpointKeepsData(t *testing.T) {
	if role := os.Getenv(walCrashRoleEnv); role == "writer" {
		walCrashWriterRole(os.Getenv(walCrashDirEnv))
		return
	}
	dir := t.TempDir()
	maint, err := store.OpenForMaintenance(dir)
	if err != nil {
		t.Fatalf("OpenForMaintenance: %v", err)
	}
	if _, err := store.RunMigrations(maint.DB()); err != nil {
		maint.Close()
		t.Fatalf("RunMigrations: %v", err)
	}
	if err := maint.Close(); err != nil {
		t.Fatalf("Close maintenance: %v", err)
	}

	bin, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cmd := exec.Command(bin, "-test.run=TestRestoreInterruptedAfterCheckpointKeepsData")
	cmd.Env = append(os.Environ(), walCrashRoleEnv+"=writer", walCrashDirEnv+"="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("crash writer: %v (%s)", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, store.DBFileName+"-wal")); err != nil {
		t.Fatalf("WAL sidecar missing after abrupt exit: %v", err)
	}
	if got := rowsInMainFileOnly(t, dir); got != 0 {
		t.Fatalf("committed row is already checkpointed: %d, want 0", got)
	}

	src, srcDB := seedProbeState(t, 2)
	archive := makeArchive(t, src, "wal-crash")
	srcDB.Close()
	oldHook := prePublishHook
	prePublishHook = func(staged string) { _ = os.Remove(staged) }
	defer func() { prePublishHook = oldHook }()
	if _, err := Restore(context.Background(), RestoreOptions{Archive: archive, StateDir: dir, Force: true}); err == nil {
		t.Fatal("interrupted restore unexpectedly succeeded")
	}
	if got := rowsInMainFileOnly(t, dir); got != 1 {
		t.Fatalf("committed row after interrupted restore = %d, want 1", got)
	}
}

func TestRestoreRejectsSymlinkedDestinationBeforeCheckpoint(t *testing.T) {
	src, srcDB := seedProbeState(t, 2)
	archive := makeArchive(t, src, "symlink-destination")
	srcDB.Close()

	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "outside.db")
	const sentinel = "do not touch me"
	if err := os.WriteFile(target, []byte(sentinel), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	final := filepath.Join(dir, store.DBFileName)
	if err := os.Symlink(target, final); err != nil {
		t.Fatalf("symlink destination: %v", err)
	}
	if _, err := Restore(context.Background(), RestoreOptions{Archive: archive, StateDir: dir, Force: true}); !errors.Is(err, store.ErrSymlink) {
		t.Fatalf("Restore err = %v, want ErrSymlink", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != sentinel {
		t.Fatalf("target outside the state dir was modified: %q", got)
	}
	if info, err := os.Lstat(final); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("destination symlink was replaced or removed: info=%v err=%v", info, err)
	}
}
