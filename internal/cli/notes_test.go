package cli

import (
	"encoding/json"
	"testing"
)

type noteEntry struct {
	ID       string  `json:"id"`
	Text     string  `json:"text"`
	Spoiler  bool    `json:"spoiler"`
	Revision int64   `json:"revision"`
	Quote    *string `json:"quote"`
}

type noteData struct {
	ID     string      `json:"id"`
	WorkID string      `json:"work_id"`
	Notes  []noteEntry `json:"notes"`
}

func noteEnv(t *testing.T, out string) noteData {
	t.Helper()
	env := libEnv(t, out)
	if !env.OK || env.Error != nil {
		t.Fatalf("note envelope = %+v, want ok", env)
	}
	b, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var data noteData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("data is not a view: %v\n%s", err, out)
	}
	return data
}

// TestNoteCLI proves the note command: add on an edition renders the note,
// add on a work returns the work view, revise updates the text at the new
// revision, and retract removes the note from the view.
func TestNoteCLI(t *testing.T) {
	dir := initLibState(t)
	work, ed := addWorkWithEdition(t, dir, "Note Story")

	code, out, errw := runCLI(t, "note", "add", "edition", "--state-dir", dir, ed,
		"--text", "Saved for later: reading manga on my phone is inconvenient.", "--spoiler",
		"--quote", "a line", "--location", "chapter 1", "--request-id", "req-cli-note-1")
	if code != ExitOK {
		t.Fatalf("note add edition: exit = %d, stderr = %q", code, errw)
	}
	data := noteEnv(t, out)
	if len(data.Notes) != 1 || data.Notes[0].Text != "Saved for later: reading manga on my phone is inconvenient." ||
		!data.Notes[0].Spoiler || data.Notes[0].Quote == nil || *data.Notes[0].Quote != "a line" ||
		data.Notes[0].Revision != 1 {
		t.Fatalf("notes = %+v, want the spoiler note with quote and location at revision 1", data.Notes)
	}
	noteID := data.Notes[0].ID

	// A work note returns the work view.
	code, out, errw = runCLI(t, "note", "add", "work", "--state-dir", dir, work,
		"--text", "Work-level thought.")
	if code != ExitOK {
		t.Fatalf("note add work: exit = %d, stderr = %q", code, errw)
	}
	data = noteEnv(t, out)
	if data.ID != work {
		t.Fatalf("view id = %s, want the work %s", data.ID, work)
	}

	// The revise updates the same note at its new revision.
	code, out, errw = runCLI(t, "note", "revise", "--state-dir", dir, noteID,
		"--text", "Saved for later: phone reading is inconvenient.", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("note revise: exit = %d, stderr = %q", code, errw)
	}
	data = noteEnv(t, out)
	if len(data.Notes) != 1 || data.Notes[0].ID != noteID ||
		data.Notes[0].Text != "Saved for later: phone reading is inconvenient." || data.Notes[0].Revision != 2 {
		t.Fatalf("notes = %+v, want the revised note at revision 2", data.Notes)
	}

	// The retract removes the note from the view.
	code, out, errw = runCLI(t, "note", "retract", "--state-dir", dir, noteID,
		"--revision", "2", "--reason", "no longer relevant")
	if code != ExitOK {
		t.Fatalf("note retract: exit = %d, stderr = %q", code, errw)
	}
	data = noteEnv(t, out)
	if len(data.Notes) != 0 {
		t.Fatalf("notes = %+v, want empty after the retract", data.Notes)
	}
}

// TestNoteCLIInvalid proves the CLI rejects unknown verbs and targets,
// missing required flags, and stale revisions with the contract exit codes.
func TestNoteCLIInvalid(t *testing.T) {
	dir := initLibState(t)
	work, ed := addWorkWithEdition(t, dir, "Invalid Notes")

	if code, _, _ := runCLI(t, "note", "rename", "--state-dir", dir, work); code != ExitUsage {
		t.Fatalf("unknown verb: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "note", "add", "series", "--state-dir", dir, work, "--text", "t"); code != ExitUsage {
		t.Fatalf("unknown target: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "note", "add", "edition", "--state-dir", dir, ed); code != ExitUsage {
		t.Fatalf("missing text: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "note", "revise", "--state-dir", dir, "some-note", "--text", "t"); code != ExitUsage {
		t.Fatalf("revise without revision: exit = %d, want %d", code, ExitUsage)
	}

	// A stale revision is a conflict-class failure.
	code, out, errw := runCLI(t, "note", "add", "edition", "--state-dir", dir, ed, "--text", "t")
	if code != ExitOK {
		t.Fatalf("note add: exit = %d, stderr = %q", code, errw)
	}
	noteID := noteEnv(t, out).Notes[0].ID
	code, out, errw = runCLI(t, "note", "revise", "--state-dir", dir, noteID, "--text", "stale", "--revision", "9")
	if code != ExitConflict {
		t.Fatalf("stale revision: exit = %d, want %d (stderr %q)", code, ExitConflict, errw)
	}

	// Prior state stays intact: the note is still at revision 1 and a valid
	// revise applies.
	code, out, errw = runCLI(t, "note", "revise", "--state-dir", dir, noteID, "--text", "valid", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("note revise after failures: exit = %d, stderr = %q", code, errw)
	}
	data := noteEnv(t, out)
	if len(data.Notes) != 1 || data.Notes[0].Revision != 2 || data.Notes[0].Text != "valid" {
		t.Fatalf("notes = %+v, want the valid revise at revision 2", data.Notes)
	}
}
