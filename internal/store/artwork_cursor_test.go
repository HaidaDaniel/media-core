package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

func TestMissingArtworkCursorDoesNotSkipAfterSelection(t *testing.T) {
	ctx := context.Background()
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	base := time.Now().UTC().Truncate(time.Second)
	ids := make([]string, 0, 7)
	for i := 0; i < 7; i++ {
		w, err := db.CreateWork(ctx, base.Add(time.Duration(i)*time.Second), []TitleInput{{
			Text: fmt.Sprintf("Work %d", i), Kind: domain.TitleOriginal, Preferred: true, Provenance: "test",
		}})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, w.ID)
	}
	page1, total, err := db.ListWorksMissingArtwork(ctx, nil, 3)
	if err != nil {
		t.Fatal(err)
	}
	if total != 7 || len(page1) != 3 {
		t.Fatalf("page1 total=%d len=%d", total, len(page1))
	}
	if _, err := db.SaveArtwork(ctx, ArtworkInput{
		WorkID: ids[0], Provider: "kitsu", ExternalID: "1", Title: "Cover",
		SourceURL: "https://kitsu.io/manga/1", AttributionURL: "https://kitsu.io/",
		Kind: "manga", LocalRef: "abc.img", MIMEType: "image/jpeg", ByteSize: 1,
		Provenance: "artwork-search:kitsu",
	}, base, false); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, w := range page1 {
		seen[w.ID] = true
	}
	last := page1[len(page1)-1]
	cursor := &MissingArtworkCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	page2, _, err := db.ListWorksMissingArtwork(ctx, cursor, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range page2 {
		seen[w.ID] = true
	}
	last = page2[len(page2)-1]
	cursor = &MissingArtworkCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	page3, _, err := db.ListWorksMissingArtwork(ctx, cursor, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range page3 {
		seen[w.ID] = true
	}
	for i, id := range ids {
		if i == 0 {
			continue
		}
		if !seen[id] {
			t.Fatalf("work index %d was skipped after a selection between pages", i)
		}
	}
}
