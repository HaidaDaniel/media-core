package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

func libraryNow() time.Time { return time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC) }

func mustWork(t *testing.T, db *Database, now time.Time, text string) Work {
	t.Helper()
	w, err := db.CreateWork(ctx(t), now, []TitleInput{{Text: text, Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork(%q): %v", text, err)
	}
	return w
}

func mustEdition(t *testing.T, db *Database, now time.Time, w Work, kind domain.EditionKind, titles ...string) (Edition, Work) {
	t.Helper()
	in := EditionInput{Kind: kind}
	for _, text := range titles {
		in.Titles = append(in.Titles, TitleInput{Text: text, Kind: domain.TitleOriginal, Provenance: "manual"})
	}
	e, err := db.AddEdition(ctx(t), now, w.ID, w.Revision, in)
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	fresh, err := db.GetWork(ctx(t), w.ID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	return e, fresh
}

func ctx(t *testing.T) context.Context { return context.Background() }

// TestLibrarySearchAliases proves JP, RU, English and romaji aliases find
// the same work and identify the matching edition.
func TestLibrarySearchAliases(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	ctx := ctx(t)

	w := mustWork(t, db, now, "フリーレン")
	w = mustWorkAddTitle(t, db, w, "Фрирен: Сильнейшая Магическая Воин")
	w = mustWorkAddTitle(t, db, w, "Frieren: Beyond Journey's End")
	manga, w := mustEdition(t, db, now, w, domain.KindManga, "Frieren Manga Edition")
	anime, w := mustEdition(t, db, now, w, domain.KindAnime, "Frieren Anime")

	for _, q := range []string{"フリーレン", "фрирен", "FRIEREN", "frieren"} {
		cards, total, err := db.SearchLibrary(ctx, LibraryQuery{Query: q, Sort: SortTitle, Limit: 10})
		if err != nil {
			t.Fatalf("SearchLibrary(%q): %v", q, err)
		}
		if total != 1 || len(cards) != 1 || cards[0].ID != w.ID {
			t.Fatalf("query %q: total=%d cards=%+v, want the one work", q, total, cards)
		}
		matched := cards[0].MatchedEditionIDs
		if len(matched) != 2 {
			t.Fatalf("query %q: matched = %v, want both editions (work title match)", q, matched)
		}
	}
	// An edition-specific alias identifies only that edition.
	cards, _, err := db.SearchLibrary(ctx, LibraryQuery{Query: "Manga Edition", Sort: SortTitle, Limit: 10})
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	if len(cards) != 1 || len(cards[0].MatchedEditionIDs) != 1 || cards[0].MatchedEditionIDs[0] != manga.ID {
		t.Fatalf("edition alias: %+v, want the manga edition only", cards)
	}
	if len(anime.Titles) == 0 {
		t.Fatal("the anime edition must keep its title")
	}
}

func mustWorkAddTitle(t *testing.T, db *Database, w Work, text string) Work {
	t.Helper()
	updated, err := db.AddWorkTitle(ctx(t), libraryNow(), w.ID, w.Revision, TitleInput{Text: text, Kind: domain.TitleLocalized, Provenance: "manual"})
	if err != nil {
		t.Fatalf("AddWorkTitle: %v", err)
	}
	return updated
}

// TestLibrarySearchWildcardsLiteral proves percent and underscore in the
// query match literally, never as LIKE wildcards.
func TestLibrarySearchWildcardsLiteral(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	ctx := ctx(t)
	percent := mustWork(t, db, now, "100% coverage_done")
	mustWork(t, db, now, "100 coverage done")
	// A percent sign in the query is literal, not a LIKE wildcard: only the
	// title that actually contains a percent matches.
	cards, total, err := db.SearchLibrary(ctx, LibraryQuery{Query: "100%", Sort: SortTitle, Limit: 10})
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	if total != 1 || len(cards) != 1 || cards[0].ID != percent.ID {
		t.Fatalf("percent query: total=%d cards=%+v, want the literal percent title only", total, cards)
	}
	// A bare underscore must not act as a one-character wildcard: the title
	// with a space in place of the underscore must not match.
	cards, total, err = db.SearchLibrary(ctx, LibraryQuery{Query: "coverage_done", Sort: SortTitle, Limit: 10})
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	if total != 1 || len(cards) != 1 || cards[0].ID != percent.ID {
		t.Fatalf("underscore query: total=%d cards=%+v, want the literal underscore title only", total, cards)
	}
}

// TestLibrarySearchCursor proves seek pagination: stable pages, the probe
// row, and that seeking past a row resumes exactly after it.
func TestLibrarySearchCursor(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	ctx := ctx(t)
	var ids []string
	for _, text := range []string{"Alpha", "Beta", "Gamma", "Delta"} {
		ids = append(ids, mustWork(t, db, now, text).ID)
	}
	// Page 1: two rows, a next cursor.
	cards, total, err := db.SearchLibrary(ctx, LibraryQuery{Sort: SortTitle, Limit: 2})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if total != 4 || len(cards) != 2 {
		t.Fatalf("page 1: total=%d len=%d, want 4/2", total, len(cards))
	}
	after := cards[1]
	// Page 2: resumes after the second row.
	page2, _, err := db.SearchLibrary(ctx, LibraryQuery{Sort: SortTitle, Limit: 2, AfterKey: after.SortKey, AfterID: after.ID})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page2) != 2 || page2[0].ID != ids[3] || page2[1].ID != ids[2] {
		t.Fatalf("page 2 = %v, want Delta then Gamma (title order)", page2)
	}
	// The final page has no further rows.
	last := page2[1]
	page3, _, err := db.SearchLibrary(ctx, LibraryQuery{Sort: SortTitle, Limit: 2, AfterKey: last.SortKey, AfterID: last.ID})
	if err != nil {
		t.Fatalf("page 3: %v", err)
	}
	if len(page3) != 0 {
		t.Fatalf("page 3 = %v, want empty", page3)
	}
	// An unknown seek ID resumes at the end.
	none, _, err := db.SearchLibrary(ctx, LibraryQuery{Sort: SortTitle, Limit: 2, AfterKey: "zzz", AfterID: "no-such-id"})
	if err != nil {
		t.Fatalf("unknown seek: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("unknown seek = %v, want empty", none)
	}
}

// TestLibrarySearchTotalsDistinct proves the total counts distinct works,
// not matching titles or editions.
func TestLibrarySearchTotalsDistinct(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Twin Peaks Story")
	w = mustWorkAddTitle(t, db, w, "Twin Peaks Story (alt)")
	w = mustWorkAddTitle(t, db, w, "Twin Peaks Story (romaji)")
	_, w = mustEdition(t, db, now, w, domain.KindManga, "Twin Peaks Manga")
	_, w = mustEdition(t, db, now, w, domain.KindAnime, "Twin Peaks Anime")
	_, total, err := db.SearchLibrary(ctx(t), LibraryQuery{Query: "Twin Peaks", Sort: SortTitle, Limit: 10})
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1 distinct work", total)
	}
}

// TestLibrarySearchKindFilter proves a kind filter qualifies the work
// through the matching edition only.
func TestLibrarySearchKindFilter(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	ctx := ctx(t)
	w := mustWork(t, db, now, "Medium Mix")
	manga, w := mustEdition(t, db, now, w, domain.KindManga)
	mustEdition(t, db, now, w, domain.KindAnime)
	other := mustWork(t, db, now, "Anime Only")
	mustEdition(t, db, now, other, domain.KindAnime)

	cards, total, err := db.SearchLibrary(ctx, LibraryQuery{Kinds: []domain.EditionKind{domain.KindManga}, Sort: SortTitle, Limit: 10})
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	if total != 1 || len(cards) != 1 || cards[0].ID != w.ID {
		t.Fatalf("kind filter: total=%d cards=%+v, want the mixed work", total, cards)
	}
	if len(cards[0].MatchedEditionIDs) != 1 || cards[0].MatchedEditionIDs[0] != manga.ID {
		t.Fatalf("kind filter matched = %v, want the manga edition only", cards[0].MatchedEditionIDs)
	}
}

// TestLibrarySearchPublicationFilter proves publication filters operate on
// one qualifying edition: a work with completed manga and ongoing anime
// matches ongoing through the anime edition only.
func TestLibrarySearchPublicationFilter(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	ctx := ctx(t)
	mixed := mustWork(t, db, now, "Mixed Status")
	manga, mixed := mustEdition(t, db, now, mixed, domain.KindManga)
	anime, mixed := mustEdition(t, db, now, mixed, domain.KindAnime)
	if _, err := db.RecordPublication(ctx, now, manga.ID, manga.Revision, PublicationInput{Value: domain.PublicationCompleted, Source: strPtr("manual"), Provenance: "manual"}); err != nil {
		t.Fatalf("RecordPublication: %v", err)
	}
	if _, err := db.RecordPublication(ctx, now, anime.ID, anime.Revision, PublicationInput{Value: domain.PublicationOngoing, Source: strPtr("manual"), Provenance: "manual"}); err != nil {
		t.Fatalf("RecordPublication: %v", err)
	}
	solo := mustWork(t, db, now, "Solo Status")
	soloAnime, _ := mustEdition(t, db, now, solo, domain.KindAnime)
	if _, err := db.RecordPublication(ctx, now, soloAnime.ID, soloAnime.Revision, PublicationInput{Value: domain.PublicationOngoing, Source: strPtr("manual"), Provenance: "manual"}); err != nil {
		t.Fatalf("RecordPublication: %v", err)
	}

	cards, total, err := db.SearchLibrary(ctx, LibraryQuery{Publications: []domain.Publication{domain.PublicationOngoing}, Sort: SortTitle, Limit: 10})
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	if total != 2 || len(cards) != 2 {
		t.Fatalf("publication filter: total=%d cards=%+v, want both works", total, cards)
	}
	byID := map[string]WorkCard{}
	for _, c := range cards {
		byID[c.ID] = c
	}
	mixedCard := byID[mixed.ID]
	if len(mixedCard.MatchedEditionIDs) != 1 || mixedCard.MatchedEditionIDs[0] != anime.ID {
		t.Fatalf("mixed work matched = %v, want the ongoing anime edition only", mixedCard.MatchedEditionIDs)
	}
	soloCard := byID[solo.ID]
	if len(soloCard.MatchedEditionIDs) != 1 || soloCard.MatchedEditionIDs[0] != soloAnime.ID {
		t.Fatalf("solo work matched = %v, want its anime edition", soloCard.MatchedEditionIDs)
	}
	// The combined kind + publication filter must not combine the manga
	// edition's kind with the anime edition's status.
	cards, total, err = db.SearchLibrary(ctx, LibraryQuery{
		Kinds:        []domain.EditionKind{domain.KindManga},
		Publications: []domain.Publication{domain.PublicationOngoing},
		Sort:         SortTitle,
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("combined filter: %v", err)
	}
	if total != 0 || len(cards) != 0 {
		t.Fatalf("combined filter: total=%d, want nothing (no manga edition is ongoing)", total)
	}
}

// TestLibrarySearchQueryFilterCorrelation proves the query and the
// edition-qualified filters are correlated: a work is listed only when one
// edition satisfies both, or the query hits a work title and some edition
// satisfies the filter.
func TestLibrarySearchQueryFilterCorrelation(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	ctx := ctx(t)
	w := mustWork(t, db, now, "Correlated Story")
	manga, w := mustEdition(t, db, now, w, domain.KindManga, "MangaOnly Title")
	anime, w := mustEdition(t, db, now, w, domain.KindAnime, "AnimeOnly Title")
	if _, err := db.RecordPublication(ctx, now, manga.ID, manga.Revision, PublicationInput{Value: domain.PublicationCompleted, Source: strPtr("manual"), Provenance: "manual"}); err != nil {
		t.Fatalf("RecordPublication manga: %v", err)
	}
	if _, err := db.RecordPublication(ctx, now, anime.ID, anime.Revision, PublicationInput{Value: domain.PublicationOngoing, Source: strPtr("manual"), Provenance: "manual"}); err != nil {
		t.Fatalf("RecordPublication anime: %v", err)
	}

	// The query matches only the anime title; the kind filter is satisfied
	// only by the manga edition. No single edition satisfies both, so the
	// work must not be listed.
	cards, total, err := db.SearchLibrary(ctx, LibraryQuery{Query: "animeonly", Kinds: []domain.EditionKind{domain.KindManga}, Sort: SortTitle, Limit: 10})
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	if total != 0 || len(cards) != 0 {
		t.Fatalf("cross-edition query+kind: total=%d cards=%+v, want nothing", total, cards)
	}
	// The symmetric case: the publication filter lives on the manga edition,
	// the query on the anime title.
	cards, total, err = db.SearchLibrary(ctx, LibraryQuery{Query: "animeonly", Publications: []domain.Publication{domain.PublicationCompleted}, Sort: SortTitle, Limit: 10})
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	if total != 0 || len(cards) != 0 {
		t.Fatalf("cross-edition query+publication: total=%d cards=%+v, want nothing", total, cards)
	}
	// The same query with the matching kind lists the work through the
	// anime edition only.
	cards, total, err = db.SearchLibrary(ctx, LibraryQuery{Query: "animeonly", Kinds: []domain.EditionKind{domain.KindAnime}, Sort: SortTitle, Limit: 10})
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	if total != 1 || len(cards) != 1 || len(cards[0].MatchedEditionIDs) != 1 || cards[0].MatchedEditionIDs[0] != anime.ID {
		t.Fatalf("same-edition query+kind: total=%d cards=%+v, want the work through the anime edition", total, cards)
	}
	// A work-title match counts for the filter: the work is listed and the
	// matched editions are the ones satisfying the filter.
	cards, total, err = db.SearchLibrary(ctx, LibraryQuery{Query: "correlated", Kinds: []domain.EditionKind{domain.KindManga}, Sort: SortTitle, Limit: 10})
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	if total != 1 || len(cards) != 1 || len(cards[0].MatchedEditionIDs) != 1 || cards[0].MatchedEditionIDs[0] != manga.ID {
		t.Fatalf("work-title query+kind: total=%d cards=%+v, want the work through the manga edition", total, cards)
	}
	// The query matches a work title and the filter is satisfied by another
	// edition: still listed, matched through the filter-satisfying edition.
	cards, total, err = db.SearchLibrary(ctx, LibraryQuery{Query: "correlated", Publications: []domain.Publication{domain.PublicationOngoing}, Sort: SortTitle, Limit: 10})
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	if total != 1 || len(cards) != 1 || len(cards[0].MatchedEditionIDs) != 1 || cards[0].MatchedEditionIDs[0] != anime.ID {
		t.Fatalf("work-title query+publication: total=%d cards=%+v, want the work through the ongoing anime edition", total, cards)
	}
}

// TestLibrarySearchQueryFilterPagination proves the correlated query and
// filter predicate paginates correctly: the seek condition applies to the
// whole disjunction, so later pages never re-serve a work from the
// work-title branch or skip a correlated work.
func TestLibrarySearchQueryFilterPagination(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	ctx := ctx(t)
	// Work-title branch: the work title matches, the anime edition does not.
	alpha := mustWork(t, db, now, "Alpha Story")
	mustEdition(t, db, now, alpha, domain.KindAnime, "Alpha Anime Line")
	// Correlated branch: only the anime edition title matches.
	beta := mustWork(t, db, now, "Beta")
	mustEdition(t, db, now, beta, domain.KindAnime, "Beta Story Anime")
	// Work-title branch again, after the correlated work.
	gamma := mustWork(t, db, now, "Gamma Story")
	mustEdition(t, db, now, gamma, domain.KindAnime, "Gamma Anime Line")

	query := LibraryQuery{Query: "story", Kinds: []domain.EditionKind{domain.KindAnime}, Sort: SortTitle, Limit: 1}
	page1, total, err := db.SearchLibrary(ctx, query)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if total != 3 || len(page1) != 1 || page1[0].ID != alpha.ID {
		t.Fatalf("page 1 = %v (total %d), want the alpha work only", page1, total)
	}
	after := page1[0]
	page2, _, err := db.SearchLibrary(ctx, LibraryQuery{Query: query.Query, Kinds: query.Kinds, Sort: query.Sort, Limit: 1, AfterKey: after.SortKey, AfterID: after.ID})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page2) != 1 || page2[0].ID != beta.ID {
		t.Fatalf("page 2 = %v, want the correlated beta work only (no re-served work-title work)", page2)
	}
	after = page2[0]
	page3, _, err := db.SearchLibrary(ctx, LibraryQuery{Query: query.Query, Kinds: query.Kinds, Sort: query.Sort, Limit: 1, AfterKey: after.SortKey, AfterID: after.ID})
	if err != nil {
		t.Fatalf("page 3: %v", err)
	}
	if len(page3) != 1 || page3[0].ID != gamma.ID {
		t.Fatalf("page 3 = %v, want the gamma work only", page3)
	}
}

// TestLibrarySearchSorts proves the three sort modes with the stable ID
// tie-breaker.
func TestLibrarySearchSorts(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	ctx := ctx(t)
	a := mustWork(t, db, now, "Zeta")
	b := mustWork(t, db, now, "alpha")
	c := mustWork(t, db, now, "Beta")
	// Same preferred key, different IDs: the ID breaks the tie.
	d, err := db.CreateWork(ctx, now, []TitleInput{{Text: "Alpha", Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}

	cards, _, err := db.SearchLibrary(ctx, LibraryQuery{Sort: SortTitle, Limit: 10})
	if err != nil {
		t.Fatalf("title sort: %v", err)
	}
	// alpha < Alpha < Beta < zeta (case-folded keys); the two alpha works
	// tie on the key, so their random UUIDs decide the order.
	first, second := b.ID, d.ID
	if first > second {
		first, second = second, first
	}
	want := []string{first, second, c.ID, a.ID}
	if len(cards) != 4 {
		t.Fatalf("title sort: %d cards, want 4", len(cards))
	}
	for i, id := range want {
		if cards[i].ID != id {
			t.Fatalf("title sort position %d = %s, want %s (order: %v)", i, cards[i].ID, id, cardIDs(cards))
		}
	}

	cards, _, err = db.SearchLibrary(ctx, LibraryQuery{Sort: SortCreatedAt, Limit: 10})
	if err != nil {
		t.Fatalf("created sort: %v", err)
	}
	// All works share the same second; the ID tie-breaker orders them.
	var createdOrder []string
	for _, c := range cards {
		createdOrder = append(createdOrder, c.ID)
	}
	for i := 1; i < len(createdOrder); i++ {
		if createdOrder[i-1] > createdOrder[i] {
			t.Fatalf("created sort not stable: %v", createdOrder)
		}
	}

	// Touch one work's updated_at and check the updated sort moves it last.
	if _, err := db.AddWorkTitle(ctx, now.Add(time.Hour), a.ID, a.Revision, TitleInput{Text: "Zeta extra", Kind: domain.TitleAlias, Provenance: "manual"}); err != nil {
		t.Fatalf("AddWorkTitle: %v", err)
	}
	cards, _, err = db.SearchLibrary(ctx, LibraryQuery{Sort: SortUpdatedAt, Limit: 10})
	if err != nil {
		t.Fatalf("updated sort: %v", err)
	}
	if cards[len(cards)-1].ID != a.ID {
		t.Fatalf("updated sort: last = %s, want the touched work %s", cards[len(cards)-1].ID, a.ID)
	}
}

func cardIDs(cards []WorkCard) []string {
	out := make([]string, 0, len(cards))
	for _, c := range cards {
		out = append(out, c.ID)
	}
	return out
}

// TestLibrarySearchSortPagination proves seek pagination covers every work
// for each sort mode: the cursor value and the seek clause must use the
// same ordering (search key for title, the timestamp column otherwise).
func TestLibrarySearchSortPagination(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	base := libraryNow()
	ctx := ctx(t)
	ids := []string{}
	for i, name := range []string{"W1", "W2", "W3", "W4"} {
		ids = append(ids, mustWork(t, db, base.Add(time.Duration(i)*time.Second), name).ID)
	}
	serveAll := func(sort SortMode) []string {
		served := []string{}
		var afterKey, afterID string
		for page := 0; page < 4; page++ {
			cards, _, err := db.SearchLibrary(ctx, LibraryQuery{Sort: sort, Limit: 2, AfterKey: afterKey, AfterID: afterID})
			if err != nil {
				t.Fatalf("%s page %d: %v", sort, page+1, err)
			}
			for _, c := range cards {
				served = append(served, c.ID)
			}
			if len(cards) == 0 {
				break
			}
			last := cards[len(cards)-1]
			afterKey, afterID = last.SortKey, last.ID
		}
		return served
	}

	// created_at sort: pages of two cover all four works in creation order.
	served := serveAll(SortCreatedAt)
	if len(served) != 4 {
		t.Fatalf("created_at pages served %d works, want 4: %v", len(served), served)
	}
	for i, id := range ids {
		if served[i] != id {
			t.Fatalf("created_at order = %v, want the creation order %v", served, ids)
		}
	}

	// updated_at sort: touching the oldest work moves it last, and the
	// pages still cover the whole library.
	if _, err := db.AddWorkTitle(ctx, base.Add(10*time.Second), ids[0], 0, TitleInput{Text: "W1 extra", Kind: domain.TitleAlias, Provenance: "manual"}); err != nil {
		t.Fatalf("AddWorkTitle: %v", err)
	}
	served = serveAll(SortUpdatedAt)
	if len(served) != 4 {
		t.Fatalf("updated_at pages served %d works, want 4: %v", len(served), served)
	}
	if served[3] != ids[0] {
		t.Fatalf("updated_at order = %v, want the touched work last", served)
	}
}

// TestLibrarySearchQueryBound proves an empty or whitespace-only query
// lists everything, and an invalid sort or limit is rejected.
func TestLibrarySearchQueryBound(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	ctx := ctx(t)
	mustWork(t, db, now, "One")
	mustWork(t, db, now, "Two")
	cards, total, err := db.SearchLibrary(ctx, LibraryQuery{Query: "   ", Sort: SortTitle, Limit: 10})
	if err != nil {
		t.Fatalf("empty query: %v", err)
	}
	if total != 2 || len(cards) != 2 {
		t.Fatalf("empty query: total=%d, want the whole library", total)
	}
	if _, _, err := db.SearchLibrary(ctx, LibraryQuery{Sort: "bogus"}); err == nil {
		t.Fatal("invalid sort: want an error")
	}
	if _, _, err := db.SearchLibrary(ctx, LibraryQuery{Sort: SortTitle, Limit: MaxLibraryLimit + 2}); err == nil {
		t.Fatal("over-limit: want an error")
	}
}

// TestLibraryArchive proves archiving preserves history, records the
// instant, is idempotent per state, and unarchiving clears the instant.
func TestLibraryArchive(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	ctx := ctx(t)
	w := mustWork(t, db, now, "Archive Me")
	e, w := mustEdition(t, db, now, w, domain.KindManga, "Archive Edition")

	archived, err := db.SetWorkArchived(ctx, now, w.ID, w.Revision, true, nil)
	if err != nil {
		t.Fatalf("archive work: %v", err)
	}
	if archived.ArchivedAt == nil || archived.Revision != w.Revision+1 {
		t.Fatalf("archived work = %+v, want archived_at set and revision bumped", archived)
	}
	// Same state again: no revision bump.
	again, err := db.SetWorkArchived(ctx, now, w.ID, archived.Revision, true, nil)
	if err != nil {
		t.Fatalf("re-archive: %v", err)
	}
	if again.Revision != archived.Revision {
		t.Fatalf("re-archive revision = %d, want unchanged %d", again.Revision, archived.Revision)
	}
	// History survives: titles and editions are intact.
	loaded, err := db.GetWork(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if len(loaded.Titles) != 1 {
		t.Fatalf("archived work titles = %d, want the history kept", len(loaded.Titles))
	}
	ed, err := db.ListEditions(ctx, w.ID)
	if err != nil {
		t.Fatalf("ListEditions: %v", err)
	}
	if len(ed) != 1 || len(ed[0].Titles) != 1 {
		t.Fatalf("archived work editions = %+v, want the history kept", ed)
	}
	// Unarchive clears the instant.
	restored, err := db.SetWorkArchived(ctx, now, w.ID, archived.Revision, false, nil)
	if err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	if restored.ArchivedAt != nil {
		t.Fatalf("restored work archived_at = %v, want null", restored.ArchivedAt)
	}
	// A stale revision is a distinguishable conflict.
	if _, err := db.SetWorkArchived(ctx, now, w.ID, 0, true, nil); err == nil {
		t.Fatal("stale archive: want a conflict")
	}
	// Edition archive keeps its titles and publication history.
	edArchived, err := db.SetEditionArchived(ctx, now, e.ID, e.Revision, true, nil)
	if err != nil {
		t.Fatalf("archive edition: %v", err)
	}
	if edArchived.ArchivedAt == nil || edArchived.Revision != e.Revision+1 {
		t.Fatalf("archived edition = %+v, want archived_at set and revision bumped", edArchived)
	}
	edLoaded, err := db.GetEdition(ctx, e.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if len(edLoaded.Titles) != 1 {
		t.Fatalf("archived edition titles = %d, want the history kept", len(edLoaded.Titles))
	}
}

// TestLibraryUpdateEditionMeta proves metadata correction: partial updates,
// validation of the resulting combination, the same-value no-op and the
// stale-revision conflict.
func TestLibraryUpdateEditionMeta(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	ctx := ctx(t)
	w := mustWork(t, db, now, "Meta Story")
	e, _ := mustEdition(t, db, now, w, domain.KindManga)

	updated, err := db.UpdateEditionMeta(ctx, now, e.ID, e.Revision, kindPtr(domain.KindAnime), nil, nil, nil)
	if err != nil {
		t.Fatalf("update kind: %v", err)
	}
	if updated.Kind != domain.KindAnime || updated.Revision != e.Revision+1 {
		t.Fatalf("updated edition = %+v, want anime at revision %d", updated, e.Revision+1)
	}
	// other requires a label; the combination is validated.
	if _, err := db.UpdateEditionMeta(ctx, now, e.ID, updated.Revision, kindPtr(domain.KindOther), nil, nil, nil); err == nil {
		t.Fatal("other kind without a label: want an error")
	}
	labeled, err := db.UpdateEditionMeta(ctx, now, e.ID, updated.Revision, kindPtr(domain.KindOther), nil, strPtr("radio drama"), nil)
	if err != nil {
		t.Fatalf("update other+label: %v", err)
	}
	if labeled.Kind != domain.KindOther || labeled.OtherLabel == nil || *labeled.OtherLabel != "radio drama" {
		t.Fatalf("labeled edition = %+v, want other with the label", labeled)
	}
	// Same-value no-op: no revision bump.
	same, err := db.UpdateEditionMeta(ctx, now, e.ID, labeled.Revision, kindPtr(domain.KindOther), nil, strPtr("radio drama"), nil)
	if err != nil {
		t.Fatalf("no-op update: %v", err)
	}
	if same.Revision != labeled.Revision {
		t.Fatalf("no-op revision = %d, want unchanged %d", same.Revision, labeled.Revision)
	}
	// Stale revision conflicts.
	if _, err := db.UpdateEditionMeta(ctx, now, e.ID, 0, kindPtr(domain.KindManga), nil, nil, nil); err == nil {
		t.Fatal("stale update: want a conflict")
	}
}

// TestLibraryApplyTitles proves one-transaction title corrections on works
// and editions: add with provenance, retire keeping history, re-pointing
// the preferred title, the work last-title guard and the edition
// empty-fallback, plus the journal row committed with the change.
func TestLibraryApplyTitles(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	ctx := ctx(t)
	w := mustWork(t, db, now, "Correct Me")
	// Add two titles, one preferred.
	added, err := db.ApplyWorkTitles(ctx, now, w.ID, w.Revision, TitleMutation{
		Add: []TitleInput{
			{Text: "Corrected (ja)", Kind: domain.TitleLocalized, Language: strPtr("ja"), Provenance: "user", Preferred: true},
			{Text: "Corrected (romaji)", Kind: domain.TitleRomanized, Provenance: "user"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("add titles: %v", err)
	}
	if len(added.Titles) != 3 {
		t.Fatalf("work titles = %d, want 3", len(added.Titles))
	}
	preferred := 0
	var jaID string
	for _, t := range added.Titles {
		if t.Preferred {
			preferred++
			jaID = t.ID
		}
	}
	if preferred != 1 {
		t.Fatalf("preferred count = %d, want 1", preferred)
	}
	// Retire the original title; history is kept.
	retired, err := db.ApplyWorkTitles(ctx, now, added.ID, added.Revision, TitleMutation{Retire: []string{w.Titles[0].ID}}, nil)
	if err != nil {
		t.Fatalf("retire title: %v", err)
	}
	if len(retired.Titles) != 2 {
		t.Fatalf("retired work titles = %d, want 2 active", len(retired.Titles))
	}
	var raw string
	if err := db.DB().QueryRow(`SELECT text FROM work_titles WHERE id = ?`, w.Titles[0].ID).Scan(&raw); err != nil {
		t.Fatalf("retired row missing: %v", err)
	}
	if raw != "Correct Me" {
		t.Fatalf("retired row = %q, want the original text kept", raw)
	}
	// Re-point the preferred title.
	repointed, err := db.ApplyWorkTitles(ctx, now, retired.ID, retired.Revision, TitleMutation{Preferred: strPtr(jaID)}, nil)
	if err != nil {
		t.Fatalf("re-point preferred: %v", err)
	}
	for _, title := range repointed.Titles {
		if title.ID != jaID && title.Preferred {
			t.Fatalf("preferred = %s, want %s", title.ID, jaID)
		}
	}
	// The work keeps its last active title.
	lastIDs := make([]string, 0, len(repointed.Titles))
	for _, t := range repointed.Titles {
		lastIDs = append(lastIDs, t.ID)
	}
	if _, err := db.ApplyWorkTitles(ctx, now, repointed.ID, repointed.Revision, TitleMutation{Retire: lastIDs}, nil); err == nil {
		t.Fatal("retiring all work titles: want an error")
	}
	// An edition may empty out and a new first title becomes preferred.
	e, _ := mustEdition(t, db, now, repointed, domain.KindManga, "Old Edition Title")
	emptied, err := db.ApplyEditionTitles(ctx, now, e.ID, e.Revision, TitleMutation{Retire: []string{e.Titles[0].ID}}, nil)
	if err != nil {
		t.Fatalf("empty edition: %v", err)
	}
	if len(emptied.Titles) != 0 {
		t.Fatalf("emptied edition titles = %d, want 0 (falls back to the work title)", len(emptied.Titles))
	}
	refilled, err := db.ApplyEditionTitles(ctx, now, emptied.ID, emptied.Revision, TitleMutation{
		Add: []TitleInput{{Text: "New Edition Title", Kind: domain.TitleOriginal, Provenance: "user"}},
	}, nil)
	if err != nil {
		t.Fatalf("refill edition: %v", err)
	}
	if len(refilled.Titles) != 1 || !refilled.Titles[0].Preferred {
		t.Fatalf("refilled edition = %+v, want the new preferred title", refilled.Titles)
	}
	// A correction with a journal row commits both in one transaction.
	w2 := mustWork(t, db, now, "Journal Work")
	_ = w2
	journal := &JournalInput{RequestID: "lib-1", Operation: "correct_work_titles", RequestHash: "h", Actor: "local"}
	if _, err := db.ApplyWorkTitles(ctx, now, w2.ID, w2.Revision, TitleMutation{
		Add: []TitleInput{{Text: "Journal Title", Kind: domain.TitleAlias, Provenance: "user"}},
	}, journal); err != nil {
		t.Fatalf("journal correction: %v", err)
	}
	entry, err := db.LookupJournal(ctx, "lib-1")
	if err != nil {
		t.Fatalf("LookupJournal: %v", err)
	}
	if entry == nil || !strings.Contains(entry.ResultJSON, w2.ID) {
		t.Fatalf("journal row = %+v, want the correction result", entry)
	}
}
