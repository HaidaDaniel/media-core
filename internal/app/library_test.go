package app

import (
	"context"
	"strings"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func mustCreateWork(t *testing.T, s *Service, text string) store.Work {
	t.Helper()
	w, err := s.db.CreateWork(context.Background(), s.now(), []store.TitleInput{{Text: text, Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	return w
}

func addWorkTitle(t *testing.T, s *Service, workID string, text string) string {
	t.Helper()
	ctx := context.Background()
	w, err := s.db.GetWork(ctx, workID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	added, err := s.db.AddWorkTitle(ctx, s.now(), workID, w.Revision, store.TitleInput{Text: text, Kind: domain.TitleOriginal, Provenance: "manual"})
	if err != nil {
		t.Fatalf("AddWorkTitle: %v", err)
	}
	for _, title := range added.Titles {
		if title.Text == text {
			return title.ID
		}
	}
	t.Fatalf("title %q not found in %+v", text, added.Titles)
	return ""
}

func addEdition(t *testing.T, s *Service, workID string, kind domain.EditionKind, titles ...string) store.Edition {
	t.Helper()
	ctx := context.Background()
	w, err := s.db.GetWork(ctx, workID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	in := store.EditionInput{Kind: kind}
	for _, text := range titles {
		in.Titles = append(in.Titles, store.TitleInput{Text: text, Kind: domain.TitleOriginal, Provenance: "manual"})
	}
	e, err := s.db.AddEdition(ctx, s.now(), workID, w.Revision, in)
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	return e
}

func listCards(t *testing.T, s *Service, f LibraryFilter) []WorkCard {
	t.Helper()
	ctx := context.Background()
	payload, err := s.SearchLibrary(ctx, f)
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	cards := make([]WorkCard, 0, len(payload.Items))
	for _, item := range payload.Items {
		cards = append(cards, item.(WorkCard))
	}
	return cards
}

// TestLibrarySearchAliases proves Japanese, Russian, English and romaji
// spellings find the same work, and that an edition-title match identifies
// the matching edition.
func TestLibrarySearchAliases(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	w, err := s.db.CreateWork(ctx, s.now(), []store.TitleInput{{Text: "Frieren", Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	addWorkTitle(t, s, w.ID, "Frieren: Beyond Journey's End")
	addWorkTitle(t, s, w.ID, "葬送のフリーレン")
	addWorkTitle(t, s, w.ID, "Фрирен")
	manga := addEdition(t, s, w.ID, domain.KindManga, "Frieren Manga Edition")
	addEdition(t, s, w.ID, domain.KindAnime)

	for _, query := range []string{"frieren", "FRIEREN", "beyond journey", "フリーレン", "фрирен"} {
		cards := listCards(t, s, LibraryFilter{Query: &query, Sort: SortTitle})
		if len(cards) != 1 || cards[0].WorkID != w.ID {
			t.Fatalf("query %q: cards = %+v, want the one aliased work", query, cards)
		}
	}
	cards := listCards(t, s, LibraryFilter{Query: strPtr("frieren manga"), Sort: SortTitle})
	if len(cards) != 1 || cards[0].WorkID != w.ID {
		t.Fatalf("edition query: cards = %+v, want the aliased work", cards)
	}
	if len(cards[0].MatchedEditionIDs) != 1 || cards[0].MatchedEditionIDs[0] != manga.ID {
		t.Fatalf("matched editions = %v, want the manga edition", cards[0].MatchedEditionIDs)
	}
	if len(cards[0].Editions) != 2 {
		t.Fatalf("card editions = %+v, want both edition lines", cards[0].Editions)
	}
}

// TestLibrarySearchQueryFilterCorrelation proves the query is correlated
// with the edition-qualified filters: a work is listed only when one
// edition satisfies both, or the query hits a work title and some edition
// satisfies the filter.
func TestLibrarySearchQueryFilterCorrelation(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	w, err := s.db.CreateWork(ctx, s.now(), []store.TitleInput{{Text: "Correlated Story", Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	manga := addEdition(t, s, w.ID, domain.KindManga, "MangaOnly Title")
	fresh, err := s.db.GetWork(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	anime := addEdition(t, s, fresh.ID, domain.KindAnime, "AnimeOnly Title")
	if _, err := s.db.RecordPublication(ctx, s.now(), manga.ID, manga.Revision, store.PublicationInput{Value: domain.PublicationCompleted, Source: appStr("manual"), Provenance: "manual"}); err != nil {
		t.Fatalf("RecordPublication manga: %v", err)
	}
	if _, err := s.db.RecordPublication(ctx, s.now(), anime.ID, anime.Revision, store.PublicationInput{Value: domain.PublicationOngoing, Source: appStr("manual"), Provenance: "manual"}); err != nil {
		t.Fatalf("RecordPublication anime: %v", err)
	}

	// The query matches only the anime title; the kind filter is satisfied
	// only by the manga edition. No single edition satisfies both.
	cards := listCards(t, s, LibraryFilter{Query: strPtr("animeonly"), Kinds: []domain.EditionKind{domain.KindManga}, Sort: SortTitle})
	if len(cards) != 0 {
		t.Fatalf("cross-edition query+kind = %+v, want nothing", cards)
	}
	cards = listCards(t, s, LibraryFilter{Query: strPtr("animeonly"), Publications: []domain.Publication{domain.PublicationCompleted}, Sort: SortTitle})
	if len(cards) != 0 {
		t.Fatalf("cross-edition query+publication = %+v, want nothing", cards)
	}
	cards = listCards(t, s, LibraryFilter{Query: strPtr("animeonly"), Kinds: []domain.EditionKind{domain.KindAnime}, Sort: SortTitle})
	if len(cards) != 1 || len(cards[0].MatchedEditionIDs) != 1 || cards[0].MatchedEditionIDs[0] != anime.ID {
		t.Fatalf("same-edition query+kind = %+v, want the work through the anime edition", cards)
	}
	cards = listCards(t, s, LibraryFilter{Query: strPtr("correlated"), Kinds: []domain.EditionKind{domain.KindManga}, Sort: SortTitle})
	if len(cards) != 1 || len(cards[0].MatchedEditionIDs) != 1 || cards[0].MatchedEditionIDs[0] != manga.ID {
		t.Fatalf("work-title query+kind = %+v, want the work through the manga edition", cards)
	}
}

// TestLibrarySearchCursorBinding proves a cursor is bound to the sort,
// limit and filter of the listing that issued it.
func TestLibrarySearchCursorBinding(t *testing.T) {
	s := newTestService(t)
	var ids []string
	for _, text := range []string{"Alpha", "Beta", "Gamma", "Delta"} {
		ids = append(ids, mustCreateWork(t, s, text).ID)
	}
	first, err := s.SearchLibrary(context.Background(), LibraryFilter{Sort: SortTitle, Limit: 2})
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	if first.NextCursor == nil {
		t.Fatal("first page: want a next cursor")
	}
	cursor := *first.NextCursor
	base := func() LibraryFilter { return LibraryFilter{Sort: SortTitle, Limit: 2, Cursor: &cursor} }
	cases := map[string]LibraryFilter{
		"different limit": {Sort: SortTitle, Limit: 3, Cursor: &cursor},
		"different sort":  {Sort: SortUpdatedAt, Limit: 2, Cursor: &cursor},
		"different query": {Query: strPtr("alpha"), Sort: SortTitle, Limit: 2, Cursor: &cursor},
		"different kinds": {Kinds: []domain.EditionKind{domain.KindManga}, Sort: SortTitle, Limit: 2, Cursor: &cursor},
		"garbage cursor":  base(),
	}
	for name, f := range cases {
		if name == "garbage cursor" {
			f.Cursor = strPtr("not-base64!!")
		}
		if _, err := s.SearchLibrary(context.Background(), f); err == nil {
			t.Fatalf("%s: want an invalid-argument error", name)
		} else if ae := appError(t, err); ae.Code != ErrInvalidArgument {
			t.Fatalf("%s: code = %s, want invalid_argument", name, ae.Code)
		}
	}
	// The cursor seeks after the last served row (Beta), so the second
	// page holds the remaining two works and the pages cover the total.
	second, err := s.SearchLibrary(context.Background(), base())
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if second.Total != 4 || len(second.Items) != 2 {
		t.Fatalf("second page = %+v, want two items and total 4", second)
	}
	delta := second.Items[0].(WorkCard)
	gamma := second.Items[1].(WorkCard)
	if delta.WorkID != ids[3] || gamma.WorkID != ids[2] {
		t.Fatalf("second page = %v, %v, want delta %s then gamma %s", delta.WorkID, gamma.WorkID, ids[3], ids[2])
	}
	if second.NextCursor != nil {
		t.Fatalf("second page cursor = %v, want none", second.NextCursor)
	}
}

// TestLibrarySearchValidation proves the bounded input surface: the sort,
// the limit and the query length are all checked before any lookup.
func TestLibrarySearchValidation(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	if _, err := s.SearchLibrary(ctx, LibraryFilter{Sort: "rating"}); err == nil || appError(t, err).Code != ErrInvalidArgument {
		t.Fatal("unknown sort: want invalid_argument")
	}
	for _, limit := range []int{-1, 101} {
		if _, err := s.SearchLibrary(ctx, LibraryFilter{Sort: SortTitle, Limit: limit}); err == nil || appError(t, err).Code != ErrInvalidArgument {
			t.Fatalf("limit %d: want invalid_argument, got %v", limit, err)
		}
	}
	long := strings.Repeat("a", MaxLibraryQueryRunes+1)
	if _, err := s.SearchLibrary(ctx, LibraryFilter{Query: &long, Sort: SortTitle}); err == nil || appError(t, err).Code != ErrInvalidArgument {
		t.Fatal("long query: want invalid_argument")
	}
	if _, err := s.SearchLibrary(ctx, LibraryFilter{Kinds: []domain.EditionKind{domain.EditionKind("nope")}, Sort: SortTitle}); err == nil || appError(t, err).Code != ErrInvalidArgument {
		t.Fatal("unknown kind: want invalid_argument")
	}
}

// TestLibrarySearchTotalAndLimit proves the total counts distinct filtered
// works and the page honours the limit with the probe row hidden.
func TestLibrarySearchTotalAndLimit(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Twin Peaks")
	mustCreateWork(t, s, "Other Story")
	addEdition(t, s, w.ID, domain.KindManga, "Twin Peaks Manga")
	addEdition(t, s, w.ID, domain.KindAnime, "Twin Peaks Anime")

	payload, err := s.SearchLibrary(context.Background(), LibraryFilter{Sort: SortTitle, Limit: 1})
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	if payload.Total != 2 || len(payload.Items) != 1 || payload.NextCursor == nil {
		t.Fatalf("payload = %+v, want total 2, one item and a cursor", payload)
	}
	cards := listCards(t, s, LibraryFilter{Kinds: []domain.EditionKind{domain.KindManga}, Sort: SortTitle})
	if len(cards) != 1 || cards[0].WorkID != w.ID {
		t.Fatalf("kind filter = %+v, want the work with a manga edition", cards)
	}
}

// TestLibraryGetMissing proves missing ids and blank ids return the stable
// coded errors before anything touches storage.
func TestLibraryGetMissing(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	if _, err := s.GetWork(ctx, "no-such-work"); err == nil || appError(t, err).Code != ErrNotFound {
		t.Fatalf("GetWork missing: got %v, want not_found", err)
	}
	if _, err := s.GetEdition(ctx, "no-such-edition"); err == nil || appError(t, err).Code != ErrNotFound {
		t.Fatalf("GetEdition missing: got %v, want not_found", err)
	}
	if _, err := s.GetWork(ctx, "  "); err == nil || appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("GetWork blank: got %v, want invalid_argument", err)
	}
	if _, err := s.GetEdition(ctx, ""); err == nil || appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("GetEdition blank: got %v, want invalid_argument", err)
	}
}

func mustCreateWorkRevision(t *testing.T, s *Service, workID string) store.Work {
	t.Helper()
	w, err := s.db.GetWork(context.Background(), workID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	return w
}

func titleReq(requestID string, targetID string, revision int64) TitleMutationRequest {
	return TitleMutationRequest{
		MutationBase: MutationBase{
			RequestID:        requestID,
			Actor:            Actor{Principal: "local", Origin: domain.OriginUser},
			ExpectedRevision: &revision,
		},
		TargetID: targetID,
	}
}

// TestLibraryCorrectWorkTitles proves one atomic title correction: add,
// retire and re-point in a single mutation; a stale revision conflicts;
// the same key and payload replays; the same key with a different payload
// is a collision.
func TestLibraryCorrectWorkTitles(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	w := mustCreateWork(t, s, "Original Name")
	originalID := w.Titles[0].ID
	rev := w.Revision

	req := titleReq("t-fix", w.ID, rev)
	req.Add = []store.TitleInput{{Text: "Corrected Name", Kind: domain.TitleOriginal, Provenance: "manual"}}
	req.Retire = []string{originalID}
	view, err := s.CorrectWorkTitles(ctx, req)
	if err != nil {
		t.Fatalf("CorrectWorkTitles: %v", err)
	}
	if view.Revision != rev+1 || len(view.Titles) != 1 || view.Titles[0].Text != "Corrected Name" || !view.Titles[0].Preferred {
		t.Fatalf("view = %+v, want the corrected preferred title at revision %d", view, rev+1)
	}

	// A stale expected revision is a conflict.
	stale := titleReq("t-stale", w.ID, rev)
	stale.Retire = []string{view.Titles[0].ID}
	if _, err := s.CorrectWorkTitles(ctx, stale); err == nil || appError(t, err).Code != ErrConflict {
		t.Fatalf("stale revision: got %v, want conflict", err)
	}

	// The same key and payload replays the stored result without another
	// revision bump.
	replay, err := s.CorrectWorkTitles(ctx, req)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Revision != view.Revision || replay.ID != w.ID {
		t.Fatalf("replay = %+v, want the same view", replay)
	}

	// The same key with a different payload is a collision.
	collision := req
	collision.Add = []store.TitleInput{{Text: "Another Name", Kind: domain.TitleOriginal, Provenance: "manual"}}
	if _, err := s.CorrectWorkTitles(ctx, collision); err == nil || appError(t, err).Code != ErrConflict {
		t.Fatalf("collision: got %v, want conflict", err)
	}

	// A work must keep at least one active title.
	current, err := s.db.GetWork(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	only := titleReq("t-last", w.ID, current.Revision)
	for _, title := range current.Titles {
		only.Retire = append(only.Retire, title.ID)
	}
	if _, err := s.CorrectWorkTitles(ctx, only); err == nil || appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("retire the last title: got %v, want invalid_argument", err)
	}
}

// TestLibraryCorrectEditionTitles proves edition title corrections: adding
// a title makes it available for display, retiring leaves the work title as
// the fallback, and the first remaining title becomes preferred.
func TestLibraryCorrectEditionTitles(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	w := mustCreateWork(t, s, "Work With Edition")
	e := addEdition(t, s, w.ID, domain.KindManga, "Edition Original")
	originalID := e.Titles[0].ID

	req := titleReq("e-fix", e.ID, e.Revision)
	req.Add = []store.TitleInput{{Text: "Edition Corrected", Kind: domain.TitleOriginal, Provenance: "manual"}}
	req.Retire = []string{originalID}
	view, err := s.CorrectEditionTitles(ctx, req)
	if err != nil {
		t.Fatalf("CorrectEditionTitles: %v", err)
	}
	if len(view.Titles) != 1 || view.Titles[0].Text != "Edition Corrected" || !view.Titles[0].Preferred {
		t.Fatalf("view = %+v, want the corrected preferred edition title", view)
	}

	// Retiring the only edition title is allowed: the work title is the
	// fallback and the edition may be empty.
	empty := titleReq("e-empty", e.ID, view.Revision)
	empty.Retire = []string{view.Titles[0].ID}
	after, err := s.CorrectEditionTitles(ctx, empty)
	if err != nil {
		t.Fatalf("empty the edition: %v", err)
	}
	if len(after.Titles) != 0 {
		t.Fatalf("emptied edition titles = %+v, want none", after.Titles)
	}

	// Adding to an empty edition re-points the first title as preferred.
	refill := titleReq("e-refill", e.ID, after.Revision)
	refill.Add = []store.TitleInput{
		{Text: "First New", Kind: domain.TitleOriginal, Provenance: "manual"},
		{Text: "Second New", Kind: domain.TitleOriginal, Provenance: "manual"},
	}
	final, err := s.CorrectEditionTitles(ctx, refill)
	if err != nil {
		t.Fatalf("refill: %v", err)
	}
	if len(final.Titles) != 2 || !final.Titles[0].Preferred {
		t.Fatalf("refilled titles = %+v, want two titles, first preferred", final.Titles)
	}
}

// TestLibraryArchiveWork proves archiving preserves titles and editions,
// the same state is a no-op without a revision bump, and unarchive clears
// the instant.
func TestLibraryArchiveWork(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	w := mustCreateWork(t, s, "Archive Me")
	addEdition(t, s, w.ID, domain.KindManga)
	w = mustCreateWorkRevision(t, s, w.ID)
	base := MutationBase{RequestID: "a-1", Actor: Actor{Principal: "local", Origin: domain.OriginUser}, ExpectedRevision: &w.Revision}

	archived, err := s.ArchiveWork(ctx, base, w.ID, true)
	if err != nil {
		t.Fatalf("ArchiveWork: %v", err)
	}
	if archived.ArchivedAt == nil || archived.Revision != w.Revision+1 || len(archived.Titles) != 1 || len(archived.Editions) != 1 {
		t.Fatalf("archived = %+v, want the history preserved at a new revision", archived)
	}

	// Same state again: no revision bump.
	same := base
	same.RequestID = "a-2"
	same.ExpectedRevision = &archived.Revision
	noOp, err := s.ArchiveWork(ctx, same, w.ID, true)
	if err != nil {
		t.Fatalf("same-state archive: %v", err)
	}
	if noOp.Revision != archived.Revision {
		t.Fatalf("same-state archive bumped the revision to %d, want %d", noOp.Revision, archived.Revision)
	}

	// Unarchive clears the instant.
	un := base
	un.RequestID = "a-3"
	un.ExpectedRevision = &archived.Revision
	restored, err := s.ArchiveWork(ctx, un, w.ID, false)
	if err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	if restored.ArchivedAt != nil {
		t.Fatalf("restored archived_at = %v, want null", restored.ArchivedAt)
	}

	// A stale revision conflicts.
	stale := base
	stale.RequestID = "a-4"
	stale.ExpectedRevision = &w.Revision
	if _, err := s.ArchiveWork(ctx, stale, w.ID, true); err == nil || appError(t, err).Code != ErrConflict {
		t.Fatalf("stale archive: got %v, want conflict", err)
	}
}

// TestLibraryArchiveEdition proves edition archiving keeps titles and the
// publication history and honours the revision.
func TestLibraryArchiveEdition(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	w := mustCreateWork(t, s, "Edition Archive")
	e := addEdition(t, s, w.ID, domain.KindManga, "Keep Me")
	base := MutationBase{RequestID: "ea-1", Actor: Actor{Principal: "local", Origin: domain.OriginUser}, ExpectedRevision: &e.Revision}
	archived, err := s.ArchiveEdition(ctx, base, e.ID, true)
	if err != nil {
		t.Fatalf("ArchiveEdition: %v", err)
	}
	if len(archived.Titles) != 1 {
		t.Fatalf("archived edition = %+v, want the title preserved", archived)
	}
	stored, err := s.db.GetEdition(ctx, e.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if stored.ArchivedAt == nil {
		t.Fatalf("stored edition = %+v, want archived_at set", stored)
	}
	if _, err := s.GetWork(ctx, w.ID); err != nil {
		t.Fatalf("work still present: %v", err)
	}
}

// TestLibraryUpdateEdition proves partial metadata correction, the
// same-value no-op, the stale conflict and the field validation.
func TestLibraryUpdateEdition(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	w := mustCreateWork(t, s, "Meta Work")
	e := addEdition(t, s, w.ID, domain.KindManga)
	base := MutationBase{RequestID: "u-1", Actor: Actor{Principal: "local", Origin: domain.OriginUser}, ExpectedRevision: &e.Revision}

	req := UpdateEditionRequest{MutationBase: base, EditionID: e.ID, Kind: appKind(domain.KindAnime)}
	updated, err := s.UpdateEdition(ctx, req)
	if err != nil {
		t.Fatalf("UpdateEdition: %v", err)
	}
	if updated.Kind != domain.KindAnime || updated.Revision != e.Revision+1 {
		t.Fatalf("updated = %+v, want anime at a new revision", updated)
	}

	// Same value again: a no-op without a revision bump.
	same := UpdateEditionRequest{
		MutationBase: MutationBase{
			RequestID:        "u-2",
			Actor:            Actor{Principal: "local", Origin: domain.OriginUser},
			ExpectedRevision: &updated.Revision,
		},
		EditionID: e.ID,
		Kind:      appKind(domain.KindAnime),
	}
	noOp, err := s.UpdateEdition(ctx, same)
	if err != nil {
		t.Fatalf("same-value update: %v", err)
	}
	if noOp.Revision != updated.Revision {
		t.Fatalf("same-value update bumped the revision to %d, want %d", noOp.Revision, updated.Revision)
	}

	// Nothing supplied is invalid.
	nothing := same
	nothing.MutationBase.RequestID = "u-3"
	nothing.Kind = nil
	if _, err := s.UpdateEdition(ctx, nothing); err == nil || appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("empty update: got %v, want invalid_argument", err)
	}

	// Stale revision conflicts.
	stale := UpdateEditionRequest{
		MutationBase: MutationBase{
			RequestID:        "u-4",
			Actor:            Actor{Principal: "local", Origin: domain.OriginUser},
			ExpectedRevision: &e.Revision,
		},
		EditionID: e.ID,
		Kind:      appKind(domain.KindManga),
	}
	if _, err := s.UpdateEdition(ctx, stale); err == nil || appError(t, err).Code != ErrConflict {
		t.Fatalf("stale update: got %v, want conflict", err)
	}
}

// TestLibraryMutationBaseValidation proves the mutation identity fields are
// required on every correction and archive entry point.
func TestLibraryMutationBaseValidation(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	w := mustCreateWork(t, s, "Base Check")

	noKey := titleReq("", w.ID, w.Revision)
	noKey.Retire = []string{w.Titles[0].ID}
	if _, err := s.CorrectWorkTitles(ctx, noKey); err == nil || appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("missing request id: got %v, want invalid_argument", err)
	}

	noActor := titleReq("b-1", w.ID, w.Revision)
	noActor.Actor = Actor{}
	noActor.Retire = []string{w.Titles[0].ID}
	if _, err := s.CorrectWorkTitles(ctx, noActor); err == nil || appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("missing actor: got %v, want invalid_argument", err)
	}

	noRevision := TitleMutationRequest{
		MutationBase: MutationBase{RequestID: "b-2", Actor: Actor{Principal: "local", Origin: domain.OriginUser}},
		TargetID:     w.ID,
		Retire:       []string{w.Titles[0].ID},
	}
	if _, err := s.CorrectWorkTitles(ctx, noRevision); err == nil || appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("missing expected revision: got %v, want invalid_argument", err)
	}

	noTarget := titleReq("b-3", "  ", w.Revision)
	noTarget.Retire = []string{w.Titles[0].ID}
	if _, err := s.CorrectWorkTitles(ctx, noTarget); err == nil || appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("missing target: got %v, want invalid_argument", err)
	}

	empty := titleReq("b-4", w.ID, w.Revision)
	if _, err := s.CorrectWorkTitles(ctx, empty); err == nil || appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("empty mutation: got %v, want invalid_argument", err)
	}
}
