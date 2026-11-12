package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestBackupService exercises the service-level archive creation: the
// success payload mirrors the manifest, and each failure class maps to its
// stable v1 error code.
func TestBackupService(t *testing.T) {
	t.Run("success payload", func(t *testing.T) {
		s := newTestService(t)
		dest := filepath.Join(t.TempDir(), "backups")
		res, err := s.CreateBackup(context.Background(), BackupRequest{DestDir: dest, Name: "one"})
		if err != nil {
			t.Fatalf("CreateBackup: %v", err)
		}
		if res.Format != "media-core-snapshot-v1" {
			t.Fatalf("Format = %q", res.Format)
		}
		if !filepath.IsAbs(res.Archive) || filepath.Dir(res.Archive) != dest {
			t.Fatalf("Archive = %q, want under %q", res.Archive, dest)
		}
		if _, err := os.Stat(res.Archive); err != nil {
			t.Fatalf("archive %s missing: %v", res.Archive, err)
		}
		if res.Schema <= 0 {
			t.Fatalf("Schema = %d, want positive", res.Schema)
		}
		if res.DB.Name != "media-core.db" || res.DB.Size <= 0 {
			t.Fatalf("DB = %+v, want a measured database file", res.DB)
		}
		if ok, _ := regexp.MatchString(`^[0-9a-f]{64}$`, res.DB.SHA256); !ok {
			t.Fatalf("SHA256 = %q, want 64 hex chars", res.DB.SHA256)
		}
		if res.App.Version == "" || res.App.GoVersion == "" {
			t.Fatalf("App = %+v, want identity fields", res.App)
		}
	})
	t.Run("existing target is a conflict", func(t *testing.T) {
		s := newTestService(t)
		dest := filepath.Join(t.TempDir(), "backups")
		if _, err := s.CreateBackup(context.Background(), BackupRequest{DestDir: dest, Name: "dup"}); err != nil {
			t.Fatalf("first CreateBackup: %v", err)
		}
		_, err := s.CreateBackup(context.Background(), BackupRequest{DestDir: dest, Name: "dup"})
		if err == nil {
			t.Fatalf("second CreateBackup succeeded, want conflict")
		}
		var ae *AppError
		if !errors.As(err, &ae) || ae.Code != ErrConflict {
			t.Fatalf("err = %v, want code %q", err, ErrConflict)
		}
	})
	t.Run("overwrite replaces", func(t *testing.T) {
		s := newTestService(t)
		dest := filepath.Join(t.TempDir(), "backups")
		first, err := s.CreateBackup(context.Background(), BackupRequest{DestDir: dest, Name: "ow"})
		if err != nil {
			t.Fatalf("first CreateBackup: %v", err)
		}
		second, err := s.CreateBackup(context.Background(), BackupRequest{DestDir: dest, Name: "ow", Overwrite: true})
		if err != nil {
			t.Fatalf("overwriting CreateBackup: %v", err)
		}
		if first.Archive != second.Archive {
			t.Fatalf("Archive = %q, want %q", second.Archive, first.Archive)
		}
	})
	t.Run("invalid options are invalid_argument", func(t *testing.T) {
		s := newTestService(t)
		cases := []BackupRequest{
			{DestDir: "relative"},
			{DestDir: t.TempDir(), Name: "a/b"},
		}
		for _, req := range cases {
			_, err := s.CreateBackup(context.Background(), req)
			var ae *AppError
			if !errors.As(err, &ae) || ae.Code != ErrInvalidArgument {
				t.Fatalf("CreateBackup(%+v) = %v, want code %q", req, err, ErrInvalidArgument)
			}
		}
	})
}
