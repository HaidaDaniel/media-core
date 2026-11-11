package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDatabaseOpenCreatesSecureStateAndDB(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("state dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("state dir mode = %o, want 0700", perm)
	}
	if filepath.Base(db.Path()) != DBFileName {
		t.Errorf("db path = %q, want base %s", db.Path(), DBFileName)
	}
	dbInfo, err := os.Stat(db.Path())
	if err != nil {
		t.Fatalf("db file: %v", err)
	}
	if perm := dbInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("db file mode = %o, want 0600", perm)
	}
	if _, err := os.Stat(filepath.Join(dir, MaintenanceLockFileName)); err != nil {
		t.Errorf("maintenance lock file missing: %v", err)
	}
}

func TestDatabasePragmasEffective(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var journal string
	if err := db.DB().QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if journal != "wal" {
		t.Errorf("journal_mode = %q, want wal", journal)
	}
	var foreignKeys, synchronous, busyTimeout int
	if err := db.DB().QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}
	if err := db.DB().QueryRow("PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatal(err)
	}
	if synchronous != synchronousFull {
		t.Errorf("synchronous = %d, want FULL (%d)", synchronous, synchronousFull)
	}
	if err := db.DB().QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if busyTimeout != busyTimeoutMs {
		t.Errorf("busy_timeout = %d, want %d", busyTimeout, busyTimeoutMs)
	}
}

func TestDatabaseOpenHoldsLifetimeLock(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if _, err := acquireMaintenanceLock(dir, LockExclusive, 100*time.Millisecond); !errors.Is(err, ErrLocked) {
		t.Errorf("exclusive lock while db open: err = %v, want ErrLocked", err)
	}
}

func TestDatabaseCloseReleasesLock(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	l, err := acquireMaintenanceLock(dir, LockExclusive, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("exclusive lock after close: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
}

func TestDatabaseReopenSurvives(t *testing.T) {
	dir := t.TempDir()
	first, err := Open(dir)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	path := first.Path()
	if err := first.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	second, err := Open(dir)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer second.Close()
	if second.Path() != path {
		t.Errorf("path changed across reopen: %q vs %q", second.Path(), path)
	}
	var journal string
	if err := second.DB().QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if journal != "wal" {
		t.Errorf("journal_mode after reopen = %q, want wal", journal)
	}
}

// TestDatabaseOpenFailureReleasesLock proves a failed open never keeps the
// maintenance lock or a partially configured connection.
func TestDatabaseOpenFailureReleasesLock(t *testing.T) {
	dir := t.TempDir()
	// A directory where the DB file should be makes the driver open fail.
	if err := os.Mkdir(filepath.Join(dir, DBFileName), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("Open with a directory as the DB file: want error, got nil")
	}
	l, err := acquireMaintenanceLock(dir, LockExclusive, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("exclusive lock after failed open: %v, want the lock released", err)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseOpenFailsOnFileStateDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(dir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("Open with a file as the state dir: want error, got nil")
	}
}

func TestDatabaseRejectsSymlinkedDBFile(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(t.TempDir(), "real.db")
	if err := os.WriteFile(real, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(dir, DBFileName)); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if _, err := Open(dir); !errors.Is(err, ErrSymlink) {
		t.Errorf("symlinked DB file: err = %v, want ErrSymlink", err)
	}
	l, err := acquireMaintenanceLock(dir, LockExclusive, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("lock not released after symlink rejection: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseRejectsSymlinkedStateDir(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "state-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if _, err := Open(link); !errors.Is(err, ErrSymlink) {
		t.Errorf("symlinked state dir: err = %v, want ErrSymlink", err)
	}
}

func TestDatabaseRejectsRelativeStateDir(t *testing.T) {
	if _, err := Open("relative/state"); err == nil {
		t.Fatal("Open with a relative state dir: want error, got nil")
	}
}

func TestOpenTightensPreexistingModes(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, MaintenanceLockFileName)
	if err := os.WriteFile(lockPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("pre-existing state dir mode = %o, want 0700", perm)
	}
	lockInfo, err := os.Stat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := lockInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("pre-existing lock file mode = %o, want 0600", perm)
	}
}

// TestOpenAppliesSchemaCheckSeam proves the schema runner seam is invoked
// on the normal open path and that a failing check releases everything.
func TestOpenAppliesSchemaCheckSeam(t *testing.T) {
	dir := t.TempDir()
	orig := schemaCheck
	schemaCheck = func(*sql.DB) error { return errors.New("schema boom") }
	defer func() { schemaCheck = orig }()

	_, err := Open(dir)
	if err == nil || !strings.Contains(err.Error(), "schema boom") {
		t.Fatalf("Open with failing schema check: err = %v, want the check error", err)
	}
	l, err := acquireMaintenanceLock(dir, LockExclusive, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("lock not released after failed schema check: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
}
