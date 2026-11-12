package store

import (
	"errors"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

func consumptionActor() ConsumptionActor {
	return ConsumptionActor{Principal: "local", Origin: "user"}
}

func mustSetConsumption(t *testing.T, db *Database, now time.Time, e Edition, expectedRevision int64, in ConsumptionInput) Edition {
	t.Helper()
	fresh, err := db.SetConsumption(ctx(t), now, e.ID, expectedRevision, in, consumptionActor(), nil)
	if err != nil {
		t.Fatalf("SetConsumption(%q): %v", in.Value, err)
	}
	return fresh
}

func countConsumptionRows(t *testing.T, db *Database, editionID string) int {
	t.Helper()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM consumption_observations WHERE edition_id = ?`, editionID).Scan(&n); err != nil {
		t.Fatalf("count consumption rows: %v", err)
	}
	return n
}

// TestConsumptionSetAndUpdate proves transitions append history rows, bump
// the edition revision and update the current view, preserving every prior
// observation.
func TestConsumptionSetAndUpdate(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story A")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga Title")
	if e.Consumption != nil {
		t.Fatalf("fresh store-level edition consumption = %+v, want nil (no observation)", e.Consumption)
	}

	reason := "picked it up"
	e = mustSetConsumption(t, db, now.Add(2*time.Second), e, e.Revision, ConsumptionInput{Value: "in_progress", Reason: &reason})
	if e.Revision != 1 {
		t.Fatalf("edition revision = %d, want 1 after the first observation", e.Revision)
	}
	if e.Consumption == nil || e.Consumption.Value != "in_progress" {
		t.Fatalf("consumption = %+v, want in_progress", e.Consumption)
	}
	if e.Consumption.Reason == nil || *e.Consumption.Reason != "picked it up" {
		t.Fatalf("consumption reason = %+v, want the supplied reason", e.Consumption.Reason)
	}
	if e.Consumption.Actor.Principal != "local" || e.Consumption.Actor.Origin != "user" {
		t.Fatalf("consumption actor = %+v, want principal local origin user", e.Consumption.Actor)
	}

	e = mustSetConsumption(t, db, now.Add(3*time.Second), e, e.Revision, ConsumptionInput{Value: "completed"})
	if e.Revision != 2 {
		t.Fatalf("edition revision = %d, want 2 after the transition", e.Revision)
	}
	if e.Consumption == nil || e.Consumption.Value != "completed" {
		t.Fatalf("consumption = %+v, want completed", e.Consumption)
	}
	if got := countConsumptionRows(t, db, e.ID); got != 2 {
		t.Fatalf("consumption rows = %d, want 2 (history is preserved)", got)
	}
}

// TestConsumptionSameValueNoOp proves setting the same current value is a
// no-op: no new row, no revision bump, but the journal still settles so an
// exact retry replays.
func TestConsumptionSameValueNoOp(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story B")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindAnime, "Anime Title")

	e = mustSetConsumption(t, db, now.Add(2*time.Second), e, e.Revision, ConsumptionInput{Value: "in_progress"})
	req := &JournalInput{RequestID: "req-same-value", Operation: "set_consumption", RequestHash: "h", Actor: "local"}
	e, err := db.SetConsumption(ctx(t), now.Add(3*time.Second), e.ID, e.Revision, ConsumptionInput{Value: "in_progress"}, consumptionActor(), req)
	if err != nil {
		t.Fatalf("SetConsumption(same value): %v", err)
	}
	if e.Revision != 1 {
		t.Fatalf("edition revision = %d, want 1 (same-value no-op must not bump)", e.Revision)
	}
	if got := countConsumptionRows(t, db, e.ID); got != 1 {
		t.Fatalf("consumption rows = %d, want 1 (no duplicate observation)", got)
	}
	var journal int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM mutation_journal WHERE request_id = ?`, "req-same-value").Scan(&journal); err != nil {
		t.Fatalf("count journal: %v", err)
	}
	if journal != 1 {
		t.Fatalf("journal rows = %d, want 1 (the no-op settles the request key)", journal)
	}
}

// TestConsumptionUnknownAndRetraction proves an edition without
// observations is unknown, a transition out of unknown is recorded, and an
// explicit retraction back to unknown renders as the absence of a state
// while the history rows stay.
func TestConsumptionUnknownAndRetraction(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story C")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga C")
	if e.Consumption != nil {
		t.Fatalf("unknown historical edition consumption = %+v, want nil", e.Consumption)
	}

	e = mustSetConsumption(t, db, now.Add(2*time.Second), e, e.Revision, ConsumptionInput{Value: "dropped"})
	if e.Consumption == nil || e.Consumption.Value != "dropped" {
		t.Fatalf("consumption = %+v, want dropped", e.Consumption)
	}

	e = mustSetConsumption(t, db, now.Add(3*time.Second), e, e.Revision, ConsumptionInput{Value: ConsumptionUnknown, Reason: strPtr("misread")})
	if e.Consumption == nil || e.Consumption.Value != ConsumptionUnknown {
		t.Fatalf("retracted edition consumption = %+v, want the explicit unknown row (absence rendering is the app concern)", e.Consumption)
	}
	if got := countConsumptionRows(t, db, e.ID); got != 2 {
		t.Fatalf("consumption rows = %d, want 2 (the retraction stays in history)", got)
	}
	// Setting unknown again on an unknown state is a no-op.
	e, err := db.SetConsumption(ctx(t), now.Add(4*time.Second), e.ID, e.Revision, ConsumptionInput{Value: ConsumptionUnknown}, consumptionActor(), nil)
	if err != nil {
		t.Fatalf("SetConsumption(unknown on unknown): %v", err)
	}
	if e.Revision != 2 {
		t.Fatalf("edition revision = %d, want 2 (unknown on unknown is a no-op)", e.Revision)
	}
}

// TestConsumptionStaleRevision proves a lost-update guard: a wrong expected
// revision is rejected and the prior state stays intact.
func TestConsumptionStaleRevision(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story D")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga D")

	e = mustSetConsumption(t, db, now.Add(2*time.Second), e, e.Revision, ConsumptionInput{Value: "in_progress"})
	_, err := db.SetConsumption(ctx(t), now.Add(3*time.Second), e.ID, 0, ConsumptionInput{Value: "completed"}, consumptionActor(), nil)
	if !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("stale revision: err = %v, want ErrRevisionMismatch", err)
	}
	fresh, err := db.GetEdition(ctx(t), e.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if fresh.Consumption == nil || fresh.Consumption.Value != "in_progress" || fresh.Revision != 1 {
		t.Fatalf("state after stale attempt = %+v rev %d, want in_progress rev 1 intact", fresh.Consumption, fresh.Revision)
	}
}

// TestConsumptionSupersedeCorrection proves a correction supersedes an
// existing row (kept as history), a row is superseded at most once, and a
// missing or foreign target is a distinct not-found.
func TestConsumptionSupersedeCorrection(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story E")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga E")
	other, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindAnime, "Anime E")

	first := mustSetConsumption(t, db, now.Add(2*time.Second), e, e.Revision, ConsumptionInput{Value: "in_progress"})
	firstID := first.Consumption.ID
	e = mustSetConsumption(t, db, now.Add(3*time.Second), first, first.Revision, ConsumptionInput{Value: "completed"})

	corrected := mustSetConsumption(t, db, now.Add(4*time.Second), e, e.Revision, ConsumptionInput{Value: "paused", SupersedesID: &firstID})
	if corrected.Consumption == nil || corrected.Consumption.Value != "paused" {
		t.Fatalf("corrected consumption = %+v, want paused", corrected.Consumption)
	}
	if corrected.Consumption.SupersedesID == nil || *corrected.Consumption.SupersedesID != firstID {
		t.Fatalf("corrected supersedes = %+v, want the first row", corrected.Consumption.SupersedesID)
	}
	if got := countConsumptionRows(t, db, e.ID); got != 3 {
		t.Fatalf("consumption rows = %d, want 3 (the corrected row stays in history)", got)
	}

	_, err := db.SetConsumption(ctx(t), now.Add(5*time.Second), e.ID, corrected.Revision, ConsumptionInput{Value: "dropped", SupersedesID: &firstID}, consumptionActor(), nil)
	if !errors.Is(err, ErrSupersedeConflict) {
		t.Fatalf("second supersede of the same row: err = %v, want ErrSupersedeConflict", err)
	}
	missing := "00000000-0000-4000-8000-000000000000"
	_, err = db.SetConsumption(ctx(t), now.Add(6*time.Second), e.ID, corrected.Revision, ConsumptionInput{Value: "dropped", SupersedesID: &missing}, consumptionActor(), nil)
	if !errors.Is(err, ErrConsumptionNotFound) {
		t.Fatalf("unknown supersede target: err = %v, want ErrConsumptionNotFound", err)
	}
	_, err = db.SetConsumption(ctx(t), now.Add(7*time.Second), other.ID, other.Revision, ConsumptionInput{Value: "paused", SupersedesID: &firstID}, consumptionActor(), nil)
	if !errors.Is(err, ErrConsumptionNotFound) {
		t.Fatalf("foreign supersede target: err = %v, want ErrConsumptionNotFound", err)
	}
}

// TestConsumptionPastDatedUpdate proves a past-dated observation is stored
// but never becomes current over a newer one.
func TestConsumptionPastDatedUpdate(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story F")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga F")

	e = mustSetConsumption(t, db, now.Add(4*time.Second), e, e.Revision, ConsumptionInput{Value: "completed", ObservedAt: now.Add(4 * time.Second)})
	e = mustSetConsumption(t, db, now.Add(5*time.Second), e, e.Revision, ConsumptionInput{Value: "in_progress", ObservedAt: now.Add(2 * time.Second)})
	if e.Consumption == nil || e.Consumption.Value != "completed" {
		t.Fatalf("consumption = %+v, want completed (the past-dated row must not overtake)", e.Consumption)
	}
	if got := countConsumptionRows(t, db, e.ID); got != 2 {
		t.Fatalf("consumption rows = %d, want 2 (the past-dated row is stored)", got)
	}
	e = mustSetConsumption(t, db, now.Add(6*time.Second), e, e.Revision, ConsumptionInput{Value: "in_progress"})
	if e.Consumption == nil || e.Consumption.Value != "in_progress" {
		t.Fatalf("consumption = %+v, want in_progress after the newer row", e.Consumption)
	}
}

// TestConsumptionInvalidValue proves values outside the five states plus
// the explicit retraction value are rejected without touching the state.
func TestConsumptionInvalidValue(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story G")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga G")

	for _, v := range []string{"", "planned", "on_hold", "watching", "UNKNOWN"} {
		_, err := db.SetConsumption(ctx(t), now.Add(2*time.Second), e.ID, e.Revision, ConsumptionInput{Value: v}, consumptionActor(), nil)
		if !errors.Is(err, ErrInvalidConsumption) {
			t.Fatalf("SetConsumption(%q): err = %v, want ErrInvalidConsumption", v, err)
		}
	}
	if e.Revision != 0 || countConsumptionRows(t, db, e.ID) != 0 {
		t.Fatalf("state after invalid values: revision %d rows %d, want untouched", e.Revision, countConsumptionRows(t, db, e.ID))
	}
}

// TestConsumptionFilter proves the library consumption dimension matches the
// current effective value of the same edition, OR within supplied values,
// including unknown, and correlates with the query.
func TestConsumptionFilter(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()

	wA := mustWork(t, db, now, "Alpha Saga")
	eA, _ := mustEdition(t, db, now.Add(time.Second), wA, domain.KindManga, "Alpha Manga")
	eA = mustSetConsumption(t, db, now.Add(2*time.Second), eA, eA.Revision, ConsumptionInput{Value: "completed"})

	wB := mustWork(t, db, now.Add(3*time.Second), "Beta Saga")
	eB, _ := mustEdition(t, db, now.Add(4*time.Second), wB, domain.KindManga, "Beta Manga")

	wC := mustWork(t, db, now.Add(5*time.Second), "Gamma Saga")
	eC, _ := mustEdition(t, db, now.Add(6*time.Second), wC, domain.KindManga, "Gamma Manga")
	eC = mustSetConsumption(t, db, now.Add(7*time.Second), eC, eC.Revision, ConsumptionInput{Value: "in_progress"})

	// An edition retracted to unknown counts as unknown again.
	wD := mustWork(t, db, now.Add(8*time.Second), "Delta Saga")
	eD, _ := mustEdition(t, db, now.Add(9*time.Second), wD, domain.KindManga, "Delta Manga")
	eD = mustSetConsumption(t, db, now.Add(10*time.Second), eD, eD.Revision, ConsumptionInput{Value: "dropped"})
	eD = mustSetConsumption(t, db, now.Add(11*time.Second), eD, eD.Revision, ConsumptionInput{Value: ConsumptionUnknown})

	ids := func(q LibraryQuery) map[string]bool {
		if q.Sort == "" {
			q.Sort = SortTitle
		}
		cards, total, err := db.SearchLibrary(ctx(t), q)
		if err != nil {
			t.Fatalf("SearchLibrary: %v", err)
		}
		if int(total) != len(cards) {
			t.Fatalf("total = %d, want %d", total, len(cards))
		}
		got := map[string]bool{}
		for _, c := range cards {
			for _, id := range c.MatchedEditionIDs {
				got[id] = true
			}
		}
		return got
	}
	want := func(t *testing.T, got map[string]bool, editionIDs ...string) {
		t.Helper()
		if len(got) != len(editionIDs) {
			t.Fatalf("matched editions = %v, want exactly %v", got, editionIDs)
		}
		for _, id := range editionIDs {
			if !got[id] {
				t.Fatalf("matched editions = %v, want to include %s", got, id)
			}
		}
	}

	want(t, ids(LibraryQuery{Consumptions: []domain.Consumption{domain.ConsumptionCompleted}}), eA.ID)
	want(t, ids(LibraryQuery{UnknownConsumption: true}), eB.ID, eD.ID)
	want(t, ids(LibraryQuery{Consumptions: []domain.Consumption{domain.ConsumptionCompleted, domain.ConsumptionInProgress}}), eA.ID, eC.ID)
	want(t, ids(LibraryQuery{Consumptions: []domain.Consumption{domain.ConsumptionCompleted}, UnknownConsumption: true}), eA.ID, eB.ID, eD.ID)
	want(t, ids(LibraryQuery{}), eA.ID, eB.ID, eC.ID, eD.ID)

	// Query correlation: "alpha" matches work A by title, but the
	// completed dimension must hold on a matching edition of the same work.
	want(t, ids(LibraryQuery{Query: "alpha", Consumptions: []domain.Consumption{domain.ConsumptionCompleted}}), eA.ID)
	want(t, ids(LibraryQuery{Query: "alpha", Consumptions: []domain.Consumption{domain.ConsumptionInProgress}}))
}
