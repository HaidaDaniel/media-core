package store

import (
	"errors"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

var ratingActor = RatingActor{Principal: "local", Origin: "user"}

func mustSetRating(t *testing.T, db *Database, now time.Time, e Edition, in RatingInput) Edition {
	t.Helper()
	fresh, err := db.SetRating(ctx(t), now, e.ID, e.Revision, in, ratingActor, nil)
	if err != nil {
		t.Fatalf("SetRating(%+v): %v", in, err)
	}
	return fresh
}

func mustInt(n int) *int { return &n }

// TestRatingSetChangeClear proves the rating lifecycle on an edition:
// setting renders the current rating and bumps the edition revision, a
// change appends a new observation while the original stays recoverable,
// the same current value and comment is a no-op without a bump, and the
// explicit clear is a NULL observation rendered as absence.
func TestRatingSetChangeClear(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Rating Story")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga R")
	edRev := e.Revision

	e = mustSetRating(t, db, now.Add(2*time.Second), e, RatingInput{Value: mustInt(8), Comment: strPtr("great")})
	if e.Revision != edRev+1 {
		t.Fatalf("edition revision = %d, want %d (a rating change bumps the edition)", e.Revision, edRev+1)
	}
	if e.Rating == nil || e.Rating.Value == nil || *e.Rating.Value != 8 || e.Rating.Comment == nil || *e.Rating.Comment != "great" {
		t.Fatalf("rating = %+v, want 8 with the comment", e.Rating)
	}
	if e.Consumption != nil {
		t.Fatalf("consumption = %+v, want untouched (a rating never marks status)", e.Consumption)
	}
	first := e.Rating

	// A change appends a new current observation; the original stays
	// recoverable.
	e = mustSetRating(t, db, now.Add(3*time.Second), e, RatingInput{Value: mustInt(2), Comment: strPtr("disappointed")})
	if e.Rating == nil || e.Rating.Value == nil || *e.Rating.Value != 2 || e.Rating.ID == first.ID {
		t.Fatalf("rating = %+v, want the new observation 2", e.Rating)
	}
	var (
		rows   int
		oldVal int
	)
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM rating_observations WHERE edition_id = ?`, e.ID).Scan(&rows); err != nil {
		t.Fatalf("count observations: %v", err)
	}
	if rows != 2 {
		t.Fatalf("observation rows = %d, want 2 (the original is preserved)", rows)
	}
	if err := db.DB().QueryRow(
		`SELECT value FROM rating_observations WHERE id = ?`, first.ID).Scan(&oldVal); err != nil {
		t.Fatalf("read original: %v", err)
	}
	if oldVal != 8 {
		t.Fatalf("original rating = %d, want 8 recoverable after the change", oldVal)
	}

	// The same current value and comment is a no-op without a bump.
	same := e
	e = mustSetRating(t, db, now.Add(4*time.Second), e, RatingInput{Value: mustInt(2), Comment: strPtr("disappointed")})
	if e.Revision != same.Revision {
		t.Fatalf("edition revision = %d, want %d (a no-op does not bump)", e.Revision, same.Revision)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM rating_observations WHERE edition_id = ?`, e.ID).Scan(&rows); err != nil {
		t.Fatalf("count observations: %v", err)
	}
	if rows != 2 {
		t.Fatalf("observation rows = %d, want 2 (a no-op appends nothing)", rows)
	}

	// The explicit clear is a NULL observation; the store keeps the live
	// clear row, the application renders it as absence.
	e = mustSetRating(t, db, now.Add(5*time.Second), e, RatingInput{})
	if e.Revision != same.Revision+1 {
		t.Fatalf("edition revision = %d, want %d (the clear bumps the edition)", e.Revision, same.Revision+1)
	}
	if e.Rating == nil || e.Rating.Value != nil {
		t.Fatalf("rating = %+v, want the live clear observation with a NULL value", e.Rating)
	}
	var clearedNull bool
	if err := db.DB().QueryRow(
		`SELECT value IS NULL FROM rating_observations WHERE edition_id = ? ORDER BY recorded_at DESC, id DESC LIMIT 1`, e.ID).Scan(&clearedNull); err != nil {
		t.Fatalf("read clear: %v", err)
	}
	if !clearedNull {
		t.Fatalf("latest observation is not NULL, want a NULL observation (clear is null, not zero)")
	}

	// Clearing an already-cleared edition is a no-op.
	same = e
	e = mustSetRating(t, db, now.Add(6*time.Second), e, RatingInput{})
	if e.Revision != same.Revision {
		t.Fatalf("edition revision = %d, want %d (re-clearing is a no-op)", e.Revision, same.Revision)
	}

	// A rating after the clear re-establishes the state.
	e = mustSetRating(t, db, now.Add(7*time.Second), e, RatingInput{Value: mustInt(10)})
	if e.Rating == nil || e.Rating.Value == nil || *e.Rating.Value != 10 {
		t.Fatalf("rating = %+v, want 10 after the clear", e.Rating)
	}
}

// TestRatingInvalidAndErrors proves the rating store rejects out-of-bounds
// values, blank comments, stale revisions and missing entities with the
// stable errors and leaves the prior state intact.
func TestRatingInvalidAndErrors(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Rating Errors")
	e, _ := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga E")

	for _, v := range []int{0, 11, -1} {
		if _, err := db.SetRating(ctx(t), now, e.ID, e.Revision, RatingInput{Value: mustInt(v)}, ratingActor, nil); !errors.Is(err, ErrInvalidRating) {
			t.Fatalf("value %d: want ErrInvalidRating, got %v", v, err)
		}
	}
	if _, err := db.SetRating(ctx(t), now, e.ID, e.Revision, RatingInput{Value: mustInt(5), Comment: strPtr("   ")}, ratingActor, nil); !errors.Is(err, ErrInvalidRating) {
		t.Fatalf("blank comment: want ErrInvalidRating, got %v", err)
	}
	if _, err := db.SetRating(ctx(t), now, e.ID, e.Revision+5, RatingInput{Value: mustInt(5)}, ratingActor, nil); !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("stale revision: want ErrRevisionMismatch, got %v", err)
	}
	if _, err := db.SetRating(ctx(t), now, "00000000-0000-4000-8000-000000000000", 0, RatingInput{Value: mustInt(5)}, ratingActor, nil); !errors.Is(err, ErrEditionNotFound) {
		t.Fatalf("missing edition: want ErrEditionNotFound, got %v", err)
	}
	// A correction target that does not exist is not found.
	if _, err := db.SetRating(ctx(t), now, e.ID, e.Revision, RatingInput{Value: mustInt(5), SupersedesID: strPtr("00000000-0000-4000-8000-000000000000")}, ratingActor, nil); !errors.Is(err, ErrRatingNotFound) {
		t.Fatalf("missing correction target: want ErrRatingNotFound, got %v", err)
	}

	// The failed attempts left no state: a valid rating still applies on
	// the original revision.
	e = mustSetRating(t, db, now.Add(2*time.Second), e, RatingInput{Value: mustInt(7)})
	if e.Rating == nil || e.Rating.Value == nil || *e.Rating.Value != 7 {
		t.Fatalf("rating = %+v, want 7 after the failed attempts", e.Rating)
	}
}

// TestRatingSupersedeAndPastDated proves a supersedes_id correction retracts
// the named observation while it stays in history, correcting the same
// target again conflicts, a foreign-edition target is not found, and a
// past-dated observation never overtakes a newer one.
func TestRatingSupersedeAndPastDated(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Correction Story")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga C")
	other, _ := mustEdition(t, db, now.Add(2*time.Second), w, domain.KindManga, "Manga O")

	e = mustSetRating(t, db, now.Add(3*time.Second), e, RatingInput{Value: mustInt(9)})
	first := e.Rating

	// The correction supersedes the mistaken rating.
	e = mustSetRating(t, db, now.Add(4*time.Second), e, RatingInput{Value: mustInt(4), SupersedesID: &first.ID})
	if e.Rating == nil || e.Rating.Value == nil || *e.Rating.Value != 4 || e.Rating.SupersedesID == nil || *e.Rating.SupersedesID != first.ID {
		t.Fatalf("rating = %+v, want 4 superseding the first observation", e.Rating)
	}
	// The corrected row stays in history, superseded: the current
	// selection skips it.
	var currentID string
	if err := db.DB().QueryRow(
		`SELECT id FROM rating_observations WHERE edition_id = ? ORDER BY recorded_at DESC, id DESC LIMIT 1`, e.ID).Scan(&currentID); err != nil {
		t.Fatalf("read latest: %v", err)
	}
	if currentID == first.ID {
		t.Fatalf("current observation is the superseded row, want the correction")
	}

	// Correcting the same target again conflicts; a target from another
	// edition is not found.
	if _, err := db.SetRating(ctx(t), now.Add(5*time.Second), e.ID, e.Revision, RatingInput{Value: mustInt(5), SupersedesID: &first.ID}, ratingActor, nil); !errors.Is(err, ErrSupersedeConflict) {
		t.Fatalf("repeat correction: want ErrSupersedeConflict, got %v", err)
	}
	if _, err := db.SetRating(ctx(t), now.Add(6*time.Second), other.ID, other.Revision, RatingInput{Value: mustInt(5), SupersedesID: &first.ID}, ratingActor, nil); !errors.Is(err, ErrRatingNotFound) {
		t.Fatalf("foreign correction target: want ErrRatingNotFound, got %v", err)
	}

	// A past-dated observation is stored but never overtakes the newer
	// current rating.
	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	e = mustSetRating(t, db, now.Add(7*time.Second), e, RatingInput{Value: mustInt(10), ObservedAt: past})
	if e.Rating == nil || e.Rating.Value == nil || *e.Rating.Value != 4 {
		t.Fatalf("rating = %+v, want the newer 4 to stay current over the past-dated 10", e.Rating)
	}
}

// TestRatingLibraryDimensions proves the shared search dimensions: the
// minimum-rating filter matches editions at or above the value (an absent
// or cleared rating never matches) and the rating sort orders rated works
// by their best rating with missing ratings last, across pages.
func TestRatingLibraryDimensions(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()

	mk := func(title string, at time.Time) (Work, Edition) {
		w := mustWork(t, db, at, title)
		e, w := mustEdition(t, db, at.Add(time.Second), w, domain.KindManga, title)
		return w, e
	}
	w8, e8 := mk("Eight Story", now.Add(10*time.Second))
	w5, e5 := mk("Five Story", now.Add(20*time.Second))
	w3, e3 := mk("Three Story", now.Add(30*time.Second))
	wU, _ := mk("Unrated Story", now.Add(40*time.Second))
	wC, eC := mk("Cleared Story", now.Add(50*time.Second))

	e8 = mustSetRating(t, db, now.Add(60*time.Second), e8, RatingInput{Value: mustInt(8)})
	e5 = mustSetRating(t, db, now.Add(61*time.Second), e5, RatingInput{Value: mustInt(5)})
	e3 = mustSetRating(t, db, now.Add(62*time.Second), e3, RatingInput{Value: mustInt(3)})
	eC = mustSetRating(t, db, now.Add(63*time.Second), eC, RatingInput{Value: mustInt(3)})
	eC = mustSetRating(t, db, now.Add(64*time.Second), eC, RatingInput{})

	workIDs := func(q LibraryQuery) []string {
		cards, total, err := db.SearchLibrary(ctx(t), q)
		if err != nil {
			t.Fatalf("SearchLibrary(%+v): %v", q, err)
		}
		if total != int64(len(cards)) {
			t.Fatalf("total = %d, want %d (the query returns one page)", total, len(cards))
		}
		var ids []string
		for _, c := range cards {
			ids = append(ids, c.Work.ID)
		}
		return ids
	}
	contains := func(ids []string, id string) bool {
		for _, v := range ids {
			if v == id {
				return true
			}
		}
		return false
	}

	// Minimum 5 picks the eight and five works; the three, unrated and
	// cleared works never match a minimum of 5.
	five := workIDs(LibraryQuery{MinRating: mustInt(5), Sort: SortTitle, Limit: MaxLibraryLimit})
	if len(five) != 2 || !contains(five, w8.ID) || !contains(five, w5.ID) {
		t.Fatalf("minimum 5 filter = %v, want the eight and five works only", five)
	}
	three := workIDs(LibraryQuery{MinRating: mustInt(3), Sort: SortTitle, Limit: MaxLibraryLimit})
	if len(three) != 3 || !contains(three, w3.ID) {
		t.Fatalf("minimum 3 filter = %v, want the rated three work too", three)
	}

	// The rating sort orders by the best current rating, missing ratings
	// last, stable across a seek cursor.
	page1, total, err := db.SearchLibrary(ctx(t), LibraryQuery{Sort: SortRating, Limit: 2})
	if err != nil {
		t.Fatalf("SearchLibrary(rating): %v", err)
	}
	if total != 5 || len(page1) != 2 {
		t.Fatalf("rating sort = %d works, %d rows, want 5 works on two pages", total, len(page1))
	}
	if page1[0].Work.ID != w8.ID || page1[1].Work.ID != w5.ID {
		t.Fatalf("rating sort page 1 = [%s, %s], want the eight and five works", page1[0].Work.ID, page1[1].Work.ID)
	}
	page2, _, err := db.SearchLibrary(ctx(t), LibraryQuery{Sort: SortRating, Limit: 2, AfterKey: page1[1].SortKey, AfterID: page1[1].Work.ID})
	if err != nil {
		t.Fatalf("SearchLibrary(rating page 2): %v", err)
	}
	if len(page2) != 2 || page2[0].Work.ID != w3.ID {
		t.Fatalf("rating sort page 2 = %v, want the three work first", page2)
	}
	page3, _, err := db.SearchLibrary(ctx(t), LibraryQuery{Sort: SortRating, Limit: 2, AfterKey: page2[1].SortKey, AfterID: page2[1].Work.ID})
	if err != nil {
		t.Fatalf("SearchLibrary(rating page 3): %v", err)
	}
	rest := map[string]bool{page2[1].Work.ID: true}
	for _, c := range page3 {
		rest[c.Work.ID] = true
	}
	// The remaining rows are the unrated and the cleared work; the cleared
	// work is missing, not zero, and ties on the work ID.
	if len(page2)+len(page3) != 3 || !rest[wU.ID] || !rest[wC.ID] {
		t.Fatalf("rating sort tail = %v, want the unrated and cleared works last", rest)
	}

	// The work card keeps the matched-edition identity and never an
	// invented rating average: the card summary carries no rating field.
	if len(page1[0].MatchedEditionIDs) == 0 {
		t.Fatalf("card %s has no matched editions", page1[0].Work.ID)
	}
}
