package app

import (
	"context"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// seedActivityService plants one work with two editions and a few history
// rows through the store, so the service layer is tested without
// reserving mutation keys.
func seedActivityService(t *testing.T, s *Service) (store.Work, store.Edition, store.Edition) {
	t.Helper()
	now := testNow()
	w, err := s.db.CreateWork(context.Background(), now, []store.TitleInput{{Text: "Beta", Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	eA, err := s.db.AddEdition(context.Background(), now, w.ID, w.Revision, store.EditionInput{
		Kind:   domain.KindManga,
		Titles: []store.TitleInput{{Text: "Beta Manga", Kind: domain.TitleOriginal, Provenance: "manual"}},
	})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	eB, err := s.db.AddEdition(context.Background(), now, w.ID, w.Revision+1, store.EditionInput{
		Kind:   domain.KindAnime,
		Titles: []store.TitleInput{{Text: "Beta Anime", Kind: domain.TitleOriginal, Provenance: "manual"}},
	})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	ca := store.ConsumptionActor{Principal: "local", Origin: "user"}
	eA, err = s.db.SetConsumption(context.Background(), now.Add(time.Second), eA.ID, eA.Revision, store.ConsumptionInput{Value: "in_progress"}, ca, nil)
	if err != nil {
		t.Fatalf("SetConsumption: %v", err)
	}
	rating := store.RatingActor{Principal: "local", Origin: "user"}
	eA, err = s.db.SetRating(context.Background(), now.Add(2*time.Second), eA.ID, eA.Revision, store.RatingInput{Value: func() *int { n := 9; return &n }()}, rating, nil)
	if err != nil {
		t.Fatalf("SetRating: %v", err)
	}
	eA, err = s.db.AddEditionNote(context.Background(), now.Add(3*time.Second), eA.ID, store.NoteInput{Text: "A note"}, store.NoteActor{Principal: "local", Origin: "user"}, nil)
	if err != nil {
		t.Fatalf("AddEditionNote: %v", err)
	}
	ta := store.TagActor{Principal: "local", Origin: "user"}
	eA, err = s.db.SetEditionTag(context.Background(), now.Add(4*time.Second), eA.ID, eA.Revision, "cool", false, ta, nil)
	if err != nil {
		t.Fatalf("SetEditionTag: %v", err)
	}
	eB, err = s.db.GetEdition(context.Background(), eB.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if _, err := s.db.SetConsumption(context.Background(), now.Add(5*time.Second), eB.ID, eB.Revision, store.ConsumptionInput{Value: "paused"}, ca, nil); err != nil {
		t.Fatalf("SetConsumption eB: %v", err)
	}
	return w, eA, eB
}

func mustListActivity(t *testing.T, s *Service, q ActivityQuery) ListPayload {
	t.Helper()
	p, err := s.ListActivity(context.Background(), q)
	if err != nil {
		t.Fatalf("ListActivity(%+v): %v", q, err)
	}
	return p
}

func activityItem(t *testing.T, p ListPayload, i int) ActivityItem {
	t.Helper()
	item, ok := p.Items[i].(ActivityItem)
	if !ok {
		t.Fatalf("items[%d] = %T, want ActivityItem", i, p.Items[i])
	}
	return item
}

// TestActivityService lists the recent activity through the service: the
// item shape with its enrichment, the type filter and the cursor
// pagination with the mismatch rejections.
func TestActivityService(t *testing.T) {
	s := newTestService(t)
	w, eA, eB := seedActivityService(t, s)

	// The whole feed: five rows, newest first, enriched with the target's
	// current title, notes and references.
	p := mustListActivity(t, s, ActivityQuery{})
	if p.Total != 5 || len(p.Items) != 5 {
		t.Fatalf("feed = %d items, total %d, want 5 of 5", len(p.Items), p.Total)
	}
	first := activityItem(t, p, 0)
	if first.Type != ActivityConsumption || first.EditionID == nil || *first.EditionID != eB.ID {
		t.Fatalf("first item = %+v, want the eB consumption", first)
	}
	if first.Title == nil || *first.Title != "Beta Anime" {
		t.Fatalf("first item title = %v, want the eB preferred title", first.Title)
	}
	last := activityItem(t, p, 4)
	if last.Type != ActivityConsumption || last.Label != "effective" || last.EditionID == nil || *last.EditionID != eA.ID || last.Title == nil || *last.Title != "Beta Manga" {
		t.Fatalf("last item = %+v, want the eA consumption with the eA title", last)
	}
	if len(last.Notes) != 1 || last.Notes[0].Text != "A note" {
		t.Fatalf("last item notes = %+v, want the current note", last.Notes)
	}
	if last.Actor == nil || last.Actor.Principal != "local" || last.Actor.Origin != domain.OriginUser {
		t.Fatalf("last item actor = %+v, want local user", last.Actor)
	}

	// The type filter keeps only the matching rows.
	p = mustListActivity(t, s, ActivityQuery{Types: []ActivityType{ActivityRating}})
	if p.Total != 1 {
		t.Fatalf("rating feed total = %d, want 1", p.Total)
	}
	if item := activityItem(t, p, 0); item.Type != ActivityRating || item.Label != "effective" || item.Value == nil || *item.Value != "9" {
		t.Fatalf("rating item = %+v, want effective 9", item)
	}

	// Cursor pagination: one row per page, the cursor bound to the limit.
	p1 := mustListActivity(t, s, ActivityQuery{Limit: 1})
	if p1.Total != 5 || len(p1.Items) != 1 || p1.NextCursor == nil {
		t.Fatalf("page 1 = %+v, want 1 of 5 with a cursor", p1)
	}
	p2 := mustListActivity(t, s, ActivityQuery{Limit: 1, Cursor: p1.NextCursor})
	if len(p2.Items) != 1 || p2.NextCursor == nil {
		t.Fatalf("page 2 = %+v, want 1 item with a cursor", p2)
	}
	if activityItem(t, p1, 0).ID == activityItem(t, p2, 0).ID {
		t.Fatalf("pages 1 and 2 share %s", activityItem(t, p1, 0).ID)
	}
	// A cursor from a limit-1 request is rejected on a limit-2 request.
	if _, err := s.ListActivity(context.Background(), ActivityQuery{Limit: 2, Cursor: p1.NextCursor}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("cursor mismatch: err = %v, want invalid_argument", err)
	}
	if _, err := s.ListActivity(context.Background(), ActivityQuery{Cursor: strPtr("garbage")}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("garbage cursor: err = %v, want invalid_argument", err)
	}
	if _, err := s.ListActivity(context.Background(), ActivityQuery{Types: []ActivityType{ActivityType("nope")}}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("unknown type: err = %v, want invalid_argument", err)
	}
	if _, err := s.ListActivity(context.Background(), ActivityQuery{Limit: MaxActivityLimit + 1}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("limit over bound: err = %v, want invalid_argument", err)
	}
	// The work still exists and its state is intact after the reads.
	fresh, err := s.db.GetWork(context.Background(), w.ID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if len(fresh.Titles) != 1 {
		t.Fatalf("work titles = %+v, want the original title", fresh.Titles)
	}
}

// TestHistoryView projects the complete history of one work and one
// edition through the service and keeps the failures distinguishable.
func TestHistoryView(t *testing.T) {
	s := newTestService(t)
	w, eA, eB := seedActivityService(t, s)
	// An edition without any history row; the work revision is fetched
	// fresh, the editions bumped it.
	freshW, err := s.db.GetWork(context.Background(), w.ID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	eC, err := s.db.AddEdition(context.Background(), testNow(), freshW.ID, freshW.Revision, store.EditionInput{
		Kind:   domain.KindManga,
		Titles: []store.TitleInput{{Text: "Beta Second", Kind: domain.TitleOriginal, Provenance: "manual"}},
	})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}

	// Missing entities are stable not-found failures.
	if _, err := s.GetHistory(context.Background(), HistoryQuery{WorkID: strPtr("00000000-0000-4000-8000-000000000000")}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("missing work: err = %v, want not_found", err)
	}
	if _, err := s.GetHistory(context.Background(), HistoryQuery{EditionID: strPtr("00000000-0000-4000-8000-000000000000")}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("missing edition: err = %v, want not_found", err)
	}
	if _, err := s.GetHistory(context.Background(), HistoryQuery{}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("no entity: err = %v, want invalid_argument", err)
	}
	if _, err := s.GetHistory(context.Background(), HistoryQuery{WorkID: strPtr(w.ID), EditionID: strPtr(eA.ID)}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("both entities: err = %v, want invalid_argument", err)
	}

	// The edition history is bounded to the edition's own rows.
	p, err := s.GetHistory(context.Background(), HistoryQuery{EditionID: strPtr(eA.ID)})
	if err != nil {
		t.Fatalf("GetHistory(eA): %v", err)
	}
	if p.Total != 4 {
		t.Fatalf("eA history total = %d, want 4", p.Total)
	}
	for _, itemAny := range p.Items {
		item := itemAny.(ActivityItem)
		if item.EditionID != nil && *item.EditionID != eA.ID {
			t.Fatalf("eA history leaks an item outside eA: %+v", item)
		}
	}
	// The eB consumption stays out of the eA history.
	for _, itemAny := range p.Items {
		if itemAny.(ActivityItem).Type == ActivityConsumption && itemAny.(ActivityItem).Label == "effective" && itemAny.(ActivityItem).Value != nil && *itemAny.(ActivityItem).Value == "paused" {
			t.Fatalf("eA history contains the eB consumption")
		}
	}

	// The work history spans both editions' rows.
	p, err = s.GetHistory(context.Background(), HistoryQuery{WorkID: strPtr(w.ID)})
	if err != nil {
		t.Fatalf("GetHistory(work): %v", err)
	}
	if p.Total != 5 {
		t.Fatalf("work history total = %d, want 5", p.Total)
	}
	sawEB := false
	for _, itemAny := range p.Items {
		item := itemAny.(ActivityItem)
		if item.EditionID != nil && *item.EditionID == eB.ID {
			sawEB = true
		}
	}
	if !sawEB {
		t.Fatalf("work history misses the eB row")
	}

	// An empty history is an empty page, never an error.
	p, err = s.GetHistory(context.Background(), HistoryQuery{EditionID: strPtr(eC.ID)})
	if err != nil {
		t.Fatalf("GetHistory(eC): %v", err)
	}
	if p.Total != 0 || len(p.Items) != 0 {
		t.Fatalf("eC history = %d items, total %d, want empty", len(p.Items), p.Total)
	}

	// History pagination: two rows per page over the work history.
	p1, err := s.GetHistory(context.Background(), HistoryQuery{WorkID: strPtr(w.ID), Limit: 2})
	if err != nil {
		t.Fatalf("GetHistory page 1: %v", err)
	}
	if len(p1.Items) != 2 || p1.NextCursor == nil {
		t.Fatalf("history page 1 = %+v, want 2 items with a cursor", p1)
	}
	p2, err := s.GetHistory(context.Background(), HistoryQuery{WorkID: strPtr(w.ID), Limit: 2, Cursor: p1.NextCursor})
	if err != nil {
		t.Fatalf("GetHistory page 2: %v", err)
	}
	if len(p2.Items) != 2 {
		t.Fatalf("history page 2 = %+v, want 2 items", p2)
	}
	if activityItem(t, p1, 0).ID == activityItem(t, p2, 0).ID {
		t.Fatalf("history pages overlap")
	}
	// A feed cursor does not replay on a history request: the fingerprints
	// differ.
	feed, err := s.ListActivity(context.Background(), ActivityQuery{Limit: 2})
	if err != nil {
		t.Fatalf("ListActivity: %v", err)
	}
	if feed.NextCursor == nil {
		t.Fatalf("feed cursor missing: %+v", feed)
	}
	if _, err := s.GetHistory(context.Background(), HistoryQuery{WorkID: strPtr(w.ID), Cursor: feed.NextCursor}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("feed cursor on history: err = %v, want invalid_argument", err)
	}
}
