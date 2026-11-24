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
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/HaidaDaniel/media-core/internal/artwork"
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

// stagedSyncHook is a test seam for a failure while flushing the staged
// snapshot to stable storage, before any destination change.
var stagedSyncHook func() error

// RestoreOptions selects one restore of a validated archive.
type RestoreOptions struct {
	Archive  string
	StateDir string
	Force    bool
}

// RestoreResult is one completed restore.
type RestoreResult struct {
	StateDir     string
	Schema       int
	DB           DBInfo
	ArtworkFiles int
}

// Restore validates the archive completely — layout, manifest, every file
// hash, schema and integrity — before touching the destination, then installs
// the validated artwork bytes and replaces the destination database under the
// exclusive lifetime maintenance lock with one atomic same-filesystem rename.
// Normal writers (shared lock holders) block the replacement, so an active
// serve, MCP or CLI handle makes the restore fail with the locked error
// instead of racing. The destination state directory itself is never cleared:
// only the database file, its WAL sidecars and referenced artwork files are
// written, so an owner secret or any other state file is retained. A failure before publication leaves the previous
// database usable with a distinguishable error; a directory-sync failure is
// reported after the new snapshot is already in place.
func Restore(ctx context.Context, opts RestoreOptions) (*RestoreResult, error) {
	if !filepath.IsAbs(opts.Archive) {
		return nil, fmt.Errorf("%w: archive %q must be an absolute path", ErrArchiveInvalid, opts.Archive)
	}
	if !filepath.IsAbs(opts.StateDir) {
		return nil, fmt.Errorf("%w: state dir %q must be an absolute path", ErrArchiveInvalid, opts.StateDir)
	}

	validated, err := validateArchive(opts.Archive)
	if err != nil {
		return nil, err
	}
	manifest, dbPath := validated.manifest, validated.dbPath
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

	// Establish path trust before creating anything: MkdirAll must not be
	// able to follow a symlinked parent component into another directory.
	if err := store.VerifyPathChain(opts.StateDir); err != nil {
		return nil, err
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
	if stagedSyncHook != nil {
		if err := stagedSyncHook(); err != nil {
			return nil, fmt.Errorf("backup: sync staged snapshot: %w", err)
		}
	}
	// Flush the file itself before publishing its name; syncing only the
	// directory after rename would expose unsynced data after a power cut.
	if err := staged.Sync(); err != nil {
		return nil, fmt.Errorf("backup: sync staged snapshot: %w", err)
	}
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

	// Checkpoint the destination before touching its sidecars. A process
	// killed after committing but before a checkpoint leaves acknowledged
	// data only in the WAL; deleting the sidecars first would lose it if
	// the restore is interrupted before publish. The exclusive maintenance
	// lock already excludes other writers, so this connection needs no
	// second maintenance-lock acquisition.
	if err := checkpointDestination(final); err != nil {
		return nil, err
	}

	// With every acknowledged transaction now in the main file, the old
	// sidecars can be dropped before the single-file snapshot is published.
	for _, sidecar := range []string{final + "-wal", final + "-shm"} {
		if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("backup: remove %s: %w", filepath.Base(sidecar), err)
		}
	}

	artworkFiles, err := installArtwork(opts.StateDir, validated.artwork)
	if err != nil {
		return nil, err
	}
	if prePublishHook != nil {
		prePublishHook(stagedName)
	}
	if err := os.Rename(stagedName, final); err != nil {
		return nil, fmt.Errorf("backup: publish restored database: %w", err)
	}
	if err := syncDirectory(opts.StateDir); err != nil {
		return nil, err
	}
	return &RestoreResult{
		StateDir:     opts.StateDir,
		Schema:       schema,
		DB:           manifest.DB,
		ArtworkFiles: artworkFiles,
	}, nil
}

// archiveArtworkFile is one validated cover file inside an archive.
type archiveArtworkFile struct {
	path  string
	entry ArchivedArtwork
}

type validatedArchive struct {
	manifest Manifest
	dbPath   string
	artwork  map[string]archiveArtworkFile
}

// validateArchive proves the entry is a well-formed v1 or v2 archive. v1
// holds exactly the database and the manifest. v2 also accepts an
// artwork-cache directory whose files must match the manifest exactly.
func validateArchive(archive string) (validatedArchive, error) {
	info, err := os.Lstat(archive)
	if err != nil {
		if os.IsNotExist(err) {
			return validatedArchive{}, fmt.Errorf("%w: %s not found", ErrArchiveInvalid, archive)
		}
		return validatedArchive{}, fmt.Errorf("%w: stat archive: %v", ErrArchiveInvalid, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return validatedArchive{}, fmt.Errorf("%w: archive %s is a symlink", ErrArchiveInvalid, archive)
	}
	if !info.IsDir() {
		return validatedArchive{}, fmt.Errorf("%w: archive %s is not a directory", ErrArchiveInvalid, archive)
	}
	entries, err := os.ReadDir(archive)
	if err != nil {
		return validatedArchive{}, fmt.Errorf("%w: read archive: %v", ErrArchiveInvalid, err)
	}
	var dbPath, manifestPath, artworkDir string
	for _, e := range entries {
		full := filepath.Join(archive, e.Name())
		finfo, err := os.Lstat(full)
		if err != nil {
			return validatedArchive{}, fmt.Errorf("%w: stat archive entry %s: %v", ErrArchiveInvalid, e.Name(), err)
		}
		if finfo.Mode()&os.ModeSymlink != 0 {
			return validatedArchive{}, fmt.Errorf("%w: archive entry %s is a symlink", ErrArchiveInvalid, e.Name())
		}
		switch e.Name() {
		case DBFileName:
			if !finfo.Mode().IsRegular() {
				return validatedArchive{}, fmt.Errorf("%w: archive entry %s is not a regular file", ErrArchiveInvalid, e.Name())
			}
			dbPath = full
		case ManifestFileName:
			if !finfo.Mode().IsRegular() {
				return validatedArchive{}, fmt.Errorf("%w: archive entry %s is not a regular file", ErrArchiveInvalid, e.Name())
			}
			manifestPath = full
		case ArtworkDirName:
			if !finfo.IsDir() {
				return validatedArchive{}, fmt.Errorf("%w: archive entry %s is not a directory", ErrArchiveInvalid, e.Name())
			}
			artworkDir = full
		default:
			return validatedArchive{}, fmt.Errorf("%w: unexpected archive entry %s", ErrArchiveInvalid, e.Name())
		}
	}
	if dbPath == "" || manifestPath == "" {
		return validatedArchive{}, fmt.Errorf("%w: archive %s is missing %s or %s", ErrArchiveInvalid, archive, DBFileName, ManifestFileName)
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return validatedArchive{}, fmt.Errorf("%w: read manifest: %v", ErrArchiveInvalid, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return validatedArchive{}, fmt.Errorf("%w: manifest is not valid JSON", ErrArchiveInvalid)
	}
	switch manifest.Format {
	case archiveFormatV1:
		if artworkDir != "" || manifest.Artwork != nil {
			return validatedArchive{}, fmt.Errorf("%w: v1 archive must not contain artwork", ErrArchiveInvalid)
		}
	case ArchiveFormatName:
		if manifest.Artwork == nil {
			return validatedArchive{}, fmt.Errorf("%w: v2 archive is missing the artwork manifest", ErrArchiveInvalid)
		}
	default:
		return validatedArchive{}, fmt.Errorf("%w: manifest format %q, want %q or %q", ErrArchiveInvalid, manifest.Format, archiveFormatV1, ArchiveFormatName)
	}
	if manifest.DB.Name != DBFileName {
		return validatedArchive{}, fmt.Errorf("%w: manifest db name %q, want %q", ErrArchiveInvalid, manifest.DB.Name, DBFileName)
	}
	if manifest.Schema <= 0 {
		return validatedArchive{}, fmt.Errorf("%w: manifest schema %d, want positive", ErrArchiveInvalid, manifest.Schema)
	}
	dbInfo, err := os.Stat(dbPath)
	if err != nil {
		return validatedArchive{}, fmt.Errorf("%w: stat database: %v", ErrArchiveInvalid, err)
	}
	if dbInfo.Size() != manifest.DB.Size {
		return validatedArchive{}, fmt.Errorf("%w: database size %d, manifest says %d", ErrArchiveInvalid, dbInfo.Size(), manifest.DB.Size)
	}
	f, err := os.Open(dbPath)
	if err != nil {
		return validatedArchive{}, fmt.Errorf("%w: open database: %v", ErrArchiveInvalid, err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, f)
	closeErr := f.Close()
	if copyErr != nil {
		return validatedArchive{}, fmt.Errorf("%w: hash database: %v", ErrArchiveInvalid, copyErr)
	}
	if closeErr != nil {
		return validatedArchive{}, fmt.Errorf("%w: close database: %v", ErrArchiveInvalid, closeErr)
	}
	if hex.EncodeToString(hash.Sum(nil)) != manifest.DB.SHA256 {
		return validatedArchive{}, fmt.Errorf("%w: database hash does not match the manifest", ErrArchiveInvalid)
	}
	files := map[string]archiveArtworkFile{}
	if manifest.Format == ArchiveFormatName {
		files, err = validateArtwork(artworkDir, manifest.Artwork)
		if err != nil {
			return validatedArchive{}, err
		}
	}
	return validatedArchive{manifest: manifest, dbPath: dbPath, artwork: files}, nil
}

// validateArtwork checks the v2 artwork directory against the manifest,
// including every file hash, and rejects extras and symlinks.
func validateArtwork(dir string, manifest *ArtworkManifest) (map[string]archiveArtworkFile, error) {
	expected := map[string]ArchivedArtwork{}
	for _, entry := range manifest.Files {
		if !artwork.ValidCacheRef(entry.Name) {
			return nil, fmt.Errorf("%w: invalid artwork name %q", ErrArchiveInvalid, entry.Name)
		}
		if _, exists := expected[entry.Name]; exists {
			return nil, fmt.Errorf("%w: duplicate artwork entry %q", ErrArchiveInvalid, entry.Name)
		}
		expected[entry.Name] = entry
	}
	for _, name := range manifest.Missing {
		if !artwork.ValidCacheRef(name) {
			return nil, fmt.Errorf("%w: invalid missing artwork name %q", ErrArchiveInvalid, name)
		}
		if _, exists := expected[name]; exists {
			return nil, fmt.Errorf("%w: artwork %q is both present and missing", ErrArchiveInvalid, name)
		}
	}
	if len(expected) == 0 {
		if dir != "" {
			return nil, fmt.Errorf("%w: artwork directory is present but the manifest lists no files", ErrArchiveInvalid)
		}
		return map[string]archiveArtworkFile{}, nil
	}
	if dir == "" {
		return nil, fmt.Errorf("%w: artwork files are missing from the archive", ErrArchiveInvalid)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("%w: read artwork directory: %v", ErrArchiveInvalid, err)
	}
	if len(entries) != len(expected) {
		return nil, fmt.Errorf("%w: artwork directory holds %d files, manifest lists %d", ErrArchiveInvalid, len(entries), len(expected))
	}
	files := make(map[string]archiveArtworkFile, len(expected))
	for _, e := range entries {
		entry, ok := expected[e.Name()]
		if !ok {
			return nil, fmt.Errorf("%w: unexpected artwork entry %q", ErrArchiveInvalid, e.Name())
		}
		full := filepath.Join(dir, e.Name())
		finfo, err := os.Lstat(full)
		if err != nil {
			return nil, fmt.Errorf("%w: stat artwork %s: %v", ErrArchiveInvalid, e.Name(), err)
		}
		if finfo.Mode()&os.ModeSymlink != 0 || !finfo.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: artwork %s is not a regular file", ErrArchiveInvalid, e.Name())
		}
		if finfo.Size() != entry.Size {
			return nil, fmt.Errorf("%w: artwork %s size %d, manifest says %d", ErrArchiveInvalid, e.Name(), finfo.Size(), entry.Size)
		}
		sum, err := hashFile(full)
		if err != nil {
			return nil, fmt.Errorf("%w: hash artwork %s: %v", ErrArchiveInvalid, e.Name(), err)
		}
		if sum != entry.SHA256 {
			return nil, fmt.Errorf("%w: artwork %s hash does not match the manifest", ErrArchiveInvalid, e.Name())
		}
		files[e.Name()] = archiveArtworkFile{path: full, entry: entry}
	}
	return files, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// installArtwork writes the validated cover files into the destination cache
// before the database publish. Extra cache files are harmless if publication
// later fails; a missing or altered archive file aborts before any database
// change.
func installArtwork(stateDir string, files map[string]archiveArtworkFile) (int, error) {
	if len(files) == 0 {
		return 0, nil
	}
	dir := filepath.Join(stateDir, ArtworkDirName)
	if info, err := os.Lstat(dir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return 0, fmt.Errorf("%w: artwork cache path is not a directory", ErrArchiveInvalid)
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return 0, fmt.Errorf("backup: create artwork cache: %w", err)
		}
	} else {
		return 0, fmt.Errorf("backup: stat artwork cache: %w", err)
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	installed := 0
	for _, name := range names {
		entry := files[name]
		tmp, err := os.CreateTemp(dir, ".restore-artwork-*")
		if err != nil {
			return installed, fmt.Errorf("backup: stage artwork: %w", err)
		}
		tmpName := tmp.Name()
		removeTemp := true
		defer func() {
			if removeTemp {
				_ = os.Remove(tmpName)
			}
		}()
		if err := tmp.Chmod(0o600); err != nil {
			tmp.Close()
			return installed, err
		}
		src, openErr := os.Open(entry.path)
		if openErr != nil {
			tmp.Close()
			return installed, fmt.Errorf("backup: open artwork %s: %w", name, openErr)
		}
		h := sha256.New()
		size, copyErr := io.Copy(io.MultiWriter(tmp, h), src)
		srcCloseErr := src.Close()
		if copyErr == nil {
			copyErr = srcCloseErr
		}
		if copyErr == nil {
			copyErr = tmp.Sync()
		}
		closeErr := tmp.Close()
		if copyErr != nil {
			return installed, fmt.Errorf("backup: copy artwork %s: %w", name, copyErr)
		}
		if closeErr != nil {
			return installed, fmt.Errorf("backup: close artwork %s: %w", name, closeErr)
		}
		if size != entry.entry.Size || hex.EncodeToString(h.Sum(nil)) != entry.entry.SHA256 {
			return installed, fmt.Errorf("%w: artwork %s changed after validation", ErrArchiveInvalid, name)
		}
		if err := os.Rename(tmpName, filepath.Join(dir, name)); err != nil {
			return installed, fmt.Errorf("backup: publish artwork %s: %w", name, err)
		}
		removeTemp = false
		installed++
	}
	return installed, nil
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
		Format:    archiveFormatV1,
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

// checkpointDestination folds every acknowledged WAL frame into the main
// database before the sidecars are removed. It runs while the caller holds
// the exclusive maintenance lock, so no other process can hold a writer.
func checkpointDestination(dbPath string) error {
	info, err := os.Lstat(dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("backup: stat destination database: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s", store.ErrSymlink, dbPath)
	}
	u := &url.URL{Scheme: "file", Path: dbPath}
	db, err := sql.Open("sqlite", u.String()+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("backup: open destination for checkpoint: %w", err)
	}
	var busy, logged, checkpointed int
	err = db.QueryRow(`PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logged, &checkpointed)
	closeErr := db.Close()
	if err != nil {
		return fmt.Errorf("backup: checkpoint destination: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("backup: close destination checkpoint: %w", closeErr)
	}
	if busy != 0 {
		return fmt.Errorf("backup: checkpoint destination: database is busy")
	}
	return nil
}

// syncDirectory makes the publication rename durable before Restore returns.
func syncDirectory(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("backup: open state dir for sync: %w", err)
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return fmt.Errorf("backup: sync state dir: %w", err)
	}
	return nil
}
