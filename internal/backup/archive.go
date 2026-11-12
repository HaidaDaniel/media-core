// Package backup creates consistent SQLite snapshot archives with
// integrity manifests while normal writers may run.
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
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/HaidaDaniel/media-core/internal/buildinfo"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// ArchiveFormatName is the manifest format marker of the v1 snapshot
// archive.
const ArchiveFormatName = "media-core-snapshot-v1"

// DBFileName is the snapshot database file name inside the archive.
const DBFileName = "media-core.db"

// ManifestFileName is the manifest file name inside the archive.
const ManifestFileName = "manifest.json"

// ErrExists is returned when the target archive already exists and the
// caller did not request an overwrite.
var ErrExists = errors.New("backup: archive already exists; use force to replace it")

// ErrInvalid is returned when the options are not usable for a backup.
var ErrInvalid = errors.New("backup: invalid backup options")

// Options selects one snapshot archive creation.
type Options struct {
	StateDir  string
	DestDir   string
	Name      string
	Overwrite bool
}

// App is the build identity of the binary that created the archive.
type App struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

// DBInfo is the integrity metadata of the archived database file.
type DBInfo struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest is the archive integrity metadata. It describes the snapshot
// boundary only: every commit made before the VACUUM INTO finished is
// inside the archive, and the manifest never claims anything about commits
// made after that boundary.
type Manifest struct {
	Format    string    `json:"format"`
	CreatedAt time.Time `json:"created_at"`
	App       App       `json:"app"`
	Schema    int       `json:"schema"`
	DB        DBInfo    `json:"db"`
}

// Result is one published archive.
type Result struct {
	Path     string
	Manifest Manifest
}

// Create publishes one consistent snapshot archive of the state database
// into DestDir. It holds the shared maintenance lock, so normal writers may
// keep running; the snapshot contains every commit made before the VACUUM
// INTO finished, including WAL commits, and no claim is made about later
// commits. The archive holds only the snapshot database and the manifest;
// nothing else from the state directory is ever copied, so owner secrets,
// sessions and TLS keys are excluded by construction. Publishing is atomic:
// the archive appears as a complete directory or not at all, and on failure
// the source is left untouched and no staging remains.
func Create(ctx context.Context, opts Options) (*Result, error) {
	if !filepath.IsAbs(opts.StateDir) {
		return nil, fmt.Errorf("%w: state dir %q must be an absolute path", ErrInvalid, opts.StateDir)
	}
	if !filepath.IsAbs(opts.DestDir) {
		return nil, fmt.Errorf("%w: destination %q must be an absolute path", ErrInvalid, opts.DestDir)
	}
	name, err := archiveName(opts.Name)
	if err != nil {
		return nil, err
	}
	if insideStateDir(opts.StateDir, filepath.Join(opts.DestDir, name)) {
		return nil, fmt.Errorf("%w: the archive must not be published inside the state directory", ErrInvalid)
	}
	if err := os.MkdirAll(opts.DestDir, 0o700); err != nil {
		return nil, fmt.Errorf("backup: create destination: %w", err)
	}
	final := filepath.Join(opts.DestDir, name)
	if _, err := os.Lstat(final); err == nil {
		if !opts.Overwrite {
			return nil, fmt.Errorf("%w: %s", ErrExists, final)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("backup: stat target: %w", err)
	}

	lock, err := store.AcquireMaintenanceLock(opts.StateDir, store.LockShared)
	if err != nil {
		return nil, err
	}
	defer lock.Release()

	srcPath := filepath.Join(opts.StateDir, store.DBFileName)
	if err := verifySourceFile(srcPath); err != nil {
		return nil, err
	}

	staging, err := os.MkdirTemp(opts.DestDir, ".staging-")
	if err != nil {
		return nil, fmt.Errorf("backup: create staging: %w", err)
	}
	published := false
	setAside := ""
	defer func() {
		if !published {
			if setAside != "" {
				_ = os.Rename(setAside, final)
			}
			_ = os.RemoveAll(staging)
		}
	}()

	db, err := sql.Open("sqlite", roDSN(srcPath))
	if err != nil {
		return nil, fmt.Errorf("backup: open source: %w", err)
	}
	defer db.Close()

	if err := store.CheckSchema(db); err != nil {
		return nil, err
	}
	var schema int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&schema); err != nil {
		return nil, fmt.Errorf("backup: read schema ledger: %w", err)
	}

	snapPath := filepath.Join(staging, DBFileName)
	if _, err := db.ExecContext(ctx, "VACUUM INTO "+quoteLiteral(snapPath)); err != nil {
		return nil, fmt.Errorf("backup: snapshot: %w", err)
	}
	if err := os.Chmod(snapPath, 0o600); err != nil {
		return nil, fmt.Errorf("backup: chmod snapshot: %w", err)
	}
	if err := verifySnapshot(ctx, snapPath, schema); err != nil {
		return nil, err
	}

	manifest, err := buildManifest(snapPath, schema)
	if err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(staging, ManifestFileName)
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("backup: encode manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, append(data, '\n'), 0o600); err != nil {
		return nil, fmt.Errorf("backup: write manifest: %w", err)
	}

	if opts.Overwrite {
		if _, err := os.Lstat(final); err == nil {
			setAside, err = setAsideName(opts.DestDir, final)
			if err != nil {
				return nil, err
			}
			if err := os.Rename(final, setAside); err != nil {
				return nil, fmt.Errorf("backup: set aside replaced archive: %w", err)
			}
		}
	} else if _, err := os.Lstat(final); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrExists, final)
	}
	if err := os.Rename(staging, final); err != nil {
		if _, statErr := os.Lstat(final); statErr == nil {
			return nil, fmt.Errorf("%w: %s", ErrExists, final)
		}
		return nil, fmt.Errorf("backup: publish: %w", err)
	}
	published = true
	// The new archive is fully in place, so the set-aside copy of the
	// replaced archive can go; a crash before this point loses neither.
	if setAside != "" {
		if err := os.RemoveAll(setAside); err != nil {
			return nil, fmt.Errorf("backup: remove set-aside archive: %w", err)
		}
	}
	return &Result{Path: final, Manifest: manifest}, nil
}

// insideStateDir reports whether path is the state directory itself or
// below it; publishing there could let an overwrite reach a state file.
func insideStateDir(stateDir, path string) bool {
	stateDir = filepath.Clean(stateDir)
	path = filepath.Clean(path)
	if path == stateDir {
		return true
	}
	return strings.HasPrefix(path, stateDir+string(filepath.Separator))
}

// setAsideName chooses a free sibling name for the archive being replaced,
// so a crash between the set-aside and the publish loses neither archive.
func setAsideName(dest, final string) (string, error) {
	stamp := time.Now().UTC().Format("20060102T150405.000000")
	for i := 0; ; i++ {
		var name string
		if i == 0 {
			name = filepath.Base(final) + ".replaced-" + stamp
		} else {
			name = fmt.Sprintf("%s.replaced-%s-%d", filepath.Base(final), stamp, i)
		}
		cand := filepath.Join(dest, name)
		if _, err := os.Lstat(cand); err != nil {
			if os.IsNotExist(err) {
				return cand, nil
			}
			return "", fmt.Errorf("backup: stat set-aside name: %w", err)
		}
	}
}

// verifySourceFile rejects a missing, symlinked or non-regular source
// database before any backup work starts.
func verifySourceFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("backup: source database %s not found; run media-core init", path)
		}
		return fmt.Errorf("backup: stat source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: source database %s is a symlink", ErrInvalid, path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("backup: source database %s is not a regular file", path)
	}
	return nil
}

// verifySnapshot opens the fresh snapshot read-only and proves it is a
// complete, valid database at the source's schema version.
func verifySnapshot(ctx context.Context, path string, wantSchema int) error {
	db, err := sql.Open("sqlite", roDSN(path))
	if err != nil {
		return fmt.Errorf("backup: open snapshot: %w", err)
	}
	defer db.Close()
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("backup: snapshot integrity check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("backup: snapshot integrity check failed: %s", result)
	}
	var schema int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&schema); err != nil {
		return fmt.Errorf("backup: read snapshot schema ledger: %w", err)
	}
	if schema != wantSchema {
		return fmt.Errorf("backup: snapshot schema = %d, want %d", schema, wantSchema)
	}
	return nil
}

// buildManifest measures the snapshot file and records the snapshot
// boundary metadata.
func buildManifest(path string, schema int) (Manifest, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: stat snapshot: %w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("backup: hash snapshot: %w", err)
	}
	h := sha256.New()
	_, hashErr := io.Copy(h, f)
	closeErr := f.Close()
	if hashErr != nil {
		return Manifest{}, fmt.Errorf("backup: hash snapshot: %w", hashErr)
	}
	if closeErr != nil {
		return Manifest{}, fmt.Errorf("backup: close snapshot: %w", closeErr)
	}
	ident := buildinfo.Get()
	return Manifest{
		Format:    ArchiveFormatName,
		CreatedAt: time.Now().UTC(),
		App: App{
			Version:   ident.Version,
			Commit:    ident.Commit,
			BuildTime: ident.BuildTime,
			GoVersion: ident.GoVersion,
		},
		Schema: schema,
		DB: DBInfo{
			Name:   DBFileName,
			Size:   info.Size(),
			SHA256: hex.EncodeToString(h.Sum(nil)),
		},
	}, nil
}

// roDSN opens one read-only connection with a bounded busy timeout; reads
// see the WAL, so committed but not-yet-checkpointed data is visible.
func roDSN(path string) string {
	u := &url.URL{Scheme: "file", Path: path}
	return u.String() + "?mode=ro&_pragma=busy_timeout(5000)"
}

// archiveName validates the requested name and supplies the default.
func archiveName(name string) (string, error) {
	if name == "" {
		return "media-core-snapshot-" + time.Now().UTC().Format("20060102T150405Z"), nil
	}
	if name == "." || name == ".." || strings.ContainsAny(name, "/\\") || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("%w: archive name %q", ErrInvalid, name)
	}
	return name, nil
}

// quoteLiteral renders one SQL string literal for VACUUM INTO, which takes
// an expression rather than a bound parameter.
func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
