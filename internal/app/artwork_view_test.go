package app

import (
	"context"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func TestEditionViewKeepsSavedArtwork(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Now().UTC()
	work, err := svc.db.CreateWork(ctx, now, []store.TitleInput{{
		Text: "Cover persistence", Kind: domain.TitleOriginal, Preferred: true, Provenance: "test",
	}})
	if err != nil {
		t.Fatal(err)
	}
	edition, err := svc.db.AddEdition(ctx, now, work.ID, work.Revision, store.EditionInput{
		Kind: domain.KindAnime,
		Titles: []store.TitleInput{{
			Text: "Cover persistence", Kind: domain.TitleOriginal, Preferred: true, Provenance: "test",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := edition.ID
	if _, err := svc.db.SaveArtwork(ctx, store.ArtworkInput{
		WorkID: work.ID, EditionID: &id, Provider: "kitsu", ExternalID: "1",
		Title: "Cover persistence", SourceURL: "https://kitsu.io/anime/1",
		AttributionURL: "https://kitsu.io/", Kind: "anime", LocalRef: "ref.img",
		MIMEType: "image/jpeg", ByteSize: 1, Provenance: "artwork-search:kitsu",
	}, now, false); err != nil {
		t.Fatal(err)
	}
	view, err := svc.editionViewWithArtwork(ctx, edition)
	if err != nil {
		t.Fatal(err)
	}
	if view.Artwork == nil || view.Artwork.Provider != "kitsu" ||
		view.Artwork.ImageURL != "/api/v1/artwork/"+view.Artwork.ID+"/image" {
		t.Fatalf("edition view artwork = %+v", view.Artwork)
	}
}
