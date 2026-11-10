// Package store owns SQLite access and the process-lifetime maintenance
// lock for media-core.
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// MaintenanceLockFileName is the stable lock file inside the state
// directory. It is never unlinked or replaced while in use.
const MaintenanceLockFileName = "maintenance.lock"

// lockWaitDeadline bounds lock acquisition; after it, holders report
// locked rather than waiting indefinitely.
const lockWaitDeadline = 5 * time.Second

// ErrLocked is returned when another process holds the maintenance lock.
var ErrLocked = errors.New("store: state is locked by another process")

// ErrSymlink is returned when a state path component, the lock file or the
// database file is a symlink.
var ErrSymlink = errors.New("store: refusing a symlinked state path")

// LockMode selects the maintenance lock class.
type LockMode int

const (
	// LockShared is held by every process with an open database for the
	// whole database lifetime.
	LockShared LockMode = iota
	// LockExclusive is held by init, migration and restore before opening
	// or replacing the database.
	LockExclusive
)

// MaintenanceLock is an advisory flock on the stable maintenance.lock file.
type MaintenanceLock struct {
	path string
	file *os.File
	mode LockMode
}

// AcquireMaintenanceLock acquires the maintenance lock in mode, waiting at
// most lockWaitDeadline for competing holders and then failing with
// ErrLocked. Existing symlinked lock files and symlinked path components
// are rejected.
func AcquireMaintenanceLock(stateDir string, mode LockMode) (*MaintenanceLock, error) {
	return acquireMaintenanceLock(stateDir, mode, lockWaitDeadline)
}

func acquireMaintenanceLock(stateDir string, mode LockMode, deadline time.Duration) (*MaintenanceLock, error) {
	if err := verifyPathChain(stateDir); err != nil {
		return nil, err
	}
	path := filepath.Join(stateDir, MaintenanceLockFileName)
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: %s", ErrSymlink, path)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("store: stat maintenance lock: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("store: open maintenance lock: %w", err)
	}
	flags := unix.LOCK_SH
	if mode == LockExclusive {
		flags = unix.LOCK_EX
	}
	if err := flockWithDeadline(f, flags, deadline); err != nil {
		f.Close()
		return nil, err
	}
	return &MaintenanceLock{path: path, file: f, mode: mode}, nil
}

// flockWithDeadline retries a non-blocking flock until the deadline and
// returns ErrLocked when a competing holder remains.
func flockWithDeadline(f *os.File, flags int, deadline time.Duration) error {
	deadlineAt := time.Now().Add(deadline)
	for {
		if err := unix.Flock(int(f.Fd()), flags|unix.LOCK_NB); err == nil {
			return nil
		} else if err != unix.EWOULDBLOCK && err != unix.EAGAIN {
			return fmt.Errorf("store: flock: %w", err)
		}
		if time.Now().Add(20 * time.Millisecond).After(deadlineAt) {
			return ErrLocked
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Path returns the locked file path.
func (l *MaintenanceLock) Path() string { return l.path }

// Mode returns the held lock class.
func (l *MaintenanceLock) Mode() LockMode { return l.mode }

// Release unlocks and closes the lock file without removing it.
func (l *MaintenanceLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil
	if err != nil {
		return err
	}
	return closeErr
}

// verifyPathChain rejects the state dir when it or any existing parent is
// a symlink, so directory creation and lock placement cannot be
// redirected through a link.
func verifyPathChain(stateDir string) error {
	if !filepath.IsAbs(stateDir) {
		return fmt.Errorf("store: state dir %q must be an absolute path", stateDir)
	}
	p := filepath.Clean(stateDir)
	for {
		if info, err := os.Lstat(p); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("%w: %s", ErrSymlink, p)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("store: stat %s: %w", p, err)
		}
		parent := filepath.Dir(p)
		if parent == p || parent == string(filepath.Separator) {
			return nil
		}
		p = parent
	}
}
