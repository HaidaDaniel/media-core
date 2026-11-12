package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/HaidaDaniel/media-core/internal/store"
)

// ErrArchiveInvalid is returned when an archive fails any validation
// before a restore touches its destination.
var ErrArchiveInvalid = errors.New("backup: invalid archive")

// ErrDestinationExists is returned when the destination state directory
// already contains a database and the caller did not request an explicit
// replacement.
var ErrDestinationExists = errors.New("backup: destination already contains a database; use force to replace it")

// prePublishHook is a test seam standing in for an interruption between
// the staged-snapshot validation and the atomic publish rename.
var prePublishHook func(staged string)

// RestoreOptions selects one restore of a validated archive.
type RestoreOptions struct {
	Archive  string
	StateDir string
	Force    bool
}

// RestoreResult is one completed restore.
type RestoreResult struct {
	StateDir string
	Schema   int
	DB       DBInfo
}

// Restore validates the archive completely — layout, manifest, size, hash,
// schema and integrity — before touching the destination, then replaces
// the destination database under the exclusive lifetime maintenance lock
// with one atomic same-filesystem rename. Normal writers (shared lock
// holders) block the replacement, so an active serve, MCP or CLI handle
// makes the restore fail with the locked error instead of racing. The
// destination state directory itself is never cleared: only the database
// file and its WAL sidecars are replaced, so an owner secret or any other
// state file is retained. A failure at any point leaves the previous
// database usable with a distinguishable error.
func Restore(ctx context.Context, opts RestoreOptions) (*RestoreResult, error) {
	if !filepath.IsAbs(opts.Archive) {
		return nil, fmt.Errorf("%w: archive %q must be an absolute path", ErrArchiveInvalid, opts.Archive)
	}
	if !filepath.IsAbs(opts.StateDir) {
		return nil, fmt.Errorf("%w: state dir %q must be an absolute path", ErrArchiveInvalid, opts.StateDir)
	}

	manifest, dbPath, err := validateArchive(opts.Archive)
	if err != nil {
		return nil, err
	}
	schema, err := validateSnapshot(ctx, dbPath, manifest.Schema)
	if err != nil {
		return nil, err
	}

	final := filepath.Join(opts.StateDir, store.DBFileName)
	if _, err := os.Lstat(final); err == nil {
		if !opts.Force {
			return nil, fmt.Errorf("%w: %s", ErrDestinationExists, final)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("backup: stat destination: %w", err)
	}

	if err := os.MkdirAll(opts.StateDir, 0o700); err != nil {
		return nil, fmt.Errorf("backup: create state dir: %w", err)
	}
	lock, err := store.AcquireMaintenanceLock(opts.StateDir, store.LockExclusive)
	if err != nil {
		return nil, err
	}
	defer lock.Release()

	if _, err := os.Lstat(final); err == nil {
		if !opts.Force {
			return nil, fmt.Errorf("%w: %s", ErrDestinationExists, final)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("backup: stat destination: %w", err)
	}

	// Stage the validated snapshot inside the destination state directory
	// so the publish rename stays on one filesystem.
	src, err := os.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("backup: open archive database: %w", err)
	}
	staged, err := os.CreateTemp(opts.StateDir, ".restore-")
	if err != nil {
		src.Close()
		return nil, fmt.Errorf("backup: stage snapshot: %w", err)
	}
	stagedName := staged.Name()
	closed := false
	defer func() {
		if !closed {
			staged.Close()
		}
		_ = os.Remove(stagedName)
	}()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(staged, h), src); err != nil {
		return nil, fmt.Errorf("backup: stage snapshot: %w", err)
	}
	src.Close()
	if err := staged.Close(); err != nil {
		return nil, fmt.Errorf("backup: close staged snapshot: %w", err)
	}
	closed = true
	if hex.EncodeToString(h.Sum(nil)) != manifest.DB.SHA256 {
		return nil, fmt.Errorf("%w: staged snapshot hash mismatch", ErrArchiveInvalid)
	}
	if err := os.Chmod(stagedName, 0o600); err != nil {
		return nil, fmt.Errorf("backup: chmod staged snapshot: %w", err)
	}

	// Drop the old database's WAL sidecars; the exclusive lock guarantees
	// no writer holds them, and the staged snapshot is a single file.
	for _, sidecar := range []string{final + "-wal", final + "-shm"} {
		if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("backup: remove %s: %w", filepath.Base(sidecar), err)
		}
	}

	if prePublishHook != nil {
		prePublishHook(stagedName)
	}
	if err := os.Rename(stagedName, final); err != nil {
		return nil, fmt.Errorf("backup: publish restored database: %w", err)
	}
	return &RestoreResult{
		StateDir: opts.StateDir,
		Schema:   schema,
		DB:       manifest.DB,
	}, nil
}

// validateArchive proves the entry is a well-formed v1 archive: exactly
// the two regular files, a parseable manifest with the format marker, and
// a database file whose size and hash match the manifest.
func validateArchive(archive string) (Manifest, string, error) {
	info, err := os.Lstat(archive)
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, "", fmt.Errorf("%w: %s not found", ErrArchiveInvalid, archive)
		}
		return Manifest{}, "", fmt.Errorf("%w: stat archive: %v", ErrArchiveInvalid, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Manifest{}, "", fmt.Errorf("%w: archive %s is a symlink", ErrArchiveInvalid, archive)
	}
	if !info.IsDir() {
		return Manifest{}, "", fmt.Errorf("%w: archive %s is not a directory", ErrArchiveInvalid, archive)
	}
	entries, err := os.ReadDir(archive)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("%w: read archive: %v", ErrArchiveInvalid, err)
	}
	if len(entries) != 2 {
		return Manifest{}, "", fmt.Errorf("%w: archive %s holds %d entries, want exactly %s and %s",
			ErrArchiveInvalid, archive, len(entries), DBFileName, ManifestFileName)
	}
	var dbPath, manifestPath string
	for _, e := range entries {
		full := filepath.Join(archive, e.Name())
		finfo, err := os.Lstat(full)
		if err != nil || finfo.Mode()&os.ModeSymlink != 0 || !finfo.Mode().IsRegular() {
			return Manifest{}, "", fmt.Errorf("%w: archive entry %s is not a regular file", ErrArchiveInvalid, e.Name())
		}
		switch e.Name() {
		case DBFileName:
			dbPath = full
		case ManifestFileName:
			manifestPath = full
		default:
			return Manifest{}, "", fmt.Errorf("%w: unexpected archive entry %s", ErrArchiveInvalid, e.Name())
		}
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("%w: read manifest: %v", ErrArchiveInvalid, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, "", fmt.Errorf("%w: manifest is not valid JSON", ErrArchiveInvalid)
	}
	if manifest.Format != ArchiveFormatName {
		return Manifest{}, "", fmt.Errorf("%w: manifest format %q, want %q", ErrArchiveInvalid, manifest.Format, ArchiveFormatName)
	}
	if manifest.DB.Name != DBFileName {
		return Manifest{}, "", fmt.Errorf("%w: manifest db name %q, want %q", ErrArchiveInvalid, manifest.DB.Name, DBFileName)
	}
	if manifest.Schema <= 0 {
		return Manifest{}, "", fmt.Errorf("%w: manifest schema %d, want positive", ErrArchiveInvalid, manifest.Schema)
	}
	dbInfo, err := os.Stat(dbPath)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("%w: stat database: %v", ErrArchiveInvalid, err)
	}
	if dbInfo.Size() != manifest.DB.Size {
		return Manifest{}, "", fmt.Errorf("%w: database size %d, manifest says %d", ErrArchiveInvalid, dbInfo.Size(), manifest.DB.Size)
	}
	f, err := os.Open(dbPath)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("%w: open database: %v", ErrArchiveInvalid, err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, f)
	closeErr := f.Close()
	if copyErr != nil {
		return Manifest{}, "", fmt.Errorf("%w: hash database: %v", ErrArchiveInvalid, copyErr)
	}
	if closeErr != nil {
		return Manifest{}, "", fmt.Errorf("%w: close database: %v", ErrArchiveInvalid, closeErr)
	}
	if hex.EncodeToString(hash.Sum(nil)) != manifest.DB.SHA256 {
		return Manifest{}, "", fmt.Errorf("%w: database hash does not match the manifest", ErrArchiveInvalid)
	}
	return manifest, dbPath, nil
}

// validateSnapshot opens the archived database read-only and proves it is
// a complete, valid database at the manifest's schema, which must also sit
// exactly at the restoring build's current schema.
func validateSnapshot(ctx context.Context, dbPath string, wantSchema int) (int, error) {
	db, err := sql.Open("sqlite", roDSN(dbPath))
	if err != nil {
		return 0, fmt.Errorf("%w: open database: %v", ErrArchiveInvalid, err)
	}
	defer db.Close()
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return 0, fmt.Errorf("%w: integrity check: %v", ErrArchiveInvalid, err)
	}
	if result != "ok" {
		return 0, fmt.Errorf("%w: integrity check failed: %s", ErrArchiveInvalid, result)
	}
	var schema int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&schema); err != nil {
		return 0, fmt.Errorf("%w: read schema ledger: %v", ErrArchiveInvalid, err)
	}
	if schema != wantSchema {
		return 0, fmt.Errorf("%w: ledger schema %d, manifest says %d", ErrArchiveInvalid, schema, wantSchema)
	}
	if err := store.CheckSchema(db); err != nil {
		return 0, err
	}
	return schema, nil
}

// SyntheticArchive builds a minimal valid v1 archive whose schema ledger
// holds the given version; tests use it for the off-build schema cases a
// real backup cannot produce.
func SyntheticArchive(dir string, schema int, createdAt time.Time) (string, error) {
	dbPath := filepath.Join(dir, DBFileName)
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(delete)")
	if err != nil {
		return "", err
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		db.Close()
		return "", err
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, 'synthetic.sql', '2026-11-12T00:00:00Z')`, schema); err != nil {
		db.Close()
		return "", err
	}
	if err := db.Close(); err != nil {
		return "", err
	}
	data, err := os.ReadFile(dbPath)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	manifest := Manifest{
		Format:    ArchiveFormatName,
		CreatedAt: createdAt,
		App:       App{Version: "synthetic", Commit: "synthetic", BuildTime: "synthetic", GoVersion: "synthetic"},
		Schema:    schema,
		DB:        DBInfo{Name: DBFileName, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])},
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFileName), append(raw, '\n'), 0o600); err != nil {
		return "", err
	}
	for _, sidecar := range []string{dbPath + "-wal", dbPath + "-shm"} {
		_ = os.Remove(sidecar)
	}
	return dbPath, nil
}
