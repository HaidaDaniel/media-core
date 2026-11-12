package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/backup"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// openStateService opens an initialized state directory as a service.
func openStateService(t *testing.T, dir string) *Service {
	t.Helper()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewService(db)
}

// freshState initializes one state directory and returns its path.
func freshState(t *testing.T) string {
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
	return dir
}

// TestRestoreService exercises the service-level restore: the success
// payload and each failure class mapped to its stable v1 error code.
func TestRestoreService(t *testing.T) {
	t.Run("success payload", func(t *testing.T) {
		src := freshState(t)
		archive, err := backup.Create(context.Background(), backup.Options{StateDir: src, DestDir: t.TempDir(), Name: "one"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		dest := filepath.Join(t.TempDir(), "state")
		res, err := Restore(context.Background(), RestoreRequest{Archive: archive.Path, StateDir: dest})
		if err != nil {
			t.Fatalf("Restore: %v", err)
		}
		if res.StateDir != dest || res.Schema <= 0 {
			t.Fatalf("result = %+v, want state dir and positive schema", res)
		}
		if res.DB.Name != "media-core.db" || res.DB.Size <= 0 {
			t.Fatalf("DB = %+v, want a measured database file", res.DB)
		}
	})
	t.Run("existing destination is a conflict", func(t *testing.T) {
		src := freshState(t)
		archive, err := backup.Create(context.Background(), backup.Options{StateDir: src, DestDir: t.TempDir(), Name: "dup"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		dest := freshState(t)
		_, err = Restore(context.Background(), RestoreRequest{Archive: archive.Path, StateDir: dest})
		var ae *AppError
		if !errors.As(err, &ae) || ae.Code != ErrConflict {
			t.Fatalf("Restore err = %v, want code %q", err, ErrConflict)
		}
	})
	t.Run("invalid archive is invalid_argument", func(t *testing.T) {
		cases := []RestoreRequest{
			{Archive: filepath.Join(t.TempDir(), "missing"), StateDir: t.TempDir()},
			{Archive: "relative", StateDir: t.TempDir()},
			{Archive: t.TempDir(), StateDir: "relative"},
		}
		for _, req := range cases {
			_, err := Restore(context.Background(), req)
			var ae *AppError
			if !errors.As(err, &ae) || ae.Code != ErrInvalidArgument {
				t.Fatalf("Restore(%+v) = %v, want code %q", req, err, ErrInvalidArgument)
			}
		}
	})
	t.Run("journaled request replays", func(t *testing.T) {
		src := freshState(t)
		svc := openStateService(t, src)
		req := baseReq(t, "replay-req-1", "Replay Fixture")
		created, err := svc.CreateLibraryEntry(context.Background(), req)
		if err != nil {
			t.Fatalf("CreateLibraryEntry: %v", err)
		}
		if !created.Created || created.WorkID == "" {
			t.Fatalf("created = %+v, want a created work", created)
		}
		archive, err := backup.Create(context.Background(), backup.Options{StateDir: src, DestDir: t.TempDir(), Name: "journal"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		dest := filepath.Join(t.TempDir(), "state")
		if _, err := Restore(context.Background(), RestoreRequest{Archive: archive.Path, StateDir: dest}); err != nil {
			t.Fatalf("Restore: %v", err)
		}
		restored := openStateService(t, dest)
		replayed, err := restored.CreateLibraryEntry(context.Background(), req)
		if err != nil {
			t.Fatalf("replayed CreateLibraryEntry: %v", err)
		}
		if replayed.WorkID != created.WorkID || replayed.WorkRevision != created.WorkRevision {
			t.Fatalf("replayed = %+v, want the stored result", replayed)
		}
		var works int
		if err := restored.db.DB().QueryRow(`SELECT COUNT(*) FROM works`).Scan(&works); err != nil {
			t.Fatalf("count works: %v", err)
		}
		if works != 1 {
			t.Fatalf("work count after replay = %d, want 1 (no re-creation)", works)
		}
	})
	t.Run("off-build schema is schema_mismatch", func(t *testing.T) {
		d := freshState(t)
		handle, err := store.Open(d)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer handle.Close()
		var current int
		if err := handle.DB().QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
			t.Fatalf("read schema: %v", err)
		}
		archiveDir := t.TempDir()
		if _, err := backup.SyntheticArchive(archiveDir, current+5000, time.Now().UTC()); err != nil {
			t.Fatalf("SyntheticArchive: %v", err)
		}
		_, err = Restore(context.Background(), RestoreRequest{Archive: archiveDir, StateDir: t.TempDir()})
		var ae *AppError
		if !errors.As(err, &ae) || ae.Code != ErrSchemaMismatch {
			t.Fatalf("Restore err = %v, want code %q", err, ErrSchemaMismatch)
		}
	})
}
