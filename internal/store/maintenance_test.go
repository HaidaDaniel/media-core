package store

import (
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMaintenanceSequentialModes(t *testing.T) {
	dir := t.TempDir()
	shared, err := acquireMaintenanceLock(dir, LockShared, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("acquire shared: %v", err)
	}
	if err := shared.Release(); err != nil {
		t.Fatalf("release shared: %v", err)
	}
	excl, err := acquireMaintenanceLock(dir, LockExclusive, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("acquire exclusive: %v", err)
	}
	if err := excl.Release(); err != nil {
		t.Fatalf("release exclusive: %v", err)
	}
}

func TestMaintenanceSharedBlocksExclusive(t *testing.T) {
	dir := t.TempDir()
	shared, err := acquireMaintenanceLock(dir, LockShared, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("acquire shared: %v", err)
	}
	defer shared.Release()
	start := time.Now()
	_, err = acquireMaintenanceLock(dir, LockExclusive, 150*time.Millisecond)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("acquire exclusive while shared held: err = %v, want ErrLocked", err)
	}
	if elapsed < 140*time.Millisecond {
		t.Errorf("gave up after %v, before the 150ms deadline", elapsed)
	}
}

func TestMaintenanceExclusiveBlocksShared(t *testing.T) {
	dir := t.TempDir()
	excl, err := acquireMaintenanceLock(dir, LockExclusive, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("acquire exclusive: %v", err)
	}
	defer excl.Release()
	start := time.Now()
	_, err = acquireMaintenanceLock(dir, LockShared, 150*time.Millisecond)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("acquire shared while exclusive held: err = %v, want ErrLocked", err)
	}
	if elapsed < 140*time.Millisecond {
		t.Errorf("gave up after %v, before the 150ms deadline", elapsed)
	}
}

func TestMaintenanceLockFilePersists(t *testing.T) {
	dir := t.TempDir()
	l, err := acquireMaintenanceLock(dir, LockShared, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	path := l.Path()
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Errorf("lock file %s removed by release: %v", path, err)
	}
}

func TestMaintenanceRejectsSymlinkedLockFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "real.lock")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, MaintenanceLockFileName)); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	_, err := acquireMaintenanceLock(dir, LockShared, 100*time.Millisecond)
	if !errors.Is(err, ErrSymlink) {
		t.Errorf("symlinked lock file: err = %v, want ErrSymlink", err)
	}
}

func TestMaintenanceRejectsSymlinkedStateDir(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "state-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	_, err := acquireMaintenanceLock(link, LockShared, 100*time.Millisecond)
	if !errors.Is(err, ErrSymlink) {
		t.Errorf("symlinked state dir: err = %v, want ErrSymlink", err)
	}
}

func TestMaintenanceRejectsSymlinkedParent(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	// A missing final directory below a symlinked parent must be rejected
	// at creation time, not created through the link.
	stateDir := filepath.Join(link, "state")
	_, err := acquireMaintenanceLock(stateDir, LockShared, 100*time.Millisecond)
	if !errors.Is(err, ErrSymlink) {
		t.Errorf("symlinked parent: err = %v, want ErrSymlink", err)
	}
	if _, err := os.Lstat(stateDir); !os.IsNotExist(err) {
		t.Errorf("state dir created through symlinked parent: %v", err)
	}
}

// subprocessRole switches the test binary into child-holder mode.
const subprocessRole = "MEDIA_CORE_TEST_LOCK_ROLE"

// subprocessDir carries the state directory into the child.
const subprocessDir = "MEDIA_CORE_TEST_LOCK_DIR"

// TestMaintenanceSubprocessHold covers cross-process lock and busy
// behavior. In child mode (role set) it holds the lock or database until
// killed; in parent mode it runs the scenarios.
func TestMaintenanceSubprocessHold(t *testing.T) {
	role := os.Getenv(subprocessRole)
	if role != "" {
		childHold(role, os.Getenv(subprocessDir))
		return
	}
	t.Run("ExclusiveHolderBlocksShared", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "state")
		cmd := holdProcess(t, dir, "exclusive")
		defer killAndWait(t, cmd)
		waitForHoldMarker(t, dir)
		start := time.Now()
		_, err := AcquireMaintenanceLock(dir, LockShared)
		elapsed := time.Since(start)
		if !errors.Is(err, ErrLocked) {
			t.Fatalf("acquire shared: err = %v, want ErrLocked", err)
		}
		if elapsed < 4*time.Second || elapsed > 6*time.Second {
			t.Errorf("waited %v, want the ~5s lock deadline", elapsed)
		}
	})
	t.Run("SharedHolderBlocksExclusive", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "state")
		cmd := holdProcess(t, dir, "shared")
		defer killAndWait(t, cmd)
		waitForHoldMarker(t, dir)
		start := time.Now()
		_, err := AcquireMaintenanceLock(dir, LockExclusive)
		elapsed := time.Since(start)
		if !errors.Is(err, ErrLocked) {
			t.Fatalf("acquire exclusive: err = %v, want ErrLocked", err)
		}
		if elapsed < 4*time.Second || elapsed > 6*time.Second {
			t.Errorf("waited %v, want the ~5s lock deadline", elapsed)
		}
	})
	t.Run("OpenHolderBlocksMaintenanceOpen", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "state")
		cmd := holdProcess(t, dir, "db")
		defer killAndWait(t, cmd)
		waitForHoldMarker(t, dir)
		start := time.Now()
		db, err := OpenForMaintenance(dir)
		elapsed := time.Since(start)
		if !errors.Is(err, ErrLocked) {
			if db != nil {
				db.Close()
			}
			t.Fatalf("OpenForMaintenance with an active handle: err = %v, want ErrLocked", err)
		}
		if elapsed < 4*time.Second || elapsed > 6*time.Second {
			t.Errorf("waited %v, want the ~5s lock deadline", elapsed)
		}
	})
	t.Run("SharedOpensCoexist", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "state")
		cmd := holdProcess(t, dir, "db")
		defer killAndWait(t, cmd)
		waitForHoldMarker(t, dir)
		// A second shared holder (another CLI/MCP process) must be able to
		// open while the first handle stays active.
		db, err := Open(dir)
		if err != nil {
			t.Fatalf("second shared Open: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("second shared Close: %v", err)
		}
	})
	t.Run("BusyWriteTimesOut", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "state")
		cmd := holdProcess(t, dir, "busydb")
		defer killAndWait(t, cmd)
		waitForHoldMarker(t, dir)
		db, err := Open(dir)
		if err != nil {
			t.Fatalf("Open while holder keeps a write transaction: %v", err)
		}
		defer db.Close()
		start := time.Now()
		_, err = db.DB().Exec("CREATE TABLE busy_parent (x INTEGER)")
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("write succeeded while the holder keeps the write lock")
		}
		if !containsLocked(err.Error()) {
			t.Errorf("write error = %v, want a busy/locked error", err)
		}
		if elapsed < 4*time.Second || elapsed > 7*time.Second {
			t.Errorf("write gave up after %v, want the ~5s busy timeout", elapsed)
		}
	})
}

// migrateState initializes the schema under the exclusive maintenance lock
// so a subsequent normal open passes the schema boundary; a real server
// starts after init.
func migrateState(stateDir string) error {
	db, err := OpenForMaintenance(stateDir)
	if err != nil {
		return err
	}
	if _, err := RunMigrations(db.DB()); err != nil {
		db.Close()
		return err
	}
	return db.Close()
}

// childHold blocks forever holding the requested resource; the parent
// kills the process. A marker file signals that the hold is in place so
// the parent cannot race the child's lock acquisition.
func childHold(role, stateDir string) {
	var err error
	switch role {
	case "db":
		var db *Database
		if err = migrateState(stateDir); err == nil {
			db, err = Open(stateDir)
		}
		if err == nil {
			writeMarker(stateDir)
			<-make(chan struct{})
			db.Close()
		}
	case "busydb":
		var db *Database
		if err = migrateState(stateDir); err == nil {
			db, err = Open(stateDir)
		}
		if err == nil {
			if _, err = db.DB().Exec("CREATE TABLE busy_probe (x INTEGER)"); err == nil {
				var tx *sql.Tx
				tx, err = db.DB().Begin()
				if err == nil {
					if _, err = tx.Exec("INSERT INTO busy_probe VALUES (1)"); err == nil {
						writeMarker(stateDir)
						// Hold the write transaction without committing.
						<-make(chan struct{})
					}
				}
			}
			db.Close()
		}
	default:
		if err = os.MkdirAll(stateDir, 0o700); err == nil {
			mode := LockShared
			if role == "exclusive" {
				mode = LockExclusive
			}
			var l *MaintenanceLock
			l, err = AcquireMaintenanceLock(stateDir, mode)
			if err == nil {
				writeMarker(stateDir)
				<-make(chan struct{})
				l.Release()
			}
		}
	}
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func writeMarker(stateDir string) {
	if err := os.WriteFile(filepath.Join(stateDir, "held"), []byte("held"), 0o600); err != nil {
		os.Exit(1)
	}
}

func holdProcess(t *testing.T, dir, role string) *exec.Cmd {
	t.Helper()
	bin, err := os.Executable()
	if err != nil {
		t.Fatalf("test binary path: %v", err)
	}
	cmd := exec.Command(bin, "-test.run=TestMaintenanceSubprocessHold")
	cmd.Env = append(os.Environ(), subprocessRole+"="+role, subprocessDir+"="+dir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start holder: %v", err)
	}
	return cmd
}

func killAndWait(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd.Process == nil {
		return
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Logf("kill holder: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Logf("wait holder: %v", err)
	}
}

func waitForHoldMarker(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "held")
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Lstat(path); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("holder did not create %s within 15s", path)
}

func containsLocked(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "locked") || strings.Contains(lower, "busy")
}
