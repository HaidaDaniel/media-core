// Migration integrity: every committed append-only migration through
// 0014_artwork must bring a database stopped at any prior schema
// version forward to the current version without losing a single row, the
// upgrade must be ordered and idempotent, and a backup snapshot of the
// migrated schema must be internally consistent (integrity check, schema
// version, SHA-256) and restore to an equivalent state.
package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/HaidaDaniel/media-core/internal/backup"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// ledgerDDL mirrors the runner-managed applied-migrations ledger in
// internal/store/migrations.go so a prior-version fixture can be built with
// only exported store symbols plus the on-disk migration scripts.
const ledgerDDL = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version    INTEGER PRIMARY KEY,
	name       TEXT NOT NULL,
	applied_at TEXT NOT NULL
)`

// migFile is one on-disk migration script with its parsed version.
type migFile struct {
	Version int
	Name    string
	Path    string
}

// loadMigrationFiles reads the embedded migration scripts from disk (the
// store's embed FS is unexported) and returns them in version order.
func loadMigrationFiles(t *testing.T) []migFile {
	t.Helper()
	dir := filepath.Join("..", "..", "internal", "store", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	var out []migFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		v, ok := parseVersion(e.Name())
		if !ok {
			t.Fatalf("unexpected migration file %q", e.Name())
		}
		out = append(out, migFile{Version: v, Name: e.Name(), Path: filepath.Join(dir, e.Name())})
	}
	if len(out) == 0 {
		t.Fatal("no migration files found")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out
}

// parseVersion extracts the NNNN version prefix from a NNNN_name.sql name.
func parseVersion(name string) (int, bool) {
	base := strings.TrimSuffix(name, ".sql")
	idx := strings.Index(base, "_")
	if idx != 4 {
		return 0, false
	}
	v, err := strconv.Atoi(base[:4])
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// openRaw opens an unconfigured driver connection on the fixed database file
// name inside dir.
func openRaw(t *testing.T, dir string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, store.DBFileName))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// buildFixture opens a raw database in dir and applies migrations 1..upTo
// (each with its ledger row in one transaction), leaving the database stopped
// at exactly version upTo.
func buildFixture(t *testing.T, dir string, upTo int) *sql.DB {
	t.Helper()
	db := openRaw(t, dir)
	if _, err := db.Exec(ledgerDDL); err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	for _, m := range loadMigrationFiles(t) {
		if m.Version > upTo {
			break
		}
		raw, err := os.ReadFile(m.Path)
		if err != nil {
			t.Fatalf("read %s: %v", m.Name, err)
		}
		if err := applyOne(t, db, m, string(raw)); err != nil {
			t.Fatalf("apply %s: %v", m.Name, err)
		}
	}
	return db
}

// applyOne executes one migration script and its ledger row in a single
// transaction, mirroring the store runner's applyMigration.
func applyOne(t *testing.T, db *sql.DB, m migFile, sqlText string) error {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(sqlText); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.Version, m.Name, time.Now().UTC().Format(time.RFC3339)); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// migrateToCurrent opens the state dir under the exclusive maintenance lock,
// runs the store migrations to the current version, and returns it.
func migrateToCurrent(t *testing.T, dir string) int {
	t.Helper()
	maint, err := store.OpenForMaintenance(dir)
	if err != nil {
		t.Fatalf("OpenForMaintenance: %v", err)
	}
	version, err := store.RunMigrations(maint.DB())
	if err != nil {
		maint.Close()
		t.Fatalf("RunMigrations: %v", err)
	}
	if err := maint.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return version
}

// seedFixture inserts representative rows into the oldest and newest domain
// tables that exist at version upTo, so the upgrade must preserve them.
func seedFixture(t *testing.T, db *sql.DB, upTo int) {
	t.Helper()
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if upTo >= 2 {
		if _, err := db.Exec(`INSERT INTO works (id, revision, created_at, updated_at) VALUES ('work-fixture', 0, ?, ?)`, now, now); err != nil {
			t.Fatalf("seed works: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO work_titles (id, work_id, seq, text, search_key, kind, preferred, provenance, created_at)
			VALUES ('title-fixture', 'work-fixture', 1, 'Fixture Story', 'fixture story', 'original', 1, 'manual', ?)`, now); err != nil {
			t.Fatalf("seed work_titles: %v", err)
		}
	}
	if upTo >= 12 {
		if _, err := db.Exec(`INSERT INTO import_events (id, event_id, schema_version, event_type, raw_payload, canonical_payload, namespace, created_at)
			VALUES ('imp-fixture', 'evt_fixture', '1.0', 'release.added', '{}', '{}', 'default', ?)`, now); err != nil {
			t.Fatalf("seed import_events: %v", err)
		}
	}
}

// assertLedgerComplete proves the upgraded database records every migration
// version exactly once.
func assertLedgerComplete(t *testing.T, dir string, current int) {
	t.Helper()
	db := openRaw(t, dir)
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if count != current {
		t.Fatalf("ledger rows = %d, want %d", count, current)
	}
	for v := 1; v <= current; v++ {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, v).Scan(&n); err != nil {
			t.Fatalf("read ledger v%d: %v", v, err)
		}
		if n != 1 {
			t.Fatalf("ledger version %d rows = %d, want exactly 1", v, n)
		}
	}
}

// assertSeedSurvives proves the rows seeded at the prior version survived the
// upgrade intact (no row loss).
func assertSeedSurvives(t *testing.T, dir string, upTo int) {
	t.Helper()
	db := openRaw(t, dir)
	if upTo >= 2 {
		for table, id := range map[string]string{"works": "work-fixture", "work_titles": "title-fixture"} {
			var n int
			if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE id = ?`, id).Scan(&n); err != nil {
				t.Fatalf("read %s: %v", table, err)
			}
			if n != 1 {
				t.Fatalf("%s fixture rows = %d, want 1 (data lost during upgrade)", table, n)
			}
		}
	}
	if upTo >= 12 {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM import_events WHERE id = 'imp-fixture'`).Scan(&n); err != nil {
			t.Fatalf("read import_events: %v", err)
		}
		if n != 1 {
			t.Fatalf("import_events fixture rows = %d, want 1 (data lost during upgrade)", n)
		}
	}
}

// TestMigrationsFromEachPriorFixture proves the acceptance criterion: from
// every prior schema version the append-only migrations reach the current
// version without losing any row.
func TestMigrationsFromEachPriorFixture(t *testing.T) {
	files := loadMigrationFiles(t)
	current := files[len(files)-1].Version
	for upTo := 1; upTo < current; upTo++ {
		upTo := upTo
		t.Run(fmt.Sprintf("from_v%04d", upTo), func(t *testing.T) {
			dir := t.TempDir()
			db := buildFixture(t, dir, upTo)
			seedFixture(t, db, upTo)
			if err := db.Close(); err != nil {
				t.Fatalf("close fixture: %v", err)
			}
			version := migrateToCurrent(t, dir)
			if version != current {
				t.Fatalf("schema version = %d, want %d", version, current)
			}
			assertLedgerComplete(t, dir, current)
			assertSeedSurvives(t, dir, upTo)
		})
	}
}

// TestMigrationsUpgradeOrderedAndIdempotent proves the upgrade lands at the
// current version with the complete ledger and that re-running the migrations
// is a no-op (the ledger does not grow and no migration re-applies).
func TestMigrationsUpgradeOrderedAndIdempotent(t *testing.T) {
	files := loadMigrationFiles(t)
	current := files[len(files)-1].Version
	dir := t.TempDir()
	db := buildFixture(t, dir, 5)
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
	if version := migrateToCurrent(t, dir); version != current {
		t.Fatalf("schema version = %d, want %d", version, current)
	}
	assertLedgerComplete(t, dir, current)
	before := ledgerRowCount(t, dir)

	maint, err := store.OpenForMaintenance(dir)
	if err != nil {
		t.Fatalf("OpenForMaintenance: %v", err)
	}
	version, err := store.RunMigrations(maint.DB())
	if err != nil {
		maint.Close()
		t.Fatalf("re-run RunMigrations: %v", err)
	}
	if err := maint.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if version != current {
		t.Fatalf("re-run schema version = %d, want %d", version, current)
	}
	after := ledgerRowCount(t, dir)
	if before != after || after != current {
		t.Fatalf("ledger rows before=%d after=%d, want %d (re-run must be a no-op)", before, after, current)
	}
}

func ledgerRowCount(t *testing.T, dir string) int {
	t.Helper()
	db := openRaw(t, dir)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	return n
}

// TestBackupSnapshotIntegrity proves a backup of the migrated schema is
// internally consistent (integrity check ok, schema version recorded, SHA-256
// matching the archived file) even with a live writer open, and that restoring
// it yields an equivalent state with the same data.
func TestBackupSnapshotIntegrity(t *testing.T) {
	files := loadMigrationFiles(t)
	current := files[len(files)-1].Version
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
		t.Fatalf("Close: %v", err)
	}
	// A live shared handle stays open for the whole snapshot: the backup is
	// taken under the shared lock, so writers may keep running.
	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if _, err := db.DB().Exec(`INSERT INTO works (id, revision, created_at, updated_at) VALUES ('work-snap', 0, ?, ?)`, now, now); err != nil {
		t.Fatalf("seed work: %v", err)
	}
	if _, err := db.DB().Exec(`INSERT INTO work_titles (id, work_id, seq, text, search_key, kind, preferred, provenance, created_at)
		VALUES ('title-snap', 'work-snap', 1, 'Snapshot Story', 'snapshot story', 'original', 1, 'manual', ?)`, now); err != nil {
		t.Fatalf("seed title: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "snap")
	res, err := backup.Create(context.Background(), backup.Options{StateDir: dir, DestDir: dest, Name: "snap1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Manifest.Format != backup.ArchiveFormatName {
		t.Fatalf("manifest format = %q, want %q", res.Manifest.Format, backup.ArchiveFormatName)
	}
	if res.Manifest.Schema != current {
		t.Fatalf("manifest schema = %d, want %d", res.Manifest.Schema, current)
	}
	if got := sha256OfFile(t, filepath.Join(res.Path, backup.DBFileName)); got != res.Manifest.DB.SHA256 {
		t.Fatalf("archived db sha256 = %s, want the manifest's %s", got, res.Manifest.DB.SHA256)
	}
	snap := openReadonly(t, filepath.Join(res.Path, backup.DBFileName))
	defer snap.Close()
	var ok string
	if err := snap.QueryRow(`PRAGMA integrity_check`).Scan(&ok); err != nil {
		t.Fatalf("integrity_check: %v", err)
	}
	if ok != "ok" {
		t.Fatalf("snapshot integrity_check = %q, want ok", ok)
	}

	restored := filepath.Join(t.TempDir(), "restored")
	rres, err := backup.Restore(context.Background(), backup.RestoreOptions{Archive: res.Path, StateDir: restored})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if rres.Schema != current {
		t.Fatalf("restored schema = %d, want %d", rres.Schema, current)
	}
	ro, err := store.Open(restored)
	if err != nil {
		t.Fatalf("Open restored: %v", err)
	}
	defer ro.Close()
	var n int
	if err := ro.DB().QueryRow(`SELECT COUNT(*) FROM works WHERE id = 'work-snap'`).Scan(&n); err != nil {
		t.Fatalf("read restored work: %v", err)
	}
	if n != 1 {
		t.Fatalf("restored work rows = %d, want 1 (snapshot lost data)", n)
	}
	if err := ro.DB().QueryRow(`SELECT COUNT(*) FROM work_titles WHERE id = 'title-snap'`).Scan(&n); err != nil {
		t.Fatalf("read restored title: %v", err)
	}
	if n != 1 {
		t.Fatalf("restored title rows = %d, want 1 (snapshot lost data)", n)
	}
}

func sha256OfFile(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatalf("hash %s: %v", path, err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func openReadonly(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open readonly: %v", err)
	}
	return db
}
