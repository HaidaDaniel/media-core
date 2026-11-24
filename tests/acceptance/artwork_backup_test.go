// Artwork backup regression: a user-selected cover's provenance and cache
// reference live in SQLite and therefore travel in backups, while regenerable
// image bytes stay outside the two-file archive. A restore keeps the
// selection and leaves the cache to be rebuilt by an explicit new selection.
package acceptance

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
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

func TestArtworkSelectionSurvivesBackupAndCacheDoesNot(t *testing.T) {
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
	if len(entries) != 2 {
		t.Fatalf("archive has %d files, want exactly database and manifest", len(entries))
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "artwork-cache") {
			t.Fatalf("regenerable cache file %q must not be archived", entry.Name())
		}
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
	if _, err := os.Stat(filepath.Join(restored, "artwork-cache", ref)); !os.IsNotExist(err) {
		t.Fatalf("cache bytes must not travel with the archive; stat err = %v", err)
	}
}
