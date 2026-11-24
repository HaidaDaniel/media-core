package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/artwork"
)

func TestBackupIncludesAndRestoresSelectedCoverBytes(t *testing.T) {
	ctx := context.Background()
	stateDir, db := initState(t)
	now := "2026-11-24T00:00:00Z"
	mustExec(t, db, `INSERT INTO works (id, revision, created_at, updated_at) VALUES ('w1', 0, ?, ?)`, now, now)
	data := []byte("selected cover bytes")
	ref := artwork.Ref("kitsu", "https://kitsu.io/manga/1")
	cacheDir := filepath.Join(stateDir, artwork.CacheDirName)
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, ref), data, 0o600); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `INSERT INTO work_artwork
		(id, work_id, provider, external_id, title, source_url, attribution_url, kind, local_ref, mime_type, byte_size, provenance, created_at, updated_at)
		VALUES ('a1', 'w1', 'kitsu', '1', 'Cover', 'https://kitsu.io/manga/1', 'https://kitsu.io/', 'manga', ?, 'image/jpeg', ?, 'artwork-search:kitsu', ?, ?)`,
		ref, len(data), now, now)

	dest := t.TempDir()
	result, err := Create(ctx, Options{StateDir: stateDir, DestDir: dest, Name: "with-art"})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if result.Manifest.Artwork == nil || len(result.Manifest.Artwork.Files) != 1 {
		t.Fatalf("manifest artwork = %+v", result.Manifest.Artwork)
	}
	entry := result.Manifest.Artwork.Files[0]
	if entry.Name != ref || entry.Size != int64(len(data)) || entry.SHA256 == "" {
		t.Fatalf("manifest artwork entry = %+v", entry)
	}
	if _, err := os.Stat(filepath.Join(result.Path, ArtworkDirName, ref)); err != nil {
		t.Fatalf("archived cover: %v", err)
	}

	restored := t.TempDir()
	res, err := Restore(ctx, RestoreOptions{Archive: result.Path, StateDir: restored})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if res.ArtworkFiles != 1 {
		t.Fatalf("restored artwork files = %d, want 1", res.ArtworkFiles)
	}
	got, err := os.ReadFile(filepath.Join(restored, artwork.CacheDirName, ref))
	if err != nil {
		t.Fatalf("restored cover bytes: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("restored cover bytes differ")
	}
	cache, err := artwork.NewCache(restored, artwork.DefaultMaxCacheBytes, artwork.DefaultMaxImageBytes)
	if err != nil {
		t.Fatal(err)
	}
	if removed, _, err := cache.Clear(); err != nil || removed != 1 {
		t.Fatalf("cache clear after restore = %d, %v", removed, err)
	}

	// Tampering with an archived cover must fail before the database publish.
	if err := os.WriteFile(filepath.Join(result.Path, ArtworkDirName, ref), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if _, err := Restore(ctx, RestoreOptions{Archive: result.Path, StateDir: other}); !errors.Is(err, ErrArchiveInvalid) {
		t.Fatalf("tampered restore err = %v, want ErrArchiveInvalid", err)
	}
	if _, err := os.Stat(filepath.Join(other, "media-core.db")); !os.IsNotExist(err) {
		t.Fatalf("tampered restore published a database: %v", err)
	}
}
