// Artwork backup regression: a user-selected cover's provenance lives in
// SQLite and its bytes live in the cache. Archive v2 carries both, so a
// restore brings the exact cover back without network access.
package acceptance

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/artwork"
	"github.com/HaidaDaniel/media-core/internal/backup"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func artworkState(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	maint, err := store.OpenForMaintenance(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunMigrations(maint.DB()); err != nil {
		t.Fatal(err)
	}
	if err := maint.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func artworkJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{R: 40, G: 44, B: 48, A: 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestArtworkSelectionSurvivesBackupWithBytes(t *testing.T) {
	ctx := context.Background()
	stateDir := artworkState(t)
	db, err := store.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	work, err := db.CreateWork(ctx, time.Now().UTC(), []store.TitleInput{{
		Text: "Cover Me", Kind: domain.TitleOriginal, Preferred: true, Provenance: "manual",
	}})
	if err != nil {
		t.Fatal(err)
	}
	cache, err := artwork.NewCache(stateDir, artwork.DefaultMaxCacheBytes, artwork.DefaultMaxImageBytes)
	if err != nil {
		t.Fatal(err)
	}
	data := artworkJPEG(t)
	ref := artwork.Ref("kitsu", "https://kitsu.io/manga/1")
	if _, err := cache.Put(ref, data, "image/jpeg", nil); err != nil {
		t.Fatal(err)
	}
	record, err := db.SaveArtwork(ctx, store.ArtworkInput{
		WorkID: work.ID, Provider: "kitsu", ExternalID: "1", Title: "Cover Me",
		SourceURL: "https://kitsu.io/manga/1", AttributionURL: "https://kitsu.io/",
		Kind: "manga", LocalRef: ref, MIMEType: "image/jpeg", ByteSize: int64(len(data)),
		Provenance: "artwork-search:kitsu",
	}, time.Now().UTC(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	result, err := backup.Create(ctx, backup.Options{StateDir: stateDir, DestDir: dest, Name: "artwork-test.zip"})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	entries, err := os.ReadDir(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("archive has %d entries, want database, manifest and artwork-cache", len(entries))
	}
	if result.Manifest.Artwork == nil || len(result.Manifest.Artwork.Files) != 1 {
		t.Fatalf("manifest artwork = %+v, want one selected file", result.Manifest.Artwork)
	}
	if result.Manifest.Artwork.Files[0].Name != ref {
		t.Fatalf("manifest artwork name = %q, want %q", result.Manifest.Artwork.Files[0].Name, ref)
	}

	restored := t.TempDir()
	if _, err := backup.Restore(ctx, backup.RestoreOptions{Archive: result.Path, StateDir: restored}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	restoredDB, err := store.Open(restored)
	if err != nil {
		t.Fatal(err)
	}
	defer restoredDB.Close()
	got, err := restoredDB.ArtworkByID(ctx, record.ID)
	if err != nil {
		t.Fatalf("restored artwork: %v", err)
	}
	if got.Provider != "kitsu" || got.ExternalID != "1" || !got.SelectedByUser || got.LocalRef != ref {
		t.Fatalf("restored artwork = %+v", got)
	}
	restoredBytes, err := os.ReadFile(filepath.Join(restored, "artwork-cache", ref))
	if err != nil {
		t.Fatalf("restored cover bytes: %v", err)
	}
	if !bytes.Equal(restoredBytes, data) {
		t.Fatalf("restored cover bytes differ from the selected image")
	}
}
