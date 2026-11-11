package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Schema boundary errors. A normal opener reports them instead of
// migrating; init and migrate run under the exclusive maintenance lock.
var (
	ErrSchemaMissing = errors.New("database schema is missing; run media-core init")
	ErrSchemaPending = errors.New("database schema is behind this build; run media-core migrate")
	ErrSchemaNewer   = errors.New("database schema is newer than this build; use a newer version of media-core")
)

// ledgerDDL is the runner-managed applied-migrations ledger. It is not part
// of the versioned domain schema.
const ledgerDDL = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version    INTEGER PRIMARY KEY,
	name       TEXT NOT NULL,
	applied_at TEXT NOT NULL
)`

// migration is one embedded, versioned SQL script.
type migration struct {
	Version int
	Name    string
	SQL     string
}

// loadMigrations reads and validates the embedded migration set. File names
// must be NNNN_name.sql with unique, positive versions; the set is returned
// in version order. Migration files are append-only after committed use.
func loadMigrations() ([]migration, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("store: read embedded migrations: %w", err)
	}
	var migs []migration
	seen := map[int]bool{}
	for _, e := range entries {
		if e.IsDir() {
			return nil, fmt.Errorf("store: unexpected directory %q in embedded migrations", e.Name())
		}
		base := path.Base(e.Name())
		version, ok := parseMigrationName(base)
		if !ok {
			return nil, fmt.Errorf("store: migration file %q must be named NNNN_name.sql", base)
		}
		if seen[version] {
			return nil, fmt.Errorf("store: duplicate migration version %d", version)
		}
		seen[version] = true
		raw, err := migrationFS.ReadFile("migrations/" + base)
		if err != nil {
			return nil, fmt.Errorf("store: read migration %q: %w", base, err)
		}
		migs = append(migs, migration{Version: version, Name: base, SQL: string(raw)})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].Version < migs[j].Version })
	return migs, nil
}

// parseMigrationName extracts the version from a NNNN_name.sql file name.
func parseMigrationName(name string) (int, bool) {
	if !strings.HasSuffix(name, ".sql") {
		return 0, false
	}
	prefix := strings.TrimSuffix(name, ".sql")
	idx := strings.Index(prefix, "_")
	if idx != 4 {
		return 0, false
	}
	if prefix[idx+1:] == "" {
		return 0, false
	}
	v, err := strconv.Atoi(prefix[:4])
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// ensureLedger creates the applied-migrations ledger if it is missing.
func ensureLedger(db *sql.DB) error {
	if _, err := db.Exec(ledgerDDL); err != nil {
		return fmt.Errorf("store: create schema ledger: %w", err)
	}
	return nil
}

// appliedVersions returns the ledger contents as a set of applied versions.
func appliedVersions(db *sql.DB) (map[int]bool, error) {
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("store: read schema ledger: %w", err)
	}
	defer rows.Close()
	versions := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("store: read schema ledger: %w", err)
		}
		versions[v] = true
	}
	return versions, rows.Err()
}

// currentSchemaVersion is the highest embedded migration version.
func currentSchemaVersion() (int, error) {
	migs, err := loadMigrations()
	if err != nil {
		return 0, err
	}
	if len(migs) == 0 {
		return 0, errors.New("store: no embedded migrations")
	}
	return migs[len(migs)-1].Version, nil
}

// RunMigrations applies every pending embedded migration once, under the
// exclusive maintenance lock held by the caller. Each migration runs in its
// own transaction together with its ledger row, so a failed migration rolls
// back and leaves the previous schema and data intact. It never re-runs
// applied migrations and never downgrades: a ledger newer than the embedded
// set is rejected. It returns the schema version after the run.
func RunMigrations(db *sql.DB) (int, error) {
	if err := ensureLedger(db); err != nil {
		return 0, err
	}
	migs, err := loadMigrations()
	if err != nil {
		return 0, err
	}
	current, err := currentSchemaVersion()
	if err != nil {
		return 0, err
	}
	applied, err := appliedVersions(db)
	if err != nil {
		return 0, err
	}
	for v := range applied {
		if v > current {
			return 0, fmt.Errorf("store: %w", ErrSchemaNewer)
		}
	}
	version := 0
	for _, m := range migs {
		version = m.Version
		if applied[m.Version] {
			continue
		}
		if err := applyMigration(db, m); err != nil {
			return 0, fmt.Errorf("store: migration %s: %w", m.Name, err)
		}
	}
	return version, nil
}

// applyMigration executes one migration script and its ledger row in a
// single transaction.
func applyMigration(db *sql.DB, m migration) error {
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("store: begin migration: %w", err)
	}
	if _, err := tx.Exec(m.SQL); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store: execute migration: %w", err)
	}
	appliedAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.Version, m.Name, appliedAt); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store: record migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit migration: %w", err)
	}
	return nil
}

// CheckSchema verifies that the database sits at exactly the current schema
// version. It never migrates: a missing, pending, or newer schema is
// rejected with a distinct error so the caller can report guidance instead
// of hot-upgrading under a running server.
func CheckSchema(db *sql.DB) error {
	var count int
	var max int
	err := db.QueryRow(`SELECT COUNT(*), COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&count, &max)
	if err != nil {
		if isMissingTable(err) {
			return fmt.Errorf("store: %w", ErrSchemaMissing)
		}
		return fmt.Errorf("store: read schema ledger: %w", err)
	}
	current, err := currentSchemaVersion()
	if err != nil {
		return err
	}
	return schemaStateError(count, max, current)
}

// schemaStateError decides the boundary for a ledger state.
func schemaStateError(count, max, current int) error {
	switch {
	case count == 0:
		return ErrSchemaMissing
	case max < current:
		return ErrSchemaPending
	case max > current:
		return ErrSchemaNewer
	}
	return nil
}

// isMissingTable recognizes the driver's "no such table" failure.
func isMissingTable(err error) bool {
	return strings.Contains(err.Error(), "no such table")
}
