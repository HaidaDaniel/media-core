package app

import (
	"context"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

func mustSetIntentionView(t *testing.T, s *Service, req SetIntentionRequest) EditionView {
	t.Helper()
	view, err := s.SetIntention(context.Background(), req)
	if err != nil {
		t.Fatalf("SetIntention(%s): %v", req.Value, err)
	}
	return view
}

// TestIntentionSetReplayAndConflict proves the service set path: the view
// carries the current intention, none renders as the absence of a
// recorded intention, an exact retry replays without a second observation,
// and a different payload on the same key conflicts.
func TestIntentionSetReplayAndConflict(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Intention Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga I")

	view := mustSetIntentionView(t, s, SetIntentionRequest{
		MutationBase: mustBase(t, "req-int-1", e.Revision),
		EditionID:    e.ID,
		Value:        string(domain.IntentionSaved),
	})
	if view.Revision != e.Revision+1 {
		t.Fatalf("view revision = %d, want %d", view.Revision, e.Revision+1)
	}
	if view.Intention == nil || view.Intention.Value != domain.IntentionSaved {
		t.Fatalf("view intention = %+v, want saved", view.Intention)
	}
	if view.Intention.Actor.Principal != "local" || view.Intention.Actor.Origin != domain.OriginUser {
		t.Fatalf("view actor = %+v, want local user", view.Intention.Actor)
	}

	// An explicit retraction to none renders as the absence of a
	// recorded intention, never as a value.
	view = mustSetIntentionView(t, s, SetIntentionRequest{
		MutationBase: mustBase(t, "req-int-none", view.Revision),
		EditionID:    e.ID,
		Value:        "none",
		Reason:       strPtr("changed my mind"),
	})
	if view.Intention != nil {
		t.Fatalf("view intention = %+v, want null after the retraction", view.Intention)
	}

	// An exact retry of the first mutation replays the stored view without
	// a new observation or revision bump.
	replayed := mustSetIntentionView(t, s, SetIntentionRequest{
		MutationBase: mustBase(t, "req-int-1", e.Revision),
		EditionID:    e.ID,
		Value:        string(domain.IntentionSaved),
	})
	if replayed.Revision != view.Revision {
		t.Fatalf("replayed revision = %d, want %d (a retry appends nothing)", replayed.Revision, view.Revision)
	}

	req := SetIntentionRequest{
		MutationBase: mustBase(t, "req-int-1", view.Revision),
		EditionID:    e.ID,
		Value:        string(domain.IntentionPlanned),
	}
	if _, err := s.SetIntention(context.Background(), req); appError(t, err).Code != ErrConflict {
		t.Fatalf("same key different payload: want conflict, got %v", err)
	}
}

// TestIntentionStaleInvalidAndUnknown proves the stable error mapping: a
// wrong revision is a conflict, a missing edition a not-found, an unknown
// or already-superseded correction target a not-found or conflict, and an
// invalid value or missing identity field an invalid argument.
func TestIntentionStaleInvalidAndUnknown(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Invalid Intention")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga V")

	if _, err := s.SetIntention(context.Background(), SetIntentionRequest{
		MutationBase: mustBase(t, "req-i-stale", 99),
		EditionID:    e.ID,
		Value:        string(domain.IntentionSaved),
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("stale revision: want conflict, got %v", err)
	}
	if _, err := s.SetIntention(context.Background(), SetIntentionRequest{
		MutationBase: mustBase(t, "req-i-missing", 0),
		EditionID:    "00000000-0000-4000-8000-000000000000",
		Value:        string(domain.IntentionSaved),
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("unknown edition: want not_found, got %v", err)
	}
	if _, err := s.SetIntention(context.Background(), SetIntentionRequest{
		MutationBase: mustBase(t, "req-i-invalid", e.Revision),
		EditionID:    e.ID,
		Value:        "maybe",
	}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("invalid value: want invalid_argument, got %v", err)
	}
	noID := MutationBase{
		Actor:            Actor{Principal: "local", Origin: domain.OriginUser},
		ExpectedRevision: &e.Revision,
	}
	if _, err := s.SetIntention(context.Background(), SetIntentionRequest{
		MutationBase: noID,
		EditionID:    e.ID,
		Value:        string(domain.IntentionSaved),
	}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("missing request id: want invalid_argument, got %v", err)
	}
	view := mustSetIntentionView(t, s, SetIntentionRequest{
		MutationBase: mustBase(t, "req-i-first", e.Revision),
		EditionID:    e.ID,
		Value:        string(domain.IntentionInterested),
	})
	fresh, err := s.db.GetEdition(context.Background(), e.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if fresh.Intention == nil {
		t.Fatal("edition intention = nil, want the interested observation")
	}
	first := fresh.Intention.ID
	if _, err := s.SetIntention(context.Background(), SetIntentionRequest{
		MutationBase: mustBase(t, "req-i-unknown-sup", view.Revision),
		EditionID:    e.ID,
		Value:        string(domain.IntentionSaved),
		SupersedesID: strPtr("00000000-0000-4000-8000-000000000000"),
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("unknown supersede target: want not_found, got %v", err)
	}
	view = mustSetIntentionView(t, s, SetIntentionRequest{
		MutationBase: mustBase(t, "req-i-second", view.Revision),
		EditionID:    e.ID,
		Value:        string(domain.IntentionSaved),
		SupersedesID: strPtr(first),
	})
	if _, err := s.SetIntention(context.Background(), SetIntentionRequest{
		MutationBase: mustBase(t, "req-i-double", view.Revision),
		EditionID:    e.ID,
		Value:        string(domain.IntentionPlanned),
		SupersedesID: strPtr(first),
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("already-superseded target: want conflict, got %v", err)
	}
}
