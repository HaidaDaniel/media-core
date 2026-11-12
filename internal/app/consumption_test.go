package app

import (
	"context"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func mustBase(t *testing.T, id string, revision int64) MutationBase {
	t.Helper()
	return MutationBase{
		RequestID:        id,
		Actor:            Actor{Principal: "local", Origin: domain.OriginUser},
		ExpectedRevision: &revision,
	}
}

func mustSetConsumptionView(t *testing.T, s *Service, req SetConsumptionRequest) EditionView {
	t.Helper()
	view, err := s.SetConsumption(context.Background(), req)
	if err != nil {
		t.Fatalf("SetConsumption(%q): %v", req.Value, err)
	}
	return view
}

func editionIDFromView(t *testing.T, s *Service, workID string, index int) string {
	t.Helper()
	w, err := s.GetWork(context.Background(), workID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if index >= len(w.Editions) {
		t.Fatalf("work %s has %d editions, want at least %d", workID, len(w.Editions), index+1)
	}
	return w.Editions[index].ID
}

// TestConsumptionSetReplayAndConflict proves the service update path: the
// view carries the new state, an exact retry replays without a second row,
// and a different payload on the same key conflicts.
func TestConsumptionSetReplayAndConflict(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Replay Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga R")

	view := mustSetConsumptionView(t, s, SetConsumptionRequest{
		MutationBase: mustBase(t, "req-consumption-1", e.Revision),
		EditionID:    e.ID,
		Value:        "in_progress",
		Reason:       appStr("started"),
	})
	if view.Revision != e.Revision+1 {
		t.Fatalf("view revision = %d, want %d", view.Revision, e.Revision+1)
	}
	if view.Consumption == nil || view.Consumption.Value != domain.ConsumptionInProgress {
		t.Fatalf("view consumption = %+v, want in_progress", view.Consumption)
	}
	if view.Consumption.Reason == nil || *view.Consumption.Reason != "started" {
		t.Fatalf("view reason = %+v, want started", view.Consumption.Reason)
	}
	if view.Consumption.Actor.Principal != "local" || view.Consumption.Actor.Origin != domain.OriginUser {
		t.Fatalf("view actor = %+v, want local user", view.Consumption.Actor)
	}

	again := mustSetConsumptionView(t, s, SetConsumptionRequest{
		MutationBase: mustBase(t, "req-consumption-1", e.Revision),
		EditionID:    e.ID,
		Value:        "in_progress",
		Reason:       appStr("started"),
	})
	if again.Revision != view.Revision {
		t.Fatalf("replayed revision = %d, want %d (replay adds nothing)", again.Revision, view.Revision)
	}

	req := SetConsumptionRequest{
		MutationBase: mustBase(t, "req-consumption-1", view.Revision),
		EditionID:    e.ID,
		Value:        "completed",
	}
	if _, err := s.SetConsumption(context.Background(), req); appError(t, err).Code != ErrConflict {
		t.Fatalf("same key different payload: want conflict, got %v", err)
	}
}

// TestConsumptionStaleRevisionAndUnknown proves the stable error mapping: a
// wrong revision is a conflict, a missing edition a not-found, and an
// invalid value an invalid argument.
func TestConsumptionStaleRevisionAndUnknown(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Stale Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga S")

	if _, err := s.SetConsumption(context.Background(), SetConsumptionRequest{
		MutationBase: mustBase(t, "req-stale", 99),
		EditionID:    e.ID,
		Value:        "in_progress",
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("stale revision: want conflict, got %v", err)
	}
	if _, err := s.SetConsumption(context.Background(), SetConsumptionRequest{
		MutationBase: mustBase(t, "req-missing", 0),
		EditionID:    "00000000-0000-4000-8000-000000000000",
		Value:        "in_progress",
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("unknown edition: want not_found, got %v", err)
	}
	if _, err := s.SetConsumption(context.Background(), SetConsumptionRequest{
		MutationBase: mustBase(t, "req-invalid", e.Revision),
		EditionID:    e.ID,
		Value:        "on_hold",
	}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("invalid value: want invalid_argument, got %v", err)
	}
	if _, err := s.SetConsumption(context.Background(), SetConsumptionRequest{
		MutationBase: mustBase(t, "req-supersede-missing", e.Revision),
		EditionID:    e.ID,
		Value:        "in_progress",
		SupersedesID: appStr("00000000-0000-4000-8000-000000000000"),
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("unknown supersede target: want not_found, got %v", err)
	}
}

// TestConsumptionRetractUnknownAndReread proves the retraction renders as
// the absence of a state, a reread moves completed to in_progress by
// explicit action, and a superseded row stays superseded.
func TestConsumptionRetractUnknownAndReread(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Reread Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga Re")

	view := mustSetConsumptionView(t, s, SetConsumptionRequest{
		MutationBase: mustBase(t, "req-reread-1", e.Revision),
		EditionID:    e.ID,
		Value:        "completed",
	})
	// Retract the completed claim: the state becomes unknown.
	view = mustSetConsumptionView(t, s, SetConsumptionRequest{
		MutationBase: mustBase(t, "req-reread-2", view.Revision),
		EditionID:    e.ID,
		Value:        "unknown",
		Reason:       appStr("never finished"),
	})
	if view.Consumption != nil {
		t.Fatalf("retracted view consumption = %+v, want nil", view.Consumption)
	}
	// Reread: an explicit transition out of unknown.
	view = mustSetConsumptionView(t, s, SetConsumptionRequest{
		MutationBase: mustBase(t, "req-reread-3", view.Revision),
		EditionID:    e.ID,
		Value:        "in_progress",
	})
	if view.Consumption == nil || view.Consumption.Value != domain.ConsumptionInProgress {
		t.Fatalf("reread consumption = %+v, want in_progress", view.Consumption)
	}
}

// TestConsumptionSupersedeConflict proves a second correction of the same
// row is a conflict while the first correction stays current.
func TestConsumptionSupersedeConflict(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Supersede Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga Su")

	view := mustSetConsumptionView(t, s, SetConsumptionRequest{
		MutationBase: mustBase(t, "req-supe-1", e.Revision),
		EditionID:    e.ID,
		Value:        "in_progress",
	})
	firstID := view.Consumption.SupersedesID
	view = mustSetConsumptionView(t, s, SetConsumptionRequest{
		MutationBase: mustBase(t, "req-supe-2", view.Revision),
		EditionID:    e.ID,
		Value:        "completed",
	})
	// Two corrections targeting the first row: one wins, one conflicts.
	errCh := make(chan error, 2)
	go func() {
		_, err := s.SetConsumption(context.Background(), SetConsumptionRequest{
			MutationBase: mustBase(t, "req-supe-3", view.Revision),
			EditionID:    e.ID,
			Value:        "paused",
			SupersedesID: firstID,
		})
		errCh <- err
	}()
	go func() {
		_, err := s.SetConsumption(context.Background(), SetConsumptionRequest{
			MutationBase: mustBase(t, "req-supe-4", view.Revision),
			EditionID:    e.ID,
			Value:        "dropped",
			SupersedesID: firstID,
		})
		errCh <- err
	}()
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
}

// TestConsumptionManualAdditionDefault proves a newly manually added
// edition starts not_started, while a work added without an edition has no
// consumption at all.
func TestConsumptionManualAdditionDefault(t *testing.T) {
	s := newTestService(t)
	req := baseReq(t, "req-default", "Default Story")
	req.Kind = appKind(domain.KindManga)
	created, err := s.CreateLibraryEntry(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateLibraryEntry: %v", err)
	}
	if created.EditionID == nil {
		t.Fatalf("created = %+v, want an edition", created)
	}
	e, err := s.GetEdition(context.Background(), *created.EditionID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if e.Consumption == nil || e.Consumption.Value != domain.ConsumptionNotStarted {
		t.Fatalf("fresh manual edition consumption = %+v, want not_started", e.Consumption)
	}
	if e.Consumption.Actor.Principal != "local" || e.Consumption.Actor.Origin != domain.OriginUser {
		t.Fatalf("default consumption actor = %+v, want the adding actor", e.Consumption.Actor)
	}
}

// TestStatusFilterLibrary proves the app-level consumption filter dimension:
// values OR, unknown selects editions without a live state, and a cursor
// from a different filter is rejected.
func TestStatusFilterLibrary(t *testing.T) {
	s := newTestService(t)
	wA := mustCreateWork(t, s, "Alpha Filter")
	eA := addEdition(t, s, wA.ID, domain.KindManga, "Alpha Manga")
	wB := mustCreateWork(t, s, "Beta Filter")
	addEdition(t, s, wB.ID, domain.KindManga, "Beta Manga")
	wC := mustCreateWork(t, s, "Gamma Filter")
	eC := addEdition(t, s, wC.ID, domain.KindAnime, "Gamma Anime")

	mustSetConsumptionView(t, s, SetConsumptionRequest{
		MutationBase: mustBase(t, "req-fil-1", eA.Revision),
		EditionID:    eA.ID,
		Value:        "completed",
	})
	mustSetConsumptionView(t, s, SetConsumptionRequest{
		MutationBase: mustBase(t, "req-fil-2", eC.Revision),
		EditionID:    eC.ID,
		Value:        "paused",
	})

	list := func(f LibraryFilter) []WorkCard {
		t.Helper()
		page, err := s.SearchLibrary(context.Background(), f)
		if err != nil {
			t.Fatalf("SearchLibrary: %v", err)
		}
		cards := make([]WorkCard, 0, len(page.Items))
		for _, item := range page.Items {
			cards = append(cards, item.(WorkCard))
		}
		return cards
	}
	workIDs := func(cards []WorkCard) map[string]bool {
		got := map[string]bool{}
		for _, c := range cards {
			got[c.WorkID] = true
		}
		return got
	}
	want := func(t *testing.T, cards []WorkCard, ids ...string) {
		t.Helper()
		got := workIDs(cards)
		if len(got) != len(ids) {
			t.Fatalf("works = %v, want exactly %v", got, ids)
		}
		for _, id := range ids {
			if !got[id] {
				t.Fatalf("works = %v, want to include %s", got, id)
			}
		}
	}

	want(t, list(LibraryFilter{Consumptions: []domain.Consumption{domain.ConsumptionCompleted}}), wA.ID)
	want(t, list(LibraryFilter{Consumptions: []domain.Consumption{domain.ConsumptionPaused, domain.ConsumptionCompleted}}), wA.ID, wC.ID)
	want(t, list(LibraryFilter{UnknownConsumption: true}), wB.ID)
	want(t, list(LibraryFilter{Consumptions: []domain.Consumption{domain.ConsumptionCompleted}, UnknownConsumption: true}), wA.ID, wB.ID)
	want(t, list(LibraryFilter{}), wA.ID, wB.ID, wC.ID)

	// A cursor from the unfiltered listing must not be used with the
	// consumption filter.
	page, err := s.SearchLibrary(context.Background(), LibraryFilter{Limit: 1})
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	if page.NextCursor == nil {
		t.Fatalf("first page: want a next cursor")
	}
	if _, err := s.SearchLibrary(context.Background(), LibraryFilter{Consumptions: []domain.Consumption{domain.ConsumptionCompleted}, Limit: 1, Cursor: page.NextCursor}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("cross-filter cursor: want invalid_argument, got %v", err)
	}
}

// TestConsumptionNoAutomaticOtherMutation proves setting consumption never
// touches publication, intention or progress state.
func TestConsumptionNoAutomaticOtherMutation(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Isolated Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga I")

	view := mustSetConsumptionView(t, s, SetConsumptionRequest{
		MutationBase: mustBase(t, "req-iso-1", e.Revision),
		EditionID:    e.ID,
		Value:        "completed",
	})
	if view.Publication != nil {
		t.Fatalf("publication = %+v, want nil (consumption must not imply publication)", view.Publication)
	}
	if view.Consumption == nil || view.Consumption.Value != domain.ConsumptionCompleted {
		t.Fatalf("consumption = %+v, want completed", view.Consumption)
	}
	// A publication recorded afterwards is independent of the status.
	pub, err := s.db.RecordPublication(context.Background(), s.now(), e.ID, view.Revision, store.PublicationInput{
		Value: domain.PublicationOngoing, Source: appStr("manual"), Provenance: "manual", ObservedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("RecordPublication: %v", err)
	}
	if pub.Publication == nil || pub.Publication.Value != domain.PublicationOngoing {
		t.Fatalf("publication = %+v, want ongoing", pub.Publication)
	}
	if pub.Consumption == nil || pub.Consumption.Value != string(domain.ConsumptionCompleted) {
		t.Fatalf("consumption after publication = %+v, want unchanged completed", pub.Consumption)
	}
}
