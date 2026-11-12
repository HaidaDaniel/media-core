package app

import (
	"context"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

func intPtr(n int) *int { return &n }

func mustSetRatingView(t *testing.T, s *Service, req SetRatingRequest) EditionView {
	t.Helper()
	view, err := s.SetRating(context.Background(), req)
	if err != nil {
		t.Fatalf("SetRating(%s): %v", req.EditionID, err)
	}
	return view
}

// TestRatingSetChangeClear proves the service rating lifecycle on an
// edition: setting renders the rating, a change keeps the original
// recoverable, the explicit clear renders as absence (never zero), and the
// rating never touches consumption.
func TestRatingSetChangeClear(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Rating Story")
	e := addEdition(t, s, w.ID, domain.KindAnime, "Anime R")
	base := testNow()

	ev := mustSetRatingView(t, s, SetRatingRequest{
		MutationBase: mustBase(t, "req-rate-1", e.Revision),
		EditionID:    e.ID,
		Value:        intPtr(8),
		Comment:      strPtr("great"),
		ObservedAt:   &base,
	})
	if ev.Rating == nil || ev.Rating.Value == nil || *ev.Rating.Value != 8 || ev.Rating.Comment == nil || *ev.Rating.Comment != "great" {
		t.Fatalf("rating = %+v, want 8 with the comment", ev.Rating)
	}
	if ev.Consumption != nil {
		t.Fatalf("consumption = %+v, want untouched (a rating never marks status)", ev.Consumption)
	}

	// A change appends a new observation; the original stays recoverable.
	change := base.Add(time.Second)
	ev = mustSetRatingView(t, s, SetRatingRequest{
		MutationBase: mustBase(t, "req-rate-2", ev.Revision),
		EditionID:    e.ID,
		Value:        intPtr(2),
		Comment:      strPtr("disappointed"),
		ObservedAt:   &change,
	})
	if ev.Rating == nil || *ev.Rating.Value != 2 {
		t.Fatalf("rating = %+v, want 2 after the change", ev.Rating)
	}
	var (
		rows   int
		oldVal int
	)
	if err := s.db.DB().QueryRow(`SELECT COUNT(*) FROM rating_observations WHERE edition_id = ?`, e.ID).Scan(&rows); err != nil {
		t.Fatalf("count observations: %v", err)
	}
	if rows != 2 {
		t.Fatalf("observation rows = %d, want 2 (the original is preserved)", rows)
	}
	if err := s.db.DB().QueryRow(
		`SELECT value FROM rating_observations WHERE edition_id = ? ORDER BY recorded_at, id LIMIT 1`, e.ID).Scan(&oldVal); err != nil {
		t.Fatalf("read original: %v", err)
	}
	if oldVal != 8 {
		t.Fatalf("original rating = %d, want 8 recoverable after the change", oldVal)
	}

	// The explicit clear renders as absence, never zero.
	clearAt := base.Add(2 * time.Second)
	ev = mustSetRatingView(t, s, SetRatingRequest{
		MutationBase: mustBase(t, "req-rate-clear", ev.Revision),
		EditionID:    e.ID,
		ObservedAt:   &clearAt,
	})
	if ev.Rating != nil {
		t.Fatalf("rating = %+v, want the explicit clear rendered as absence", ev.Rating)
	}
	var clearedNull bool
	if err := s.db.DB().QueryRow(
		`SELECT value IS NULL FROM rating_observations WHERE edition_id = ? ORDER BY recorded_at DESC, id DESC LIMIT 1`, e.ID).Scan(&clearedNull); err != nil {
		t.Fatalf("read clear: %v", err)
	}
	if !clearedNull {
		t.Fatalf("latest observation is not NULL, want a NULL observation (clear is null, not zero)")
	}

	// A rating after the clear re-establishes the state.
	after := base.Add(3 * time.Second)
	ev = mustSetRatingView(t, s, SetRatingRequest{
		MutationBase: mustBase(t, "req-rate-3", ev.Revision),
		EditionID:    e.ID,
		Value:        intPtr(10),
		ObservedAt:   &after,
	})
	if ev.Rating == nil || *ev.Rating.Value != 10 {
		t.Fatalf("rating = %+v, want 10 after the clear", ev.Rating)
	}
}

// TestRatingReplayAndErrors proves the stable error mapping on the rating
// path: out-of-bounds values are invalid arguments that reserve no key, a
// stale revision conflicts, a missing edition or correction target is a
// not-found, an already-superseded target conflicts, and an exact retry
// replays.
func TestRatingReplayAndErrors(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Rating Errors")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga E")

	for _, v := range []int{0, 11, -1} {
		if _, err := s.SetRating(context.Background(), SetRatingRequest{
			MutationBase: mustBase(t, "req-rate-bad", e.Revision),
			EditionID:    e.ID,
			Value:        intPtr(v),
		}); appError(t, err).Code != ErrInvalidArgument {
			t.Fatalf("value %d: want invalid_argument, got %v", v, err)
		}
	}
	blank := "   "
	if _, err := s.SetRating(context.Background(), SetRatingRequest{
		MutationBase: mustBase(t, "req-rate-blank", e.Revision),
		EditionID:    e.ID,
		Value:        intPtr(5),
		Comment:      &blank,
	}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("blank comment: want invalid_argument, got %v", err)
	}

	// The failed validations reserved no key: the same key can carry a
	// valid rating afterwards.
	ev := mustSetRatingView(t, s, SetRatingRequest{
		MutationBase: mustBase(t, "req-rate-bad", e.Revision),
		EditionID:    e.ID,
		Value:        intPtr(7),
	})
	if ev.Rating == nil || *ev.Rating.Value != 7 {
		t.Fatalf("rating = %+v, want 7 on the reused key", ev.Rating)
	}

	// An exact retry replays without a second observation.
	replayed := mustSetRatingView(t, s, SetRatingRequest{
		MutationBase: mustBase(t, "req-rate-bad", e.Revision),
		EditionID:    e.ID,
		Value:        intPtr(7),
	})
	if replayed.Revision != ev.Revision {
		t.Fatalf("replayed revision = %d, want %d (a retry appends nothing)", replayed.Revision, ev.Revision)
	}
	var rows int
	if err := s.db.DB().QueryRow(`SELECT COUNT(*) FROM rating_observations WHERE edition_id = ?`, e.ID).Scan(&rows); err != nil {
		t.Fatalf("count observations: %v", err)
	}
	if rows != 1 {
		t.Fatalf("observation rows = %d, want 1 after the replay", rows)
	}

	// A different payload on the same key conflicts; a stale revision
	// conflicts; a missing edition and a missing correction target are
	// not-found.
	if _, err := s.SetRating(context.Background(), SetRatingRequest{
		MutationBase: mustBase(t, "req-rate-bad", ev.Revision),
		EditionID:    e.ID,
		Value:        intPtr(3),
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("same key different payload: want conflict, got %v", err)
	}
	if _, err := s.SetRating(context.Background(), SetRatingRequest{
		MutationBase: mustBase(t, "req-rate-stale", 99),
		EditionID:    e.ID,
		Value:        intPtr(3),
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("stale revision: want conflict, got %v", err)
	}
	if _, err := s.SetRating(context.Background(), SetRatingRequest{
		MutationBase: mustBase(t, "req-rate-me", 0),
		EditionID:    "00000000-0000-4000-8000-000000000000",
		Value:        intPtr(3),
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("missing edition: want not_found, got %v", err)
	}
	if _, err := s.SetRating(context.Background(), SetRatingRequest{
		MutationBase: mustBase(t, "req-rate-sup", ev.Revision),
		EditionID:    e.ID,
		Value:        intPtr(9),
		SupersedesID: strPtr("00000000-0000-4000-8000-000000000000"),
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("missing correction target: want not_found, got %v", err)
	}
}

// TestRatingCorrectionAndPastDated proves a supersedes_id correction
// retracts the named observation while it stays in history, a repeat
// correction of the same target conflicts, and a past-dated observation
// never overtakes a newer one.
func TestRatingCorrectionAndPastDated(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Correction Story")
	e := addEdition(t, s, w.ID, domain.KindAnime, "Anime C")

	ev := mustSetRatingView(t, s, SetRatingRequest{
		MutationBase: mustBase(t, "req-rate-c1", e.Revision),
		EditionID:    e.ID,
		Value:        intPtr(9),
	})
	if ev.Rating == nil || ev.Rating.Value == nil || *ev.Rating.Value != 9 {
		t.Fatalf("rating = %+v, want 9", ev.Rating)
	}
	var firstID string
	if err := s.db.DB().QueryRow(
		`SELECT id FROM rating_observations WHERE edition_id = ? ORDER BY recorded_at, id LIMIT 1`, e.ID).Scan(&firstID); err != nil {
		t.Fatalf("read observation id: %v", err)
	}

	// The correction supersedes the mistaken rating.
	ev = mustSetRatingView(t, s, SetRatingRequest{
		MutationBase: mustBase(t, "req-rate-c2", ev.Revision),
		EditionID:    e.ID,
		Value:        intPtr(4),
		SupersedesID: strPtr(firstID),
	})
	if ev.Rating == nil || *ev.Rating.Value != 4 || ev.Rating.SupersedesID == nil || *ev.Rating.SupersedesID != firstID {
		t.Fatalf("rating = %+v, want 4 superseding the first observation", ev.Rating)
	}
	// The corrected row stays in history, superseded.
	var superseding int
	if err := s.db.DB().QueryRow(`SELECT COUNT(*) FROM rating_observations WHERE supersedes_id = ?`, firstID).Scan(&superseding); err != nil {
		t.Fatalf("count superseding: %v", err)
	}
	if superseding != 1 {
		t.Fatalf("superseding rows = %d, want 1 (the correction keeps the target in history)", superseding)
	}

	// Correcting the same target again is a conflict.
	if _, err := s.SetRating(context.Background(), SetRatingRequest{
		MutationBase: mustBase(t, "req-rate-c3", ev.Revision),
		EditionID:    e.ID,
		Value:        intPtr(5),
		SupersedesID: strPtr(firstID),
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("repeat correction: want conflict, got %v", err)
	}

	// A past-dated observation is stored but never overtakes the newer
	// current rating.
	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ev = mustSetRatingView(t, s, SetRatingRequest{
		MutationBase: mustBase(t, "req-rate-c4", ev.Revision),
		EditionID:    e.ID,
		Value:        intPtr(10),
		ObservedAt:   &past,
	})
	if ev.Rating == nil || *ev.Rating.Value != 4 {
		t.Fatalf("rating = %+v, want the newer 4 to stay current over the past-dated 10", ev.Rating)
	}
}

// TestRatingFilterAndSort proves the shared library dimensions through the
// service: the minimum-rating filter matches editions at or above the
// value (a cleared or unrecorded rating never matches) and the rating sort
// orders rated works by their best rating with missing ratings last.
func TestRatingFilterAndSort(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	// Rated 8, rated 5, unrated and cleared editions across four works.
	w8 := mustCreateWork(t, s, "Eight Story")
	e8 := addEdition(t, s, w8.ID, domain.KindAnime, "Anime 8")
	w5 := mustCreateWork(t, s, "Five Story")
	e5 := addEdition(t, s, w5.ID, domain.KindAnime, "Anime 5")
	wU := mustCreateWork(t, s, "Unrated Story")
	addEdition(t, s, wU.ID, domain.KindAnime, "Anime U")
	wC := mustCreateWork(t, s, "Cleared Story")
	eC := addEdition(t, s, wC.ID, domain.KindAnime, "Anime C")

	rateAt := testNow()
	clearAt := rateAt.Add(time.Second)
	mustSetRatingView(t, s, SetRatingRequest{MutationBase: mustBase(t, "req-f-r8", e8.Revision), EditionID: e8.ID, Value: intPtr(8)})
	mustSetRatingView(t, s, SetRatingRequest{MutationBase: mustBase(t, "req-f-r5", e5.Revision), EditionID: e5.ID, Value: intPtr(5)})
	ev := mustSetRatingView(t, s, SetRatingRequest{MutationBase: mustBase(t, "req-f-rc", eC.Revision), EditionID: eC.ID, Value: intPtr(3), ObservedAt: &rateAt})
	mustSetRatingView(t, s, SetRatingRequest{MutationBase: mustBase(t, "req-f-rclear", ev.Revision), EditionID: eC.ID, ObservedAt: &clearAt})

	workIDs := func(t *testing.T, f LibraryFilter) map[string]bool {
		t.Helper()
		cards, err := s.SearchLibrary(ctx, f)
		if err != nil {
			t.Fatalf("SearchLibrary: %v", err)
		}
		out := map[string]bool{}
		for _, c := range cards.Items {
			out[c.(WorkCard).WorkID] = true
		}
		return out
	}

	// Minimum 5 picks the 8 and the 5; the unrated and cleared works never
	// match a minimum rating.
	five := workIDs(t, LibraryFilter{Sort: SortTitle, MinRating: intPtr(5), Limit: 10})
	if len(five) != 2 || !five[w8.ID] || !five[w5.ID] {
		t.Fatalf("minimum 5 filter = %+v, want the eight and five works only", five)
	}
	eight := workIDs(t, LibraryFilter{Sort: SortTitle, MinRating: intPtr(8), Limit: 10})
	if len(eight) != 1 || !eight[w8.ID] {
		t.Fatalf("minimum 8 filter = %+v, want the eight work only", eight)
	}

	// The rating sort orders by the best current rating, missing ratings
	// last; the cleared work is missing, not zero.
	cards, err := s.SearchLibrary(ctx, LibraryFilter{Sort: SortRating, Limit: 10})
	if err != nil {
		t.Fatalf("SearchLibrary(rating): %v", err)
	}
	var order []string
	for _, c := range cards.Items {
		order = append(order, c.(WorkCard).WorkID)
	}
	want := []string{w8.ID, w5.ID}
	if len(order) != 4 || order[0] != want[0] || order[1] != want[1] {
		t.Fatalf("rating sort order = %v, want %v first", order, want)
	}
	// The last two are the unrated and cleared works, in work-ID order.
	rest := map[string]bool{order[2]: true, order[3]: true}
	if !rest[wU.ID] || !rest[wC.ID] {
		t.Fatalf("rating sort tail = %v, want the unrated and cleared works last", order[2:])
	}

	// A minimum-rating filter never invents an average: a work rated 2 on
	// one edition and 8 on another matches minimum 5 through the qualifying
	// edition only.
	wM := mustCreateWork(t, s, "Mixed Story")
	emLow := addEdition(t, s, wM.ID, domain.KindManga, "Manga Low")
	emHigh := addEdition(t, s, wM.ID, domain.KindAnime, "Anime High")
	mustSetRatingView(t, s, SetRatingRequest{MutationBase: mustBase(t, "req-f-m1", emLow.Revision), EditionID: emLow.ID, Value: intPtr(2)})
	mustSetRatingView(t, s, SetRatingRequest{MutationBase: mustBase(t, "req-f-m2", emHigh.Revision), EditionID: emHigh.ID, Value: intPtr(8)})
	mixed := workIDs(t, LibraryFilter{Sort: SortTitle, MinRating: intPtr(5), Limit: 10})
	if !mixed[wM.ID] {
		t.Fatalf("minimum 5 filter misses the mixed work: %+v", mixed)
	}
}
