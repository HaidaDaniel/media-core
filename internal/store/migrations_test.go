package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// openRaw opens an unconfigured driver connection on a temporary database
// file for runner-level tests.
func openRaw(t *testing.T, dir string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, DBFileName))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestMigrationLoadOrdersAndValidates pins the embedded set's shape; update
// it when a task appends a new migration.
func TestMigrationLoadOrdersAndValidates(t *testing.T) {
	migs, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migs) != 5 {
		t.Fatalf("embedded migrations = %d, want 5 (0001 baseline, 0002 works/titles, 0003 editions, 0004 identity, 0005 consumption)", len(migs))
	}
	if migs[0].Version != 1 || migs[0].Name != "0001_schema.sql" {
		t.Fatalf("migration 1 = %+v, want version 1 named 0001_schema.sql", migs[0])
	}
	if migs[1].Version != 2 || migs[1].Name != "0002_works_titles.sql" {
		t.Fatalf("migration 2 = %+v, want version 2 named 0002_works_titles.sql", migs[1])
	}
	if migs[2].Version != 3 || migs[2].Name != "0003_editions.sql" {
		t.Fatalf("migration 3 = %+v, want version 3 named 0003_editions.sql", migs[2])
	}
	if migs[3].Version != 4 || migs[3].Name != "0004_identity.sql" {
		t.Fatalf("migration 4 = %+v, want version 4 named 0004_identity.sql", migs[3])
	}
	if migs[4].Version != 5 || migs[4].Name != "0005_consumption.sql" {
		t.Fatalf("migration 5 = %+v, want version 5 named 0005_consumption.sql", migs[4])
	}
	for _, m := range migs {
		if strings.TrimSpace(m.SQL) == "" {
			t.Fatalf("migration %s SQL is empty", m.Name)
		}
	}
}

func TestMigrationParseName(t *testing.T) {
	cases := []struct {
		name string
		want int
		ok   bool
	}{
		{"0001_schema.sql", 1, true},
		{"0012_works.sql", 12, true},
		{"12345_x.sql", 0, false},
		{"0001.sql", 0, false},
		{"0001_.sql", 0, false},
		{"0000_a.sql", 0, false},
		{"abcd_a.sql", 0, false},
		{"readme.md", 0, false},
	}
	for _, c := range cases {
		got, ok := parseMigrationName(c.name)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseMigrationName(%q) = (%d, %v), want (%d, %v)", c.name, got, ok, c.want, c.ok)
		}
	}
}

// TestMigrationRunAppliesOnce proves a fresh database ends at the current
// schema version with complete, well-formed ledger rows.
func TestMigrationRunAppliesOnce(t *testing.T) {
	db := openRaw(t, t.TempDir())
	version, err := RunMigrations(db)
	if err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if version != 5 {
		t.Fatalf("schema version = %d, want 5", version)
	}
	for _, want := range []struct {
		version int
		name    string
	}{
		{1, "0001_schema.sql"},
		{2, "0002_works_titles.sql"},
		{3, "0003_editions.sql"},
		{4, "0004_identity.sql"},
		{5, "0005_consumption.sql"},
	} {
		var name, appliedAt string
		if err := db.QueryRow(`SELECT name, applied_at FROM schema_migrations WHERE version = ?`, want.version).Scan(&name, &appliedAt); err != nil {
			t.Fatalf("read ledger row %d: %v", want.version, err)
		}
		if name != want.name {
			t.Errorf("ledger name for %d = %q, want %s", want.version, name, want.name)
		}
		if _, err := time.Parse(time.RFC3339, appliedAt); err != nil {
			t.Errorf("ledger applied_at %q is not RFC3339: %v", appliedAt, err)
		}
	}
}

// TestMigrationRunRepeatIsNoop proves an already-applied migration set is
// left untouched: the ledger row count does not grow.
func TestMigrationRunRepeatIsNoop(t *testing.T) {
	db := openRaw(t, t.TempDir())
	if _, err := RunMigrations(db); err != nil {
		t.Fatalf("first RunMigrations: %v", err)
	}
	version, err := RunMigrations(db)
	if err != nil {
		t.Fatalf("second RunMigrations: %v", err)
	}
	if version != 5 {
		t.Fatalf("schema version = %d, want 5", version)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if count != 5 {
		t.Fatalf("ledger rows = %d, want 5", count)
	}
}

// TestMigrationRunRollsBackInvalid proves a failing migration rolls back
// everything it started: no partial tables survive and the ledger is
// unchanged, so the previous schema and data stay intact.
func TestMigrationRunRollsBackInvalid(t *testing.T) {
	db := openRaw(t, t.TempDir())
	if _, err := RunMigrations(db); err != nil {
		t.Fatalf("baseline RunMigrations: %v", err)
	}
	bad := migration{
		Version: 6,
		Name:    "0006_bad.sql",
		SQL:     "CREATE TABLE rollback_probe (x INTEGER); INSERT INTO rollback_probe VALUES (1); SELECT * FROM no_such_table;",
	}
	if err := applyMigration(db, bad); err == nil {
		t.Fatal("invalid migration: want error, got nil")
	}
	var probe int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'rollback_probe'`).Scan(&probe); err != nil {
		t.Fatalf("probe table check: %v", err)
	}
	if probe != 0 {
		t.Fatal("rollback_probe survived the failed migration")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if count != 5 {
		t.Fatalf("ledger rows = %d, want 5 (failed migration must not be recorded)", count)
	}
}

// TestMigrationRunRejectsNewerLedger proves the runner never downgrades or
// silently accepts a ledger from a newer build.
func TestMigrationRunRejectsNewerLedger(t *testing.T) {
	db := openRaw(t, t.TempDir())
	if _, err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (99, '0099_future.sql', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert future ledger row: %v", err)
	}
	if _, err := RunMigrations(db); !errors.Is(err, ErrSchemaNewer) {
		t.Fatalf("RunMigrations on newer ledger: err = %v, want ErrSchemaNewer", err)
	}
}

func TestMigrationCheckSchemaFreshMissing(t *testing.T) {
	db := openRaw(t, t.TempDir())
	if err := CheckSchema(db); !errors.Is(err, ErrSchemaMissing) {
		t.Fatalf("CheckSchema on fresh database: err = %v, want ErrSchemaMissing", err)
	}
}

func TestMigrationCheckSchemaCurrent(t *testing.T) {
	db := openRaw(t, t.TempDir())
	if _, err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if err := CheckSchema(db); err != nil {
		t.Fatalf("CheckSchema on current database: err = %v, want nil", err)
	}
}

func TestMigrationCheckSchemaNewer(t *testing.T) {
	db := openRaw(t, t.TempDir())
	if _, err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (99, '0099_future.sql', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert future ledger row: %v", err)
	}
	if err := CheckSchema(db); !errors.Is(err, ErrSchemaNewer) {
		t.Fatalf("CheckSchema on newer schema: err = %v, want ErrSchemaNewer", err)
	}
}

// TestSchemaStateError pins the boundary decision for every ledger state.
func TestSchemaStateError(t *testing.T) {
	cases := []struct {
		name    string
		count   int
		max     int
		current int
		want    error
	}{
		{"missing", 0, 0, 3, ErrSchemaMissing},
		{"pending", 2, 2, 3, ErrSchemaPending},
		{"newer", 4, 5, 3, ErrSchemaNewer},
		{"current", 3, 3, 3, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := schemaStateError(c.count, c.max, c.current); !errors.Is(err, c.want) {
				t.Fatalf("schemaStateError(%d, %d, %d) = %v, want %v", c.count, c.max, c.current, err, c.want)
			}
		})
	}
}
