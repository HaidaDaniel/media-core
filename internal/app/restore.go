package app

import (
	"context"
	"errors"

	"github.com/HaidaDaniel/media-core/internal/backup"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// RestoreRequest selects one restore of a validated archive into a state
// directory. Force permits replacing a destination that already contains
// a database; without it, an existing destination is a conflict.
type RestoreRequest struct {
	Archive  string
	StateDir string
	Force    bool
}

// RestoreResult is the success payload of one restore.
type RestoreResult struct {
	StateDir string       `json:"state_dir"`
	Schema   int          `json:"schema"`
	DB       BackupDBInfo `json:"db"`
}

// Restore validates the archive completely before touching the destination
// and then replaces the destination database under the exclusive
// maintenance lock. An active shared-lock holder — a running serve, MCP or
// CLI handle — blocks the replacement with the locked error.
func Restore(ctx context.Context, req RestoreRequest) (RestoreResult, error) {
	res, err := backup.Restore(ctx, backup.RestoreOptions{
		Archive:  req.Archive,
		StateDir: req.StateDir,
		Force:    req.Force,
	})
	if err != nil {
		return RestoreResult{}, newRestoreError(err)
	}
	return RestoreResult{
		StateDir: res.StateDir,
		Schema:   res.Schema,
		DB: BackupDBInfo{
			Name:   res.DB.Name,
			Size:   res.DB.Size,
			SHA256: res.DB.SHA256,
		},
	}, nil
}

// newRestoreError maps restore failures to stable v1 error codes: an
// existing destination without force is a conflict, an active handle is
// locked, an off-build schema is a schema mismatch, an invalid archive or
// path is an invalid argument and everything else is a storage failure.
func newRestoreError(err error) *AppError {
	switch {
	case errors.Is(err, backup.ErrDestinationExists):
		return NewAppError(ErrConflict, err.Error())
	case errors.Is(err, store.ErrLocked):
		return NewAppError(ErrLocked, err.Error())
	case errors.Is(err, store.ErrSchemaMissing),
		errors.Is(err, store.ErrSchemaPending),
		errors.Is(err, store.ErrSchemaNewer):
		return NewAppError(ErrSchemaMismatch, err.Error())
	case errors.Is(err, backup.ErrArchiveInvalid):
		return NewAppError(ErrInvalidArgument, err.Error())
	default:
		return NewAppError(ErrStorageError, err.Error())
	}
}
