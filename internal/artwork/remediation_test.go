package artwork

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func TestCacheHardCapIncludesProtectedFiles(t *testing.T) {
	data := tinyJPEG(t)
	size := int64(len(data))
	cache, err := NewCache(t.TempDir(), size*2, size)
	if err != nil {
		t.Fatal(err)
	}
	protected := map[string]bool{}
	for i := 0; i < 2; i++ {
		ref := Ref("kitsu", fmt.Sprintf("https://kitsu.io/manga/%d", i))
		if _, err := cache.Put(ref, data, "image/jpeg", protected); err != nil {
			t.Fatalf("protected put %d: %v", i, err)
		}
		protected[ref] = true
	}
	third := Ref("kitsu", "https://kitsu.io/manga/third")
	if _, err := cache.Put(third, data, "image/jpeg", protected); !errors.Is(err, ErrCacheFull) {
		t.Fatalf("third protected put err = %v, want ErrCacheFull", err)
	}
	for ref := range protected {
		if _, _, err := cache.Open(ref); err != nil {
			t.Fatalf("protected selection %s was lost: %v", ref, err)
		}
	}
	count, bytes, err := cache.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || bytes != size*2 {
		t.Fatalf("cache stats = %d files/%d bytes, want 2/%d", count, bytes, size*2)
	}
}

func TestCacheConcurrentPutAndClear(t *testing.T) {
	data := tinyJPEG(t)
	cache, err := NewCache(t.TempDir(), int64(len(data))*64, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			ref := Ref("kitsu", fmt.Sprintf("https://kitsu.io/manga/%d", n))
			_, _ = cache.Put(ref, data, "image/jpeg", nil)
		}(i)
		go func() {
			defer wg.Done()
			_, _, _ = cache.Clear()
		}()
	}
	wg.Wait()
	if _, _, err := cache.Stats(); err != nil {
		t.Fatalf("stats after concurrent use: %v", err)
	}
}

func TestMatchUsesPairwiseUserAndCandidateTitles(t *testing.T) {
	unrelated := Match(SearchInput{Title: "Completely Unrelated", Kind: KindManga}, []Candidate{{
		Provider: "kitsu", ExternalID: "1", Title: "Different Work",
		AlternativeTitles: []string{"Different Work"}, Kind: KindManga, SourceURL: "https://x/1",
	}})
	if len(unrelated) != 0 {
		t.Fatalf("self-alternative inflated score: %+v", unrelated)
	}
	alt := Match(SearchInput{Title: "Foo", Kind: KindManga}, []Candidate{{
		Provider: "kitsu", ExternalID: "2", Title: "Bar",
		AlternativeTitles: []string{"Foo"}, Kind: KindManga, SourceURL: "https://x/2",
	}})
	if len(alt) != 1 || alt[0].Score < 0.9 {
		t.Fatalf("genuine alternative-title match = %+v", alt)
	}
}

func TestDefaultRegistryRegistersShikimoriManga(t *testing.T) {
	registry, _ := NewDefault()
	for _, info := range registry.Providers() {
		if info.ID != "shikimori" {
			continue
		}
		hasAnime, hasManga := false, false
		for _, kind := range info.Kinds {
			hasAnime = hasAnime || kind == string(KindAnime)
			hasManga = hasManga || kind == string(KindManga)
		}
		if !hasAnime || !hasManga {
			t.Fatalf("shikimori kinds = %v, want anime and manga", info.Kinds)
		}
		return
	}
	t.Fatal("shikimori provider is not registered")
}

func TestParseKindCoversEveryDomainFamily(t *testing.T) {
	canonical := map[string]Kind{
		"anime": KindAnime, "manga": KindManga, "manhwa": KindManga, "manhua": KindManga,
		"book": KindBook, "web_novel": KindBook, "light_novel": KindBook,
		"movie": KindMovie, "tv": KindTV,
	}
	for raw, want := range canonical {
		got, ok := ParseKind(raw)
		if !ok || got != want {
			t.Errorf("ParseKind(%q) = %q %v, want %q", raw, got, ok, want)
		}
	}
	for _, raw := range []string{"game", "visual_novel", "other", "unknown", ""} {
		if _, ok := ParseKind(raw); ok {
			t.Errorf("ParseKind(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestIsPublicIPRejectsEmbeddedAndSpecialRanges(t *testing.T) {
	rejected := []string{
		"100.64.0.1", "0.1.2.3", "192.0.2.1", "198.18.0.1", "198.51.100.1",
		"203.0.113.1", "240.0.0.1",
		"::127.0.0.1", "2002:7f00:1::", "fec0::1", "2001:db8::1", "2001:0:1::1",
		"64:ff9b::7f00:1", "fe80::1", "fd00::1", "ff02::1",
	}
	for _, raw := range rejected {
		if isPublicIP(net.ParseIP(raw)) {
			t.Errorf("isPublicIP(%s) = true, want false", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !isPublicIP(net.ParseIP(raw)) {
			t.Errorf("isPublicIP(%s) = false, want true", raw)
		}
	}
}

type slowProvider struct{}

func (slowProvider) ID() string             { return "slow" }
func (slowProvider) Label() string          { return "Slow" }
func (slowProvider) AttributionURL() string { return "https://example.com/" }
func (slowProvider) Kinds() []Kind          { return []Kind{KindBook} }
func (slowProvider) Search(ctx context.Context, _ string, _ Kind, _ int) ([]Candidate, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestFindMissingRejectsBadCursorAndHonoursDeadline(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	maint, err := store.OpenForMaintenance(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunMigrations(maint.DB()); err != nil {
		t.Fatal(err)
	}
	if err := maint.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	for i := 0; i < 2; i++ {
		work, err := db.CreateWork(ctx, now.Add(time.Duration(i)*time.Second), []store.TitleInput{{
			Text: fmt.Sprintf("Work %d", i), Kind: domain.TitleOriginal, Preferred: true, Provenance: "test",
		}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.AddEdition(ctx, now, work.ID, work.Revision, store.EditionInput{
			Kind: domain.KindBook,
			Titles: []store.TitleInput{{
				Text: fmt.Sprintf("Work %d", i), Kind: domain.TitleOriginal, Preferred: true, Provenance: "test",
			}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewService(db, NewRegistry(&slowProvider{}), nil, nil)
	if _, err := svc.FindMissing(ctx, "not-a-cursor", 5); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("bad cursor err = %v, want ErrInvalidInput", err)
	}
	deadline, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	page, err := svc.FindMissing(deadline, "", 5)
	if err != nil {
		t.Fatalf("deadline FindMissing: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("FindMissing took %s after the deadline", elapsed)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(page.Items))
	}
	for _, item := range page.Items {
		if item.Status != "timeout" {
			t.Fatalf("status = %q, want timeout", item.Status)
		}
	}
}
