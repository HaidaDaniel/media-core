package store

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

func mustAddEditionNote(t *testing.T, db *Database, now time.Time, e Edition, text string) Edition {
	t.Helper()
	fresh, err := db.AddEditionNote(ctx(t), now, e.ID, NoteInput{Text: text}, NoteActor{Principal: "local", Origin: "user"}, nil)
	if err != nil {
		t.Fatalf("AddEditionNote(%q): %v", text, err)
	}
	return fresh
}

func mustAddWorkNote(t *testing.T, db *Database, now time.Time, w Work, text string) Work {
	t.Helper()
	fresh, err := db.AddWorkNote(ctx(t), now, w.ID, NoteInput{Text: text}, NoteActor{Principal: "local", Origin: "user"}, nil)
	if err != nil {
		t.Fatalf("AddWorkNote(%q): %v", text, err)
	}
	return fresh
}

// TestNoteAddWorkAndEdition proves a note targets exactly one work or
// edition: each target keeps its own notes, notes never bump the target
// revision, and a note never touches consumption.
func TestNoteAddWorkAndEdition(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Note Story")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga N")
	workRev, edRev := w.Revision, e.Revision
	before, err := db.GetEdition(ctx(t), e.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}

	e = mustAddEditionNote(t, db, now.Add(2*time.Second), e, "Interesting anime. Original is manga.")
	if e.Revision != edRev {
		t.Fatalf("edition revision = %d, want %d (a note does not bump the target)", e.Revision, edRev)
	}
	if len(e.Notes) != 1 {
		t.Fatalf("edition notes = %+v, want 1", e.Notes)
	}
	n := e.Notes[0]
	if n.TargetID != e.ID || n.Revision != 1 || n.Text != "Interesting anime. Original is manga." || n.Spoiler {
		t.Fatalf("note = %+v, want the edition note at revision 1", n)
	}
	if n.Quote != nil || n.Location != nil {
		t.Fatalf("note quote/location = (%v, %v), want null", n.Quote, n.Location)
	}
	if n.Actor.Principal != "local" {
		t.Fatalf("note actor = %+v, want the acting principal", n.Actor)
	}

	// The work note is a distinct note on the work target.
	w = mustAddWorkNote(t, db, now.Add(3*time.Second), w, "Work-level thought.")
	if w.Revision != workRev {
		t.Fatalf("work revision = %d, want %d (a note does not bump the target)", w.Revision, workRev)
	}

	// The targets are independent: the edition view shows only its own
	// note, the work note stays a work-scoped row.
	fresh, err := db.GetEdition(ctx(t), e.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if len(fresh.Notes) != 1 || fresh.Notes[0].Text != "Interesting anime. Original is manga." {
		t.Fatalf("edition notes = %+v, want only the edition note (no work-note leak)", fresh.Notes)
	}
	workNotes, err := db.ListWorkNotes(ctx(t), w.ID)
	if err != nil {
		t.Fatalf("ListWorkNotes: %v", err)
	}
	if len(workNotes) != 1 || workNotes[0].Text != "Work-level thought." || workNotes[0].TargetID != w.ID {
		t.Fatalf("work notes = %+v, want the work-scoped note", workNotes)
	}

	// A note never changes consumption: the observation is untouched.
	if (fresh.Consumption == nil) != (before.Consumption == nil) ||
		(before.Consumption != nil && fresh.Consumption.Value != before.Consumption.Value) {
		t.Fatalf("consumption = %+v, want the untouched %v", fresh.Consumption, before.Consumption)
	}
}

// TestNoteReviseKeepsHistory proves a revision points to the immediately
// preceding revision, the original text and audit survive, and the view
// shows the latest revision.
func TestNoteReviseKeepsHistory(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Revise Story")
	e, _ := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga R")

	e = mustAddEditionNote(t, db, now.Add(2*time.Second), e, "Original text.")
	noteID := e.Notes[0].ID
	quote, location := "a quote", "chapter 1"

	fresh, err := db.ReviseNote(ctx(t), now.Add(3*time.Second), noteID, 1, NoteInput{
		Text:     "Edited text.",
		Spoiler:  true,
		Quote:    &quote,
		Location: &location,
	}, NoteActor{Principal: "local", Origin: "user"}, nil)
	if err != nil {
		t.Fatalf("ReviseNote: %v", err)
	}
	if fresh.IsWork || len(fresh.Edition.Notes) != 1 || fresh.Edition.Notes[0].Text != "Edited text." || fresh.Edition.Notes[0].Revision != 2 {
		t.Fatalf("edition notes = %+v, want the edited text at revision 2", fresh.Edition.Notes)
	}
	if !fresh.Edition.Notes[0].Spoiler || fresh.Edition.Notes[0].Quote == nil || *fresh.Edition.Notes[0].Quote != quote ||
		fresh.Edition.Notes[0].Location == nil || *fresh.Edition.Notes[0].Location != location {
		t.Fatalf("edited note = %+v, want the spoiler, quote and location", fresh.Edition.Notes[0])
	}

	// Both revisions survive: the old row keeps the original text, the new
	// row points to it.
	var (
		rows      int
		oldText   string
		newPrevID string
	)
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM note_revisions WHERE note_id = ?`, noteID).Scan(&rows); err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if rows != 2 {
		t.Fatalf("revision rows = %d, want 2 (the original is preserved)", rows)
	}
	if err := db.DB().QueryRow(`SELECT text FROM note_revisions WHERE note_id = ? AND revision = 1`, noteID).
		Scan(&oldText); err != nil {
		t.Fatalf("read old revision: %v", err)
	}
	if oldText != "Original text." {
		t.Fatalf("revision 1 text = %q, want the original text", oldText)
	}
	if err := db.DB().QueryRow(`SELECT prev_id FROM note_revisions WHERE note_id = ? AND revision = 2`, noteID).
		Scan(&newPrevID); err != nil {
		t.Fatalf("read new revision: %v", err)
	}
	var oldID string
	if err := db.DB().QueryRow(`SELECT id FROM note_revisions WHERE note_id = ? AND revision = 1`, noteID).
		Scan(&oldID); err != nil {
		t.Fatalf("read old revision id: %v", err)
	}
	if newPrevID != oldID {
		t.Fatalf("revision 2 prev_id = %s, want revision 1 id %s", newPrevID, oldID)
	}
}

// TestNoteRevisionConflict proves a stale expected revision is a distinct
// conflict, leaves every row intact and reserves no journal key.
func TestNoteRevisionConflict(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Conflict Story")
	e, _ := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga C")

	e = mustAddEditionNote(t, db, now.Add(2*time.Second), e, "First thought.")
	noteID := e.Notes[0].ID

	if _, err := db.ReviseNote(ctx(t), now.Add(3*time.Second), noteID, 99, NoteInput{Text: "Stale write."},
		NoteActor{Principal: "local", Origin: "user"}, &JournalInput{RequestID: "stale-revise"}); !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("stale revise: err = %v, want ErrRevisionMismatch", err)
	}
	if _, err := db.RetractNote(ctx(t), now.Add(3*time.Second), noteID, 99, nil,
		NoteActor{Principal: "local", Origin: "user"}, &JournalInput{RequestID: "stale-retract"}); !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("stale retract: err = %v, want ErrRevisionMismatch", err)
	}

	var rows, journalRows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM note_revisions WHERE note_id = ?`, noteID).Scan(&rows); err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if rows != 1 {
		t.Fatalf("revision rows = %d, want 1 (the stale writes left the prior state intact)", rows)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM mutation_journal`).Scan(&journalRows); err != nil {
		t.Fatalf("count journal: %v", err)
	}
	if journalRows != 0 {
		t.Fatalf("journal rows = %d, want 0 (a conflict reserves no request key)", journalRows)
	}
	fresh, err := db.GetEdition(ctx(t), e.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if len(fresh.Notes) != 1 || fresh.Notes[0].Text != "First thought." || fresh.Notes[0].Revision != 1 {
		t.Fatalf("edition notes = %+v, want the untouched original note", fresh.Notes)
	}
}

// TestNoteRetract proves a retraction keeps the retracted text and audit as
// a revision marker, removes the note from the view, is idempotent, and a
// later revision re-activates the note.
func TestNoteRetract(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Retract Story")
	e, _ := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga T")

	e = mustAddEditionNote(t, db, now.Add(2*time.Second), e, "A deferral explanation.")
	noteID := e.Notes[0].ID
	reason := "no longer relevant"

	fresh, err := db.RetractNote(ctx(t), now.Add(3*time.Second), noteID, 1, &reason,
		NoteActor{Principal: "local", Origin: "user"}, nil)
	if err != nil {
		t.Fatalf("RetractNote: %v", err)
	}
	if len(fresh.Edition.Notes) != 0 {
		t.Fatalf("edition notes = %+v, want empty after the retraction", fresh.Edition.Notes)
	}
	var (
		rows       int
		retracted  int
		keptText   string
		keptReason string
	)
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM note_revisions WHERE note_id = ?`, noteID).Scan(&rows); err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if rows != 2 {
		t.Fatalf("revision rows = %d, want 2 (the retraction is preserved)", rows)
	}
	if err := db.DB().QueryRow(
		`SELECT retracted, text, COALESCE(reason, '') FROM note_revisions WHERE note_id = ? AND revision = 2`, noteID).
		Scan(&retracted, &keptText, &keptReason); err != nil {
		t.Fatalf("read retraction: %v", err)
	}
	if retracted != 1 || keptText != "A deferral explanation." || keptReason != reason {
		t.Fatalf("retraction = (retracted %d, %q, %q), want the marker with the retracted text and reason", retracted, keptText, keptReason)
	}

	// A repeated retraction is a no-op: the note stays retracted, no new
	// revision, no bump.
	fresh, err = db.RetractNote(ctx(t), now.Add(4*time.Second), noteID, 2, nil,
		NoteActor{Principal: "local", Origin: "user"}, nil)
	if err != nil {
		t.Fatalf("repeated RetractNote: %v", err)
	}
	if len(fresh.Edition.Notes) != 0 {
		t.Fatalf("edition notes = %+v, want still empty", fresh.Edition.Notes)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM note_revisions WHERE note_id = ?`, noteID).Scan(&rows); err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if rows != 2 {
		t.Fatalf("revision rows = %d, want 2 after the no-op re-retraction", rows)
	}

	// A later revision re-activates the note with its new text.
	fresh, err = db.ReviseNote(ctx(t), now.Add(5*time.Second), noteID, 2, NoteInput{Text: "Back with more."},
		NoteActor{Principal: "local", Origin: "user"}, nil)
	if err != nil {
		t.Fatalf("ReviseNote after retraction: %v", err)
	}
	if len(fresh.Edition.Notes) != 1 || fresh.Edition.Notes[0].Text != "Back with more." || fresh.Edition.Notes[0].Revision != 3 {
		t.Fatalf("edition notes = %+v, want the re-activated note at revision 3", fresh.Edition.Notes)
	}
}

// TestNoteDistinctFromLaterThought proves editing keeps one note while a
// later thought is a distinct new note, both visible.
func TestNoteDistinctFromLaterThought(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Two Thoughts")
	e, _ := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga D")

	e = mustAddEditionNote(t, db, now.Add(2*time.Second), e, "First thought.")
	first := e.Notes[0].ID
	e = mustAddEditionNote(t, db, now.Add(3*time.Second), e, "Later thought.")
	if len(e.Notes) != 2 || e.Notes[0].ID != first {
		t.Fatalf("edition notes = %+v, want both distinct notes in creation order", e.Notes)
	}

	// Editing the first note keeps it the same note; the later thought
	// stays a second, distinct note.
	if _, err := db.ReviseNote(ctx(t), now.Add(4*time.Second), first, 1, NoteInput{Text: "First, edited."},
		NoteActor{Principal: "local", Origin: "user"}, nil); err != nil {
		t.Fatalf("ReviseNote: %v", err)
	}
	fresh, err := db.GetEdition(ctx(t), e.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if len(fresh.Notes) != 2 || fresh.Notes[0].Text != "First, edited." || fresh.Notes[0].Revision != 2 ||
		fresh.Notes[1].Text != "Later thought." {
		t.Fatalf("edition notes = %+v, want the edited note plus the distinct later note", fresh.Notes)
	}
}

// TestNoteValidation proves blank or oversized text, a blank quote or
// location, and a missing note or target fail distinctly without partial
// writes or reserved keys.
func TestNoteValidation(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Invalid Notes")
	e, _ := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga V")

	blank := NoteInput{Text: "   "}
	if _, err := db.AddEditionNote(ctx(t), now, e.ID, blank, NoteActor{Principal: "local"}, &JournalInput{RequestID: "n1"}); !errors.Is(err, ErrInvalidNote) {
		t.Fatalf("blank text: err = %v, want ErrInvalidNote", err)
	}
	text := make([]byte, MaxNoteTextBytes+1)
	for i := range text {
		text[i] = 'a'
	}
	over := NoteInput{Text: string(text)}
	if _, err := db.AddWorkNote(ctx(t), now, w.ID, over, NoteActor{Principal: "local"}, &JournalInput{RequestID: "n2"}); !errors.Is(err, ErrInvalidNote) {
		t.Fatalf("oversized text: err = %v, want ErrInvalidNote", err)
	}
	// The byte bound admits exactly 16 KiB, multi-byte included: 8192
	// two-byte runes are exactly 16384 bytes and are accepted, while one
	// more rune is 16386 bytes and fails.
	at := strings.Repeat("é", 8192)
	if _, err := db.AddWorkNote(ctx(t), now, w.ID, NoteInput{Text: at}, NoteActor{Principal: "local"}, nil); err != nil {
		t.Fatalf("16384 bytes of two-byte runes: err = %v, want accepted at the byte bound", err)
	}
	overMulti := strings.Repeat("é", 8193)
	if _, err := db.AddWorkNote(ctx(t), now, w.ID, NoteInput{Text: overMulti}, NoteActor{Principal: "local"}, nil); !errors.Is(err, ErrInvalidNote) {
		t.Fatalf("16386 bytes of two-byte runes: err = %v, want ErrInvalidNote (byte bound)", err)
	}
	emptyStr := ""
	if _, err := db.AddEditionNote(ctx(t), now, e.ID, NoteInput{Text: "ok", Quote: &emptyStr}, NoteActor{Principal: "local"}, nil); !errors.Is(err, ErrInvalidNote) {
		t.Fatalf("blank quote: err = %v, want ErrInvalidNote", err)
	}
	blankLoc := "  "
	if _, err := db.AddEditionNote(ctx(t), now, e.ID, NoteInput{Text: "ok", Location: &blankLoc}, NoteActor{Principal: "local"}, nil); !errors.Is(err, ErrInvalidNote) {
		t.Fatalf("blank location: err = %v, want ErrInvalidNote", err)
	}
	if _, err := db.ReviseNote(ctx(t), now, "00000000-0000-4000-8000-000000000000", 1, NoteInput{Text: "ok"},
		NoteActor{Principal: "local"}, nil); !errors.Is(err, ErrNoteNotFound) {
		t.Fatalf("missing note: err = %v, want ErrNoteNotFound", err)
	}
	if _, err := db.RetractNote(ctx(t), now, "00000000-0000-4000-8000-000000000000", 1, nil,
		NoteActor{Principal: "local"}, nil); !errors.Is(err, ErrNoteNotFound) {
		t.Fatalf("missing note retract: err = %v, want ErrNoteNotFound", err)
	}
	if _, err := db.AddWorkNote(ctx(t), now, "00000000-0000-4000-8000-000000000000", NoteInput{Text: "ok"},
		NoteActor{Principal: "local"}, nil); !errors.Is(err, ErrWorkNotFound) {
		t.Fatalf("missing work: err = %v, want ErrWorkNotFound", err)
	}
	if _, err := db.AddEditionNote(ctx(t), now, "00000000-0000-4000-8000-000000000000", NoteInput{Text: "ok"},
		NoteActor{Principal: "local"}, nil); !errors.Is(err, ErrEditionNotFound) {
		t.Fatalf("missing edition: err = %v, want ErrEditionNotFound", err)
	}

	// Nothing partial survived and no journal key was reserved.
	var noteRows, journalRows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM notes`).Scan(&noteRows); err != nil {
		t.Fatalf("count notes: %v", err)
	}
	// Only the exact-boundary work note above succeeded.
	if noteRows != 1 {
		t.Fatalf("note rows = %d, want 1 (the boundary success only)", noteRows)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM mutation_journal`).Scan(&journalRows); err != nil {
		t.Fatalf("count journal: %v", err)
	}
	if journalRows != 0 {
		t.Fatalf("journal rows = %d, want 0 (failed validation reserves no key)", journalRows)
	}
}
