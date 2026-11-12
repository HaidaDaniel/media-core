package app

import (
	"context"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func mustRecordProgressView(t *testing.T, s *Service, req RecordProgressRequest) EditionView {
	t.Helper()
	view, err := s.RecordProgress(context.Background(), req)
	if err != nil {
		t.Fatalf("RecordProgress(%s %s): %v", req.Unit, req.Position, err)
	}
	return view
}

// TestProgressRecordReplayAndConflict proves the service record path: the
// view carries the new position, an exact retry replays without a second
// row, and a different payload on the same key conflicts.
func TestProgressRecordReplayAndConflict(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Replay Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga R")

	view := mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-progress-1", e.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "42",
	})
	if view.Revision != e.Revision+1 {
		t.Fatalf("view revision = %d, want %d", view.Revision, e.Revision+1)
	}
	if view.Progress.Chapter == nil || view.Progress.Chapter.Position != "42" {
		t.Fatalf("view progress = %+v, want chapter 42", view.Progress)
	}
	if view.Progress.Chapter.Actor.Principal != "local" || view.Progress.Chapter.Actor.Origin != domain.OriginUser {
		t.Fatalf("view actor = %+v, want local user", view.Progress.Chapter.Actor)
	}

	view = mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-progress-1", e.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "42",
	})

	view = mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-progress-2", view.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "48",
	})
	if view.Progress.Chapter == nil || view.Progress.Chapter.Position != "48" {
		t.Fatalf("view progress = %+v, want chapter 48", view.Progress)
	}

	req := RecordProgressRequest{
		MutationBase: mustBase(t, "req-progress-1", view.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "48",
	}
	if _, err := s.RecordProgress(context.Background(), req); appError(t, err).Code != ErrConflict {
		t.Fatalf("same key different payload: want conflict, got %v", err)
	}
}

// TestProgressStaleInvalidAndUnknown proves the stable error mapping: a
// wrong revision is a conflict, a missing edition a not-found, an unknown
// supersede target a not-found, and an invalid unit, position, page
// fraction, percent above 100 or above-total value an invalid argument.
func TestProgressStaleInvalidAndUnknown(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Stale Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga S")

	if _, err := s.RecordProgress(context.Background(), RecordProgressRequest{
		MutationBase: mustBase(t, "req-p-stale", 99),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "1",
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("stale revision: want conflict, got %v", err)
	}
	if _, err := s.RecordProgress(context.Background(), RecordProgressRequest{
		MutationBase: mustBase(t, "req-p-missing", 0),
		EditionID:    "00000000-0000-4000-8000-000000000000",
		Unit:         domain.UnitChapter,
		Position:     "1",
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("unknown edition: want not_found, got %v", err)
	}
	if _, err := s.RetractProgress(context.Background(), RetractProgressRequest{
		MutationBase: mustBase(t, "req-p-retract-missing", 0),
		EditionID:    "00000000-0000-4000-8000-000000000000",
		RecordID:     "00000000-0000-4000-8000-000000000000",
		Reason:       "gone",
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("unknown retraction edition: want not_found, got %v", err)
	}
	cases := []struct {
		name  string
		unit  domain.ProgressUnit
		pos   string
		total *string
	}{
		{"unknown unit", "scene", "1", nil},
		{"negative position", domain.UnitChapter, "-1", nil},
		{"fractional page", domain.UnitPage, "3.5", nil},
		{"percent above 100", domain.UnitPercent, "101", nil},
		{"percent explicit total", domain.UnitPercent, "50", appStr("100")},
		{"above known total", domain.UnitChapter, "42", appStr("41")},
	}
	for _, c := range cases {
		_, err := s.RecordProgress(context.Background(), RecordProgressRequest{
			MutationBase: mustBase(t, "req-p-invalid-"+c.name, e.Revision),
			EditionID:    e.ID,
			Unit:         c.unit,
			Position:     c.pos,
			Total:        c.total,
		})
		if appError(t, err).Code != ErrInvalidArgument {
			t.Fatalf("%s: want invalid_argument, got %v", c.name, err)
		}
	}
	if _, err := s.RecordProgress(context.Background(), RecordProgressRequest{
		MutationBase: mustBase(t, "req-p-supersede-missing", e.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "1",
		SupersedesID: appStr("00000000-0000-4000-8000-000000000000"),
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("unknown supersede target: want not_found, got %v", err)
	}
	if _, err := s.RetractProgress(context.Background(), RetractProgressRequest{
		MutationBase: mustBase(t, "req-p-retract-noreason", e.Revision),
		EditionID:    e.ID,
		RecordID:     "00000000-0000-4000-8000-000000000000",
		Reason:       "",
	}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("retraction without reason: want invalid_argument, got %v", err)
	}
	fresh, err := s.GetEdition(context.Background(), e.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if fresh.Revision != e.Revision {
		t.Fatalf("revision after invalid attempts = %d, want %d unchanged", fresh.Revision, e.Revision)
	}
}

// TestProgressRetractAndView proves a retraction renders the unit as
// absent, a same-value no-op does not bump the revision, and a lower reread
// is accepted.
func TestProgressRetractAndView(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Retract Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga Re")

	view := mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-pr-1", e.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "48",
	})
	rowID := view.Progress.Chapter.ID
	// The same current value is a no-op without a revision bump.
	view = mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-pr-2", view.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "48",
	})
	// Retract the recorded row: the unit becomes absent.
	retracted, err := s.RetractProgress(context.Background(), RetractProgressRequest{
		MutationBase: mustBase(t, "req-pr-3", view.Revision),
		EditionID:    e.ID,
		RecordID:     rowID,
		Reason:       "never finished",
	})
	if err != nil {
		t.Fatalf("RetractProgress: %v", err)
	}
	if retracted.Progress.Chapter != nil {
		t.Fatalf("retracted view progress = %+v, want absent", retracted.Progress.Chapter)
	}
	// A lower reread position is a valid new observation.
	view = mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-pr-4", retracted.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "10",
		Reason:       appStr("rereading"),
	})
	if view.Progress.Chapter == nil || view.Progress.Chapter.Position != "10" {
		t.Fatalf("reread progress = %+v, want chapter 10", view.Progress.Chapter)
	}
}

// TestProgressSupersedeCorrectionView proves a correction targets a row of
// the same unit, keeps it in history, and a second correction of the same
// row is a conflict while the first stays current.
func TestProgressSupersedeCorrectionView(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Correction Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga Co")

	view := mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-pc-1", e.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "42",
	})
	firstID := view.Progress.Chapter.ID
	view = mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-pc-2", view.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "48",
	})
	view = mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-pc-3", view.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "42.5",
		SupersedesID: &firstID,
	})
	if view.Progress.Chapter == nil || view.Progress.Chapter.Position != "42.5" {
		t.Fatalf("corrected progress = %+v, want chapter 42.5", view.Progress.Chapter)
	}
	if view.Progress.Chapter.SupersedesID == nil || *view.Progress.Chapter.SupersedesID != firstID {
		t.Fatalf("corrected supersedes = %+v, want the first row", view.Progress.Chapter.SupersedesID)
	}
	if _, err := s.RecordProgress(context.Background(), RecordProgressRequest{
		MutationBase: mustBase(t, "req-pc-4", view.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "43",
		SupersedesID: &firstID,
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("second correction of the same row: want conflict, got %v", err)
	}
}

// TestProgressConcurrentSameRowCorrection proves two concurrent corrections
// of the same row: one succeeds, the other conflicts, and the history keeps
// exactly one correction of the row.
func TestProgressConcurrentSameRowCorrection(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Concurrent Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga Cc")

	view := mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-pcc-1", e.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "42",
	})
	firstID := view.Progress.Chapter.ID
	baseRev := view.Revision

	errCh := make(chan error, 2)
	for _, pos := range []string{"41", "43"} {
		go func(pos string) {
			_, err := s.RecordProgress(context.Background(), RecordProgressRequest{
				MutationBase: mustBase(t, "req-pcc-"+pos, baseRev),
				EditionID:    e.ID,
				Unit:         domain.UnitChapter,
				Position:     pos,
				SupersedesID: &firstID,
			})
			errCh <- err
		}(pos)
	}
	var conflicts, others int
	var lastErr error
	for range 2 {
		lastErr = <-errCh
		if lastErr == nil {
			others++
			continue
		}
		if appError(t, lastErr).Code == ErrConflict {
			conflicts++
			continue
		}
		t.Fatalf("unexpected error: %v", lastErr)
	}
	if others != 1 || conflicts != 1 {
		t.Fatalf("concurrent corrections: %d ok, %d conflict (last %v), want one of each", others, conflicts, lastErr)
	}
	fresh, err := s.GetEdition(context.Background(), e.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if fresh.Progress.Chapter == nil {
		t.Fatalf("progress after concurrent corrections = %+v, want a current chapter", fresh.Progress)
	}
}

// TestProgressUnknownTotalNoCarryForward proves a total is never carried
// forward into a newer observation.
func TestProgressUnknownTotalNoCarryForward(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Total Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga T")

	view := mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-pt-1", e.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "42",
		Total:        appStr("120"),
	})
	if view.Progress.Chapter.Total == nil || *view.Progress.Chapter.Total != "120" {
		t.Fatalf("total = %+v, want 120", view.Progress.Chapter.Total)
	}
	view = mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-pt-2", view.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "48",
	})
	if view.Progress.Chapter.Total != nil {
		t.Fatalf("total after a no-total row = %v, want unknown (no carry-forward)", *view.Progress.Chapter.Total)
	}
}

// TestProgressIsolatesUnits proves units are independent and never summed,
// and a past-dated observation is stored but does not overtake a newer one.
func TestProgressIsolatesUnits(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Units Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga U")

	view := mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-pu-1", e.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "42",
	})
	view = mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-pu-2", view.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitVolume,
		Position:     "3",
	})
	if view.Progress.Chapter == nil || view.Progress.Chapter.Position != "42" {
		t.Fatalf("chapter = %+v, want 42", view.Progress.Chapter)
	}
	if view.Progress.Volume == nil || view.Progress.Volume.Position != "3" {
		t.Fatalf("volume = %+v, want 3", view.Progress.Volume)
	}

	view = mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-pu-3", view.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "20",
		ObservedAt:   time.Now().UTC().Add(-time.Hour),
	})
	if view.Progress.Chapter == nil || view.Progress.Chapter.Position != "42" {
		t.Fatalf("chapter after past-dated row = %+v, want 42 (past-dated must not overtake)", view.Progress.Chapter)
	}
}

// TestProgressNoAutomaticOtherMutation proves recording progress never
// touches consumption or publication state.
func TestProgressNoAutomaticOtherMutation(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Isolated Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga I")

	view := mustSetConsumptionView(t, s, SetConsumptionRequest{
		MutationBase: mustBase(t, "req-iso-p1", e.Revision),
		EditionID:    e.ID,
		Value:        "completed",
	})
	view = mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-iso-p2", view.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "42",
	})
	if view.Consumption == nil || view.Consumption.Value != domain.ConsumptionCompleted {
		t.Fatalf("consumption after progress = %+v, want unchanged completed", view.Consumption)
	}
	if view.Publication != nil {
		t.Fatalf("publication = %+v, want nil (progress must not imply publication)", view.Publication)
	}
	// Recording a publication afterwards is independent of the progress.
	pub, err := s.db.RecordPublication(context.Background(), s.now(), e.ID, view.Revision, store.PublicationInput{
		Value: domain.PublicationOngoing, Source: appStr("manual"), Provenance: "manual", ObservedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("RecordPublication: %v", err)
	}
	if pub.Publication == nil || pub.Publication.Value != domain.PublicationOngoing {
		t.Fatalf("publication = %+v, want ongoing", pub.Publication)
	}
	if pub.Progress == nil || len(pub.Progress) != 1 || *pub.Progress[0].Position != "42" {
		t.Fatalf("progress after publication = %+v, want unchanged chapter 42", pub.Progress)
	}
	// A store-level edition without a consumption observation stays absent:
	// progress never invents one.
	w2 := mustCreateWork(t, s, "Fresh Story")
	e2 := addEdition(t, s, w2.ID, domain.KindManga, "Manga I2")
	view = mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-iso-p3", e2.Revision),
		EditionID:    e2.ID,
		Unit:         domain.UnitChapter,
		Position:     "1",
	})
	if view.Consumption != nil {
		t.Fatalf("fresh edition consumption after progress = %+v, want absent (progress must not invent a state)", view.Consumption)
	}
	// A newly manually added edition keeps its not_started default after
	// progress is recorded: progress never implicitly starts or completes
	// the work.
	req := baseReq(t, "req-iso-p4", "Manual Fresh Story")
	req.Kind = appKind(domain.KindManga)
	created, err := s.CreateLibraryEntry(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateLibraryEntry: %v", err)
	}
	if created.EditionID == nil {
		t.Fatalf("created = %+v, want an edition", created)
	}
	view = mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "req-iso-p5", 0),
		EditionID:    *created.EditionID,
		Unit:         domain.UnitChapter,
		Position:     "1",
	})
	if view.Consumption == nil || view.Consumption.Value != domain.ConsumptionNotStarted {
		t.Fatalf("manual edition consumption after progress = %+v, want the not_started default", view.Consumption)
	}
}
