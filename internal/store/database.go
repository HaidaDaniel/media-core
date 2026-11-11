package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DBFileName is the fixed database file name inside the state directory.
const DBFileName = "media-core.db"

// busyTimeoutMs is the per-connection busy timeout required by the
// architecture.
const busyTimeoutMs = 5000

// synchronousFull is the PRAGMA synchronous value for FULL.
const synchronousFull = 2

// schemaCheck is the schema boundary for normal opens: it verifies the
// database sits at exactly the current schema version and never migrates.
// Missing, pending, and newer schemas are rejected; only the exclusive
// maintenance session (init/migrate) runs migrations.
var schemaCheck func(*sql.DB) error = CheckSchema

// Database is an opened, fully configured SQLite connection set. It holds
// the lifetime maintenance lock until Close.
type Database struct {
	db   *sql.DB
	path string
	lock *MaintenanceLock
}

// Open opens the fixed database under the shared lifetime lock for normal
// startup. It requires an initialized, current schema: a missing, pending,
// or newer schema is rejected and never migrated.
func Open(stateDir string) (*Database, error) {
	return open(stateDir, LockShared)
}

// OpenForMaintenance opens the database under the exclusive maintenance
// lock for init, migration and restore. It performs no schema check; the
// caller runs the migrations.
func OpenForMaintenance(stateDir string) (*Database, error) {
	return open(stateDir, LockExclusive)
}

func open(stateDir string, mode LockMode) (*Database, error) {
	// The chain is verified before creation so an existing symlinked
	// component can never redirect the state dir; creation only fills in
	// missing real directories.
	if err := verifyPathChain(stateDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("store: create state dir: %w", err)
	}
	lock, err := AcquireMaintenanceLock(stateDir, mode)
	if err != nil {
		return nil, err
	}
	d := &Database{lock: lock}
	// The schema boundary guards normal startup only: the exclusive
	// maintenance session is what creates and migrates the schema, so it
	// must be able to open an uninitialized database.
	checkSchema := mode == LockShared
	if err := d.configure(stateDir, checkSchema); err != nil {
		lock.Release()
		return nil, err
	}
	return d, nil
}

// configure opens the driver connection with the bounded pool, verifies the
// per-connection pragmas took effect, and (for normal opens) enforces the
// schema boundary. Any failure closes the connection so no partially
// configured connection escapes.
func (d *Database) configure(stateDir string, checkSchema bool) error {
	path := filepath.Join(stateDir, DBFileName)
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrSymlink, path)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("store: stat database: %w", err)
	}
	db, err := sql.Open("sqlite", dsnFor(path))
	if err != nil {
		return fmt.Errorf("store: open database: %w", err)
	}
	d.db = db
	// Bounded pool: one connection per process.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := d.verifyPragmas(); err != nil {
		db.Close()
		d.db = nil
		return err
	}
	if checkSchema {
		if err := schemaCheck(db); err != nil {
			db.Close()
			d.db = nil
			return err
		}
	}
	// Secure file modes on the state directory, the maintenance lock,
	// the database and its sidecars; pre-existing loose modes are
	// tightened on every open.
	if err := os.Chmod(stateDir, 0o700); err != nil {
		db.Close()
		d.db = nil
		return fmt.Errorf("store: chmod state dir: %w", err)
	}
	for _, name := range []string{d.lock.Path(), path, path + "-wal", path + "-shm"} {
		if info, err := os.Lstat(name); err == nil && info.Mode()&os.ModeSymlink == 0 {
			if err := os.Chmod(name, 0o600); err != nil {
				db.Close()
				d.db = nil
				return fmt.Errorf("store: chmod %s: %w", name, err)
			}
		}
	}
	d.path = path
	return nil
}

// dsnFor builds the driver DSN with the per-connection pragmas; the
// modernc driver applies _pragma parameters to every new connection.
// _txlock=immediate is the driver's transaction-mode parameter (it ignores
// sql.TxOptions.Isolation): every non-readonly transaction started by the
// pool issues BEGIN IMMEDIATE, as the architecture requires for short write
// transactions. Plain autocommit reads are unaffected.
func dsnFor(path string) string {
	u := &url.URL{Scheme: "file", Path: path}
	return u.String() +
		"?_pragma=journal_mode(wal)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=synchronous(full)" +
		"&_pragma=busy_timeout(5000)" +
		"&_txlock=immediate"
}

// verifyPragmas proves the required per-connection configuration is
// effective on the live connection.
func (d *Database) verifyPragmas() error {
	var journalMode string
	if err := d.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		return fmt.Errorf("store: journal_mode: %w", err)
	}
	if journalMode != "wal" {
		return fmt.Errorf("store: journal_mode = %q, want wal", journalMode)
	}
	var foreignKeys int
	if err := d.db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("store: foreign_keys: %w", err)
	}
	if foreignKeys != 1 {
		return fmt.Errorf("store: foreign_keys = %d, want 1", foreignKeys)
	}
	var synchronous int
	if err := d.db.QueryRow("PRAGMA synchronous").Scan(&synchronous); err != nil {
		return fmt.Errorf("store: synchronous: %w", err)
	}
	if synchronous != synchronousFull {
		return fmt.Errorf("store: synchronous = %d, want FULL (%d)", synchronous, synchronousFull)
	}
	var busyTimeout int
	if err := d.db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		return fmt.Errorf("store: busy_timeout: %w", err)
	}
	if busyTimeout != busyTimeoutMs {
		return fmt.Errorf("store: busy_timeout = %d, want %d", busyTimeout, busyTimeoutMs)
	}
	return nil
}

// DB returns the bounded database handle.
func (d *Database) DB() *sql.DB { return d.db }

// Path returns the absolute database file path.
func (d *Database) Path() string { return d.path }

// Lock returns the held lifetime maintenance lock.
func (d *Database) Lock() *MaintenanceLock { return d.lock }

// Close closes the database and releases the lifetime lock.
func (d *Database) Close() error {
	var err error
	if d.db != nil {
		err = d.db.Close()
		d.db = nil
	}
	if d.lock != nil {
		if releaseErr := d.lock.Release(); releaseErr != nil && err == nil {
			err = releaseErr
		}
		d.lock = nil
	}
	return err
}
