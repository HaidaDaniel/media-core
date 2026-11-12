package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/HaidaDaniel/media-core/internal/store"
)

// initState initializes one state directory and returns it with an open
// live handle, the way a normal process would hold it.
func initState(t *testing.T) (string, *sql.DB) {
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
		t.Fatalf("Close: %v", err)
	}
	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return dir, db.DB()
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func mustQuery(t *testing.T, db *sql.DB, query string, args ...any) []any {
	t.Helper()
	rows, err := db.Query(query, args...)
	if err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("columns: %v", err)
	}
	var out []any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, vals...)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return out
}

func fileHash(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func openArchiveDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open archive db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func archiveEntries(t *testing.T, path string) []string {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// TestBackupCreate publishes one archive and checks the layout, the
// manifest metadata and the snapshot content against the live source.
func TestBackupCreate(t *testing.T) {
	dir, db := initState(t)
	mustExec(t, db, `CREATE TABLE backup_probe (id INTEGER PRIMARY KEY, n TEXT)`)
	for i := 1; i <= 5; i++ {
		mustExec(t, db, `INSERT INTO backup_probe (id, n) VALUES (?, ?)`, i, fmt.Sprintf("row-%d", i))
	}
	var liveSchema int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&liveSchema); err != nil {
		t.Fatalf("read schema: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "dest")
	res, err := Create(context.Background(), Options{StateDir: dir, DestDir: dest, Name: "snap1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if want := filepath.Join(dest, "snap1"); res.Path != want {
		t.Fatalf("Path = %q, want %q", res.Path, want)
	}
	if got := archiveEntries(t, res.Path); len(got) != 2 || got[0] != ManifestFileName || got[1] != DBFileName {
		t.Fatalf("archive entries = %v, want exactly [%s %s]", got, ManifestFileName, DBFileName)
	}
	if res.Manifest.Format != ArchiveFormatName {
		t.Fatalf("Format = %q, want %q", res.Manifest.Format, ArchiveFormatName)
	}
	if res.Manifest.Schema != liveSchema {
		t.Fatalf("Schema = %d, want %d", res.Manifest.Schema, liveSchema)
	}
	if res.Manifest.DB.Name != DBFileName || res.Manifest.DB.Size <= 0 {
		t.Fatalf("DB = %+v, want name %q and positive size", res.Manifest.DB, DBFileName)
	}
	if got := fileHash(t, filepath.Join(res.Path, DBFileName)); got != res.Manifest.DB.SHA256 {
		t.Fatalf("manifest SHA256 = %q, want %q", res.Manifest.DB.SHA256, got)
	}
	if res.Manifest.App.Version == "" || res.Manifest.App.GoVersion == "" {
		t.Fatalf("App = %+v, want identity fields", res.Manifest.App)
	}
	if time.Since(res.Manifest.CreatedAt) > time.Minute {
		t.Fatalf("CreatedAt = %v, want recent", res.Manifest.CreatedAt)
	}

	snap := openArchiveDB(t, filepath.Join(res.Path, DBFileName))
	counts := mustQuery(t, snap, `SELECT COUNT(*) FROM backup_probe`)
	if n, _ := counts[0].(int64); n != 5 {
		t.Fatalf("snapshot probe count = %v, want 5", n)
	}
	// The source is untouched and still serves the same data.
	counts = mustQuery(t, db, `SELECT COUNT(*) FROM backup_probe`)
	if n, _ := counts[0].(int64); n != 5 {
		t.Fatalf("source probe count = %v, want 5", n)
	}

	// Archive file modes follow the state directory convention.
	if info, err := os.Stat(res.Path); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("archive dir mode = %v (%v), want 0700", info.Mode().Perm(), err)
	}
	for _, name := range []string{DBFileName, ManifestFileName} {
		if info, err := os.Stat(filepath.Join(res.Path, name)); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %v (%v), want 0600", name, info.Mode().Perm(), err)
		}
	}
}

// TestBackupIncludesWALCommits proves the archive is a consistent snapshot
// of the committed state, not a bare copy of the live database file: rows
// that are committed but still in the WAL must appear in the archive, and
// the archived file must differ from the live file bytes.
func TestBackupIncludesWALCommits(t *testing.T) {
	dir, db := initState(t)
	mustExec(t, db, `CREATE TABLE backup_probe (id INTEGER PRIMARY KEY, n TEXT)`)
	for i := 1; i <= 5; i++ {
		mustExec(t, db, `INSERT INTO backup_probe (id, n) VALUES (?, ?)`, i, fmt.Sprintf("row-%d", i))
	}
	walInfo, err := os.Stat(filepath.Join(dir, store.DBFileName+"-wal"))
	if err != nil || walInfo.Size() == 0 {
		t.Fatalf("WAL sidecar missing or empty before backup (size %v, err %v); test premise is committed-but-uncheckpointed data", walInfo.Size(), err)
	}
	liveHash := fileHash(t, filepath.Join(dir, store.DBFileName))

	dest := filepath.Join(t.TempDir(), "dest")
	res, err := Create(context.Background(), Options{StateDir: dir, DestDir: dest, Name: "wal"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	snapPath := filepath.Join(res.Path, DBFileName)
	if res.Manifest.DB.SHA256 == liveHash {
		t.Fatalf("archive hash equals the live file hash; the WAL commits were not folded into the snapshot")
	}
	snap := openArchiveDB(t, snapPath)
	counts := mustQuery(t, snap, `SELECT COUNT(*) FROM backup_probe`)
	if n, _ := counts[0].(int64); n != 5 {
		t.Fatalf("snapshot probe count = %v, want 5 including the WAL commits", n)
	}
}

// TestBackupConcurrentWrites runs a steady writer while a backup happens
// and checks the snapshot consistency property: a single writer's
// committed rows appear in the archive as a contiguous prefix.
func TestBackupConcurrentWrites(t *testing.T) {
	dir, db := initState(t)
	mustExec(t, db, `CREATE TABLE backup_probe (id INTEGER PRIMARY KEY, n TEXT)`)
	total := 50

	type outcome struct {
		res *Result
		err error
	}
	outCh := make(chan outcome, 1)
	go func() {
		dest := filepath.Join(t.TempDir(), "dest")
		res, err := Create(context.Background(), Options{StateDir: dir, DestDir: dest, Name: "concurrent"})
		outCh <- outcome{res: res, err: err}
	}()
	for i := 1; i <= total; i++ {
		mustExec(t, db, `INSERT INTO backup_probe (id, n) VALUES (?, ?)`, i, fmt.Sprintf("row-%d", i))
		time.Sleep(time.Millisecond)
	}
	out := <-outCh
	if out.err != nil {
		t.Fatalf("Create under concurrent writes: %v", out.err)
	}

	snap := openArchiveDB(t, filepath.Join(out.res.Path, DBFileName))
	raw := mustQuery(t, snap, `SELECT id FROM backup_probe ORDER BY id`)
	ids := make([]int, 0, len(raw))
	for _, v := range raw {
		ids = append(ids, int(v.(int64)))
	}
	for i, id := range ids {
		if id != i+1 {
			t.Fatalf("archived ids %v are not a contiguous prefix 1..%d; the snapshot is not a consistent committed state", ids, len(ids))
		}
	}
	if len(ids) > total {
		t.Fatalf("archived %d rows, want at most %d", len(ids), total)
	}
	live := mustQuery(t, db, `SELECT COUNT(*) FROM backup_probe`)
	if n, _ := live[0].(int64); n != int64(total) {
		t.Fatalf("source count = %v, want %d", n, total)
	}
}

// TestBackupRefusesOverwrite publishes, refuses a second publish into the
// same name, and replaces only when the overwrite is explicit.
func TestBackupRefusesOverwrite(t *testing.T) {
	dir, db := initState(t)
	mustExec(t, db, `CREATE TABLE backup_probe (id INTEGER PRIMARY KEY, n TEXT)`)
	mustExec(t, db, `INSERT INTO backup_probe (id, n) VALUES (1, 'one')`)
	dest := filepath.Join(t.TempDir(), "dest")

	first, err := Create(context.Background(), Options{StateDir: dir, DestDir: dest, Name: "a"})
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	firstManifest, err := os.ReadFile(filepath.Join(first.Path, ManifestFileName))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	_, err = Create(context.Background(), Options{StateDir: dir, DestDir: dest, Name: "a"})
	if !errors.Is(err, ErrExists) {
		t.Fatalf("second Create err = %v, want ErrExists", err)
	}
	again, err := os.ReadFile(filepath.Join(first.Path, ManifestFileName))
	if err != nil || !bytes.Equal(firstManifest, again) {
		t.Fatalf("existing archive changed on refusal (err %v)", err)
	}
	// A refused publish must not leave staging debris in the destination.
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".staging-") {
			t.Fatalf("staging leftover %q in destination", e.Name())
		}
	}

	mustExec(t, db, `INSERT INTO backup_probe (id, n) VALUES (2, 'two')`)
	second, err := Create(context.Background(), Options{StateDir: dir, DestDir: dest, Name: "a", Overwrite: true})
	if err != nil {
		t.Fatalf("overwriting Create: %v", err)
	}
	if second.Path != first.Path {
		t.Fatalf("overwrite Path = %q, want %q", second.Path, first.Path)
	}
	entries, err = os.ReadDir(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "a" {
			t.Fatalf("destination entry %q after overwrite, want only the archive (no set-aside debris)", e.Name())
		}
	}
	snap := openArchiveDB(t, filepath.Join(second.Path, DBFileName))
	counts := mustQuery(t, snap, `SELECT COUNT(*) FROM backup_probe`)
	if n, _ := counts[0].(int64); n != 2 {
		t.Fatalf("replaced snapshot count = %v, want 2", n)
	}
}

// TestBackupFailureLeavesNoTrace exercises each failure path and checks
// the source is untouched and no apparently complete archive or staging
// debris remains.
func TestBackupFailureLeavesNoTrace(t *testing.T) {
	t.Run("missing source", func(t *testing.T) {
		dir := t.TempDir()
		dest := filepath.Join(t.TempDir(), "dest")
		_, err := Create(context.Background(), Options{StateDir: dir, DestDir: dest, Name: "x"})
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("err = %v, want a missing-source failure", err)
		}
		entries, _ := os.ReadDir(dest)
		for _, e := range entries {
			t.Fatalf("destination entry %q after missing-source failure", e.Name())
		}
	})
	t.Run("destination is a file", func(t *testing.T) {
		dir, _ := initState(t)
		dest := filepath.Join(t.TempDir(), "dest")
		if err := os.WriteFile(dest, []byte("not a dir"), 0o600); err != nil {
			t.Fatalf("write dest file: %v", err)
		}
		_, err := Create(context.Background(), Options{StateDir: dir, DestDir: dest, Name: "x"})
		if err == nil {
			t.Fatalf("Create into a file destination succeeded, want failure")
		}
		if info, err := os.Lstat(dest); err != nil || info.IsDir() {
			t.Fatalf("destination file was replaced (err %v)", err)
		}
	})
	t.Run("destination inside state dir", func(t *testing.T) {
		dir, db := initState(t)
		mustExec(t, db, `CREATE TABLE backup_probe (id INTEGER PRIMARY KEY)`)
		mustExec(t, db, `INSERT INTO backup_probe (id) VALUES (1)`)
		cases := []Options{
			{StateDir: dir, DestDir: dir, Name: "inside"},
			{StateDir: dir, DestDir: dir, Name: store.DBFileName, Overwrite: true},
			{StateDir: dir, DestDir: filepath.Join(dir, "nested"), Name: "x"},
		}
		for _, opts := range cases {
			_, err := Create(context.Background(), opts)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Create(%+v) err = %v, want ErrInvalid", opts, err)
			}
		}
		// The destructive combination (overwrite a colliding state file)
		// must leave the source intact and openable.
		counts := mustQuery(t, db, `SELECT COUNT(*) FROM backup_probe`)
		if n, _ := counts[0].(int64); n != 1 {
			t.Fatalf("source probe count = %v, want 1; the source was touched", n)
		}
		if _, err := os.Stat(filepath.Join(dir, store.DBFileName)); err != nil {
			t.Fatalf("source database missing: %v", err)
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.Name() == "inside" || e.Name() == "nested" {
				t.Fatalf("archive %q published inside the state dir", e.Name())
			}
		}
	})
	t.Run("symlinked destination into state dir", func(t *testing.T) {
		dir, db := initState(t)
		mustExec(t, db, `CREATE TABLE backup_probe (id INTEGER PRIMARY KEY)`)
		mustExec(t, db, `INSERT INTO backup_probe (id) VALUES (1)`)
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(dir, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		cases := []Options{
			{StateDir: dir, DestDir: link, Name: store.DBFileName, Overwrite: true},
			{StateDir: dir, DestDir: filepath.Join(link, "sub"), Name: "x"},
		}
		for _, opts := range cases {
			_, err := Create(context.Background(), opts)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Create(%+v) err = %v, want ErrInvalid", opts, err)
			}
		}
		// The destructive combination must leave the source intact.
		counts := mustQuery(t, db, `SELECT COUNT(*) FROM backup_probe`)
		if n, _ := counts[0].(int64); n != 1 {
			t.Fatalf("source probe count = %v, want 1; the source was touched", n)
		}
	})
	t.Run("symlinked destination outside state dir", func(t *testing.T) {
		dir, _ := initState(t)
		target := filepath.Join(t.TempDir(), "real")
		if err := os.MkdirAll(target, 0o700); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		res, err := Create(context.Background(), Options{StateDir: dir, DestDir: link, Name: "via-link"})
		if err != nil {
			t.Fatalf("Create through a legitimate link: %v", err)
		}
		if !strings.HasPrefix(res.Path, target) {
			t.Fatalf("archive published at %q, want under %q", res.Path, target)
		}
	})
	t.Run("invalid options", func(t *testing.T) {
		dir, _ := initState(t)
		cases := []Options{
			{StateDir: "relative", DestDir: "/abs", Name: "x"},
			{StateDir: dir, DestDir: "relative", Name: "x"},
			{StateDir: dir, DestDir: t.TempDir(), Name: "a/b"},
		}
		for _, opts := range cases {
			_, err := Create(context.Background(), opts)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Create(%+v) err = %v, want ErrInvalid", opts, err)
			}
		}
	})
	t.Run("locked source", func(t *testing.T) {
		// The state is initialized but no live handle is held, so the test
		// alone can take the exclusive lock the way init or restore would.
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
		lockFile, err := os.OpenFile(filepath.Join(dir, store.MaintenanceLockFileName), os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatalf("open lock file: %v", err)
		}
		defer lockFile.Close()
		if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
			t.Fatalf("hold exclusive lock: %v", err)
		}
		defer unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)
		dest := filepath.Join(t.TempDir(), "dest")
		_, err = Create(context.Background(), Options{StateDir: dir, DestDir: dest, Name: "x"})
		if !errors.Is(err, store.ErrLocked) {
			t.Fatalf("Create on a locked state err = %v, want ErrLocked", err)
		}
		entries, _ := os.ReadDir(dest)
		for _, e := range entries {
			t.Fatalf("destination entry %q after locked-source failure", e.Name())
		}
	})
}

// TestBackupExcludesStateContents proves the archive packages only the
// database and the manifest: other state directory contents, including
// secret material, never appear in the archive.
func TestBackupExcludesStateContents(t *testing.T) {
	dir, _ := initState(t)
	for _, name := range []string{"owner-secret", "sessions.json", "tls.key"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("topsecret-material-"+name), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	dest := filepath.Join(t.TempDir(), "dest")
	res, err := Create(context.Background(), Options{StateDir: dir, DestDir: dest, Name: "clean"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := archiveEntries(t, res.Path); len(got) != 2 || got[0] != ManifestFileName || got[1] != DBFileName {
		t.Fatalf("archive entries = %v, want exactly the database and the manifest", got)
	}
	manifest, err := os.ReadFile(filepath.Join(res.Path, ManifestFileName))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if bytes.Contains(manifest, []byte("topsecret-material")) {
		t.Fatalf("manifest carries state directory secret material")
	}
}
