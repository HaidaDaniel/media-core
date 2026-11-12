package app

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/HaidaDaniel/media-core/internal/backup"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// BackupRequest selects one snapshot archive creation from a service.
// DestDir is where the archive directory is published; Name is optional
// and Overwrite permits replacing an existing archive.
type BackupRequest struct {
	DestDir   string
	Name      string
	Overwrite bool
}

// BackupApp is the build identity that created the archive.
type BackupApp struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

// BackupDBInfo is the integrity metadata of the archived database file.
type BackupDBInfo struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// BackupResult is the success payload of one snapshot archive creation.
// The manifest describes the snapshot boundary only: it never claims
// anything about commits made after the archive was taken.
type BackupResult struct {
	Archive   string       `json:"archive"`
	Format    string       `json:"format"`
	CreatedAt time.Time    `json:"created_at"`
	App       BackupApp    `json:"app"`
	Schema    int          `json:"schema"`
	DB        BackupDBInfo `json:"db"`
}

// CreateBackup publishes one consistent snapshot archive of the service's
// database. Normal writers may keep running; the snapshot contains every
// commit made before the snapshot finished and the archive excludes
// everything from the state directory except the database itself.
func (s *Service) CreateBackup(ctx context.Context, req BackupRequest) (BackupResult, error) {
	res, err := backup.Create(ctx, backup.Options{
		StateDir:  filepath.Dir(s.db.Path()),
		DestDir:   req.DestDir,
		Name:      req.Name,
		Overwrite: req.Overwrite,
	})
	if err != nil {
		return BackupResult{}, newBackupError(err)
	}
	return BackupResult{
		Archive:   res.Path,
		Format:    res.Manifest.Format,
		CreatedAt: res.Manifest.CreatedAt,
		App: BackupApp{
			Version:   res.Manifest.App.Version,
			Commit:    res.Manifest.App.Commit,
			BuildTime: res.Manifest.App.BuildTime,
			GoVersion: res.Manifest.App.GoVersion,
		},
		Schema: res.Manifest.Schema,
		DB: BackupDBInfo{
			Name:   res.Manifest.DB.Name,
			Size:   res.Manifest.DB.Size,
			SHA256: res.Manifest.DB.SHA256,
		},
	}, nil
}

// newBackupError maps backup failures to stable v1 error codes: an
// existing target is a conflict, a locked state is locked, a schema
// boundary failure is a schema mismatch, invalid options are invalid
// arguments and everything else is a storage failure.
func newBackupError(err error) *AppError {
	switch {
	case errors.Is(err, backup.ErrExists):
		return NewAppError(ErrConflict, err.Error())
	case errors.Is(err, store.ErrLocked):
		return NewAppError(ErrLocked, err.Error())
	case errors.Is(err, store.ErrSchemaMissing),
		errors.Is(err, store.ErrSchemaPending),
		errors.Is(err, store.ErrSchemaNewer):
		return NewAppError(ErrSchemaMismatch, err.Error())
	case errors.Is(err, backup.ErrInvalid):
		return NewAppError(ErrInvalidArgument, err.Error())
	default:
		return NewAppError(ErrStorageError, err.Error())
	}
}
