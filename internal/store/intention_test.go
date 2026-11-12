package store

import (
	"errors"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

func intentionActor() IntentionActor {
	return IntentionActor{Principal: "local", Origin: "user"}
}

func mustSetIntention(t *testing.T, db *Database, now time.Time, e Edition, expectedRevision int64, in IntentionInput) Edition {
	t.Helper()
	fresh, err := db.SetIntention(ctx(t), now, e.ID, expectedRevision, in, intentionActor(), nil)
	if err != nil {
		t.Fatalf("SetIntention(%s): %v", in.Value, err)
	}
	return fresh
}

func countIntentionRows(t *testing.T, db *Database, editionID string) int {
	t.Helper()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM intention_observations WHERE edition_id = ?`, editionID).Scan(&n); err != nil {
		t.Fatalf("count intention rows: %v", err)
	}
	return n
}

// TestIntentionRecordsHistoryAndCurrent proves transitions append history
// rows, bump the edition revision and update the current intention,
// preserving every prior observation.
func TestIntentionRecordsHistoryAndCurrent(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Intention Story")
	e, _ := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga I")
	if e.Intention != nil {
		t.Fatalf("fresh edition intention = %+v, want absent", e.Intention)
	}

	e = mustSetIntention(t, db, now.Add(2*time.Second), e, e.Revision, IntentionInput{Value: string(domain.IntentionSaved)})
	if e.Revision != 1 {
		t.Fatalf("edition revision = %d, want 1 after the first observation", e.Revision)
	}
	if e.Intention == nil || e.Intention.Value != string(domain.IntentionSaved) {
		t.Fatalf("current intention = %+v, want saved", e.Intention)
	}
	if got := countIntentionRows(t, db, e.ID); got != 1 {
		t.Fatalf("intention rows = %d, want 1", got)
	}

	e = mustSetIntention(t, db, now.Add(3*time.Second), e, e.Revision, IntentionInput{Value: string(domain.IntentionPlanned)})
	if e.Revision != 2 || e.Intention == nil || e.Intention.Value != string(domain.IntentionPlanned) {
		t.Fatalf("edition = %+v, want planned at revision 2", e)
	}
	if got := countIntentionRows(t, db, e.ID); got != 2 {
		t.Fatalf("intention rows = %d, want 2 (history is preserved)", got)
	}
}

// TestIntentionSameValueIsNoOp proves a request for the same current value
// appends nothing and does not bump the revision.
func TestIntentionSameValueIsNoOp(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Noop Intention")
	e, _ := mustEdition(t, db, now.Add(time.Second), w, domain.KindAnime, "Anime N")

	e = mustSetIntention(t, db, now.Add(2*time.Second), e, e.Revision, IntentionInput{Value: string(domain.IntentionInterested)})
	e = mustSetIntention(t, db, now.Add(3*time.Second), e, e.Revision, IntentionInput{Value: string(domain.IntentionInterested)})
	if e.Revision != 1 {
		t.Fatalf("edition revision = %d, want 1 after the same-value no-op", e.Revision)
	}
	if got := countIntentionRows(t, db, e.ID); got != 1 {
		t.Fatalf("intention rows = %d, want 1 (a no-op appends nothing)", got)
	}
}

// TestIntentionNoneRetraction proves the explicit none value retracts to no
// intention while staying in history, and a later value supersedes the
// retraction like any other transition.
func TestIntentionNoneRetraction(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Retraction Story")
	e, _ := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga R")

	e = mustSetIntention(t, db, now.Add(2*time.Second), e, e.Revision, IntentionInput{Value: string(domain.IntentionSaved)})
	e = mustSetIntention(t, db, now.Add(3*time.Second), e, e.Revision, IntentionInput{Value: IntentionNone, Reason: strPtr("changed my mind")})
	if e.Revision != 2 {
		t.Fatalf("edition revision = %d, want 2 after the retraction", e.Revision)
	}
	if e.Intention == nil || e.Intention.Value != IntentionNone || e.Intention.Reason == nil || *e.Intention.Reason != "changed my mind" {
		t.Fatalf("current intention = %+v, want none with the reason", e.Intention)
	}
	if got := countIntentionRows(t, db, e.ID); got != 2 {
		t.Fatalf("intention rows = %d, want 2 (the retraction stays in history)", got)
	}

	e = mustSetIntention(t, db, now.Add(4*time.Second), e, e.Revision, IntentionInput{Value: string(domain.IntentionNotInterested)})
	if e.Intention == nil || e.Intention.Value != string(domain.IntentionNotInterested) {
		t.Fatalf("current intention = %+v, want not_interested after the retraction", e.Intention)
	}
}

// TestIntentionSupersedeCorrection proves a supersedes_id correction
// targets an existing, not-yet-superseded observation of the same edition;
// a missing or foreign target is a distinct not-found and an
// already-superseded target a conflict.
func TestIntentionSupersedeCorrection(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Correction Story")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga C")
	e2, _ := mustEdition(t, db, now.Add(2*time.Second), w, domain.KindAnime, "Anime C")

	e = mustSetIntention(t, db, now.Add(3*time.Second), e, e.Revision, IntentionInput{Value: string(domain.IntentionInterested)})
	first := e.Intention.ID
	e = mustSetIntention(t, db, now.Add(4*time.Second), e, e.Revision, IntentionInput{Value: string(domain.IntentionSaved)})

	// A correction of the first observation while a newer one is current.
	e = mustSetIntention(t, db, now.Add(5*time.Second), e, e.Revision, IntentionInput{Value: string(domain.IntentionPlanned), SupersedesID: strPtr(first)})
	if e.Intention == nil || e.Intention.Value != string(domain.IntentionPlanned) || e.Intention.SupersedesID == nil || *e.Intention.SupersedesID != first {
		t.Fatalf("current intention = %+v, want planned superseding the first row", e.Intention)
	}

	if _, err := db.SetIntention(ctx(t), now.Add(6*time.Second), e.ID, e.Revision, IntentionInput{Value: string(domain.IntentionPlanned), SupersedesID: strPtr("00000000-0000-4000-8000-000000000000")}, intentionActor(), nil); !errors.Is(err, ErrIntentionNotFound) {
		t.Fatalf("missing supersede target: err = %v, want ErrIntentionNotFound", err)
	}
	if _, err := db.SetIntention(ctx(t), now.Add(7*time.Second), e2.ID, e2.Revision, IntentionInput{Value: string(domain.IntentionPlanned), SupersedesID: strPtr(first)}, intentionActor(), nil); !errors.Is(err, ErrIntentionNotFound) {
		t.Fatalf("foreign supersede target: err = %v, want ErrIntentionNotFound", err)
	}
	if _, err := db.SetIntention(ctx(t), now.Add(8*time.Second), e.ID, e.Revision, IntentionInput{Value: string(domain.IntentionSaved), SupersedesID: strPtr(first)}, intentionActor(), nil); !errors.Is(err, ErrSupersedeConflict) {
		t.Fatalf("already-superseded target: err = %v, want ErrSupersedeConflict", err)
	}
}

// TestIntentionPastDatedStaysHistorical proves a past-dated observation is
// stored but does not become current over a newer one.
func TestIntentionPastDatedStaysHistorical(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Past Story")
	e, _ := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga P")

	e = mustSetIntention(t, db, now.Add(3*time.Second), e, e.Revision, IntentionInput{Value: string(domain.IntentionSaved)})
	e = mustSetIntention(t, db, now.Add(4*time.Second), e, e.Revision, IntentionInput{Value: string(domain.IntentionPlanned), ObservedAt: now.Add(time.Second)})
	if e.Intention == nil || e.Intention.Value != string(domain.IntentionSaved) {
		t.Fatalf("current intention = %+v, want saved (the newer recorded row wins)", e.Intention)
	}
	if got := countIntentionRows(t, db, e.ID); got != 2 {
		t.Fatalf("intention rows = %d, want 2 (the past-dated row stays in history)", got)
	}
}

// TestIntentionInvalidAndStale proves invalid values, a stale revision and
// a missing edition fail distinctly without partial writes.
func TestIntentionInvalidAndStale(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Invalid Story")
	e, _ := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga V")

	if _, err := db.SetIntention(ctx(t), now, e.ID, e.Revision, IntentionInput{Value: "maybe"}, intentionActor(), nil); !errors.Is(err, ErrInvalidIntention) {
		t.Fatalf("invalid value: err = %v, want ErrInvalidIntention", err)
	}
	if _, err := db.SetIntention(ctx(t), now.Add(time.Second), e.ID, 99, IntentionInput{Value: string(domain.IntentionSaved)}, intentionActor(), nil); !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("stale revision: err = %v, want ErrRevisionMismatch", err)
	}
	if _, err := db.SetIntention(ctx(t), now.Add(2*time.Second), "00000000-0000-4000-8000-000000000000", 0, IntentionInput{Value: string(domain.IntentionSaved)}, intentionActor(), nil); !errors.Is(err, ErrEditionNotFound) {
		t.Fatalf("missing edition: err = %v, want ErrEditionNotFound", err)
	}
	if got := countIntentionRows(t, db, e.ID); got != 0 {
		t.Fatalf("intention rows = %d, want 0 after failures", got)
	}
}
