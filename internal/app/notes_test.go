package app

import (
	"context"
	"strings"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func mustAddEditionNoteView(t *testing.T, s *Service, req AddNoteRequest) EditionView {
	t.Helper()
	view, err := s.AddEditionNote(context.Background(), req)
	if err != nil {
		t.Fatalf("AddEditionNote(%q): %v", req.TargetID, err)
	}
	return view
}

// TestNoteAddReviseRetract proves the service note lifecycle on an edition:
// add renders the note, revise keeps one note at its new revision, a later
// thought is a distinct note, retract removes the note from the view, and
// the note never touches consumption.
func TestNoteAddReviseRetract(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Deferral Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga D")

	ev := mustAddEditionNoteView(t, s, AddNoteRequest{
		MutationBase: mustBase(t, "req-note-add1", 0),
		TargetID:     e.ID,
		Text:         "Saved for later: reading manga on my phone is inconvenient.",
	})
	if len(ev.Notes) != 1 || ev.Notes[0].Text != "Saved for later: reading manga on my phone is inconvenient." {
		t.Fatalf("edition notes = %+v, want the deferral note", ev.Notes)
	}
	note := ev.Notes[0]
	if note.TargetID != e.ID || note.Revision != 1 || note.ID == "" {
		t.Fatalf("note = %+v, want the first revision on the edition", note)
	}
	if ev.Consumption != nil {
		t.Fatalf("consumption = %+v, want untouched (a note never marks dropped)", ev.Consumption)
	}

	// A later thought is a distinct note, not a revision of the first.
	ev = mustAddEditionNoteView(t, s, AddNoteRequest{
		MutationBase: mustBase(t, "req-note-add2", 0),
		TargetID:     e.ID,
		Text:         "Maybe recheck in winter.",
	})
	if len(ev.Notes) != 2 || ev.Notes[0].ID != note.ID || ev.Notes[1].Text != "Maybe recheck in winter." {
		t.Fatalf("edition notes = %+v, want the distinct later note", ev.Notes)
	}

	// Editing keeps the same note at its new revision.
	view, err := s.ReviseNote(context.Background(), ReviseNoteRequest{
		MutationBase: mustBase(t, "req-note-rev1", 1),
		NoteID:       note.ID,
		Text:         "Saved for later: phone reading is inconvenient.",
	})
	if err != nil {
		t.Fatalf("ReviseNote: %v", err)
	}
	ev = view.(EditionView)
	if len(ev.Notes) != 2 || ev.Notes[0].Text != "Saved for later: phone reading is inconvenient." || ev.Notes[0].Revision != 2 {
		t.Fatalf("edition notes = %+v, want the edited note at revision 2", ev.Notes)
	}

	// The retraction removes the note from the view; the other note
	// survives.
	view, err = s.RetractNote(context.Background(), RetractNoteRequest{
		MutationBase: mustBase(t, "req-note-ret1", 2),
		NoteID:       note.ID,
		Reason:       strPtr("superseded by the later note"),
	})
	if err != nil {
		t.Fatalf("RetractNote: %v", err)
	}
	ev = view.(EditionView)
	if len(ev.Notes) != 1 || ev.Notes[0].Text != "Maybe recheck in winter." {
		t.Fatalf("edition notes = %+v, want only the surviving note", ev.Notes)
	}
}

// TestNoteWorkTarget proves the work target path: the note is stored and
// queryable on the work, the work view returns, and a work note never
// leaks into an edition view.
func TestNoteWorkTarget(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Work Notes")
	e := addEdition(t, s, w.ID, domain.KindAnime, "Anime W")

	wv, err := s.AddWorkNote(context.Background(), AddNoteRequest{
		MutationBase: mustBase(t, "req-note-w1", 0),
		TargetID:     w.ID,
		Text:         "Work-level thought.",
	})
	if err != nil {
		t.Fatalf("AddWorkNote: %v", err)
	}
	if wv.ID != w.ID {
		t.Fatalf("work view id = %s, want %s", wv.ID, w.ID)
	}
	workNotes, err := s.db.ListWorkNotes(context.Background(), w.ID)
	if err != nil {
		t.Fatalf("ListWorkNotes: %v", err)
	}
	if len(workNotes) != 1 || workNotes[0].Text != "Work-level thought." {
		t.Fatalf("work notes = %+v, want the stored work note", workNotes)
	}

	// The work note does not leak into the edition view.
	ev := mustAddEditionNoteView(t, s, AddNoteRequest{
		MutationBase: mustBase(t, "req-note-we1", 0),
		TargetID:     e.ID,
		Text:         "Edition thought.",
	})
	if len(ev.Notes) != 1 || ev.Notes[0].Text != "Edition thought." {
		t.Fatalf("edition notes = %+v, want only the edition note", ev.Notes)
	}
}

// TestNoteErrorsAndReplay proves the stable error mapping on the note path:
// a blank target or oversized text is an invalid argument that reserves no
// key, a stale note revision conflicts, a missing note or target is a
// not-found, and an exact retry replays.
func TestNoteErrorsAndReplay(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Note Errors")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga E")

	// A blank target is an invalid argument.
	if _, err := s.AddEditionNote(context.Background(), AddNoteRequest{
		MutationBase: mustBase(t, "req-note-blank", 0),
		TargetID:     "   ",
		Text:         "text",
	}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("blank target: want invalid_argument, got %v", err)
	}

	// Oversized text fails and reserves no key: the same key can carry a
	// valid add afterwards.
	over := strings.Repeat("a", store.MaxNoteTextBytes+1)
	if _, err := s.AddEditionNote(context.Background(), AddNoteRequest{
		MutationBase: mustBase(t, "req-note-oversized", 0),
		TargetID:     e.ID,
		Text:         over,
	}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("oversized text: want invalid_argument, got %v", err)
	}
	ev := mustAddEditionNoteView(t, s, AddNoteRequest{
		MutationBase: mustBase(t, "req-note-oversized", 0),
		TargetID:     e.ID,
		Text:         "fits now",
	})
	if len(ev.Notes) != 1 || ev.Notes[0].Text != "fits now" {
		t.Fatalf("edition notes = %+v, want the valid add on the reused key", ev.Notes)
	}
	note := ev.Notes[0]

	// An exact retry of the add replays without a second note.
	replayed := mustAddEditionNoteView(t, s, AddNoteRequest{
		MutationBase: mustBase(t, "req-note-oversized", 0),
		TargetID:     e.ID,
		Text:         "fits now",
	})
	if len(replayed.Notes) != 1 || replayed.Notes[0].ID != note.ID {
		t.Fatalf("replayed notes = %+v, want the same single note", replayed.Notes)
	}

	// A different payload on the same key conflicts.
	if _, err := s.AddEditionNote(context.Background(), AddNoteRequest{
		MutationBase: mustBase(t, "req-note-oversized", 0),
		TargetID:     e.ID,
		Text:         "different",
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("same key different payload: want conflict, got %v", err)
	}

	// A stale note revision conflicts; a missing note and a missing target
	// are not-found.
	if _, err := s.ReviseNote(context.Background(), ReviseNoteRequest{
		MutationBase: mustBase(t, "req-note-stale", 99),
		NoteID:       note.ID,
		Text:         "stale",
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("stale note revision: want conflict, got %v", err)
	}
	if _, err := s.RetractNote(context.Background(), RetractNoteRequest{
		MutationBase: mustBase(t, "req-note-missing", 1),
		NoteID:       "00000000-0000-4000-8000-000000000000",
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("missing note: want not_found, got %v", err)
	}
	if _, err := s.AddWorkNote(context.Background(), AddNoteRequest{
		MutationBase: mustBase(t, "req-note-mw", 0),
		TargetID:     "00000000-0000-4000-8000-000000000000",
		Text:         "text",
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("missing work: want not_found, got %v", err)
	}
	if _, err := s.AddEditionNote(context.Background(), AddNoteRequest{
		MutationBase: mustBase(t, "req-note-me", 0),
		TargetID:     "00000000-0000-4000-8000-000000000000",
		Text:         "text",
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("missing edition: want not_found, got %v", err)
	}

	// A revise retry with the same key replays the stored target view.
	rev, err := s.ReviseNote(context.Background(), ReviseNoteRequest{
		MutationBase: mustBase(t, "req-note-retry", 1),
		NoteID:       note.ID,
		Text:         "edited once",
	})
	if err != nil {
		t.Fatalf("ReviseNote: %v", err)
	}
	if ev2 := rev.(EditionView); ev2.Notes[0].Revision != 2 {
		t.Fatalf("revised note = %+v, want revision 2", ev2.Notes[0])
	}
	replayedRev, err := s.ReviseNote(context.Background(), ReviseNoteRequest{
		MutationBase: mustBase(t, "req-note-retry", 1),
		NoteID:       note.ID,
		Text:         "edited once",
	})
	if err != nil {
		t.Fatalf("replayed ReviseNote: %v", err)
	}
	if ev2 := replayedRev.(EditionView); len(ev2.Notes) != 1 || ev2.Notes[0].Revision != 2 {
		t.Fatalf("replayed notes = %+v, want the single note at revision 2", ev2.Notes)
	}

	// The retraction removes the note from the view.
	ret, err := s.RetractNote(context.Background(), RetractNoteRequest{
		MutationBase: mustBase(t, "req-note-ret2", 2),
		NoteID:       note.ID,
	})
	if err != nil {
		t.Fatalf("RetractNote: %v", err)
	}
	if ev3 := ret.(EditionView); len(ev3.Notes) != 0 {
		t.Fatalf("edition notes = %+v, want empty after the retraction", ev3.Notes)
	}
}
