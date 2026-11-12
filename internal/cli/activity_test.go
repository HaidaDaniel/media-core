package cli

import (
	"strings"
	"testing"
)

// TestActivityCLI exercises the activity command surface: the recent feed
// with the type filter and cursor pagination, the entity histories, the
// stable failure codes and the usage errors.
func TestActivityCLI(t *testing.T) {
	dir := initLibState(t)
	workID, editionID := addWorkWithEdition(t, dir, "Alpha Story")

	// One consumption row and one note row on the edition.
	code, out, errw := runCLI(t, "status", "set", editionID, "--state-dir", dir, "--value", "in_progress", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("status set: exit = %d, stderr = %q", code, errw)
	}
	code, out, errw = runCLI(t, "note", "add", "edition", editionID, "--state-dir", dir, "--text", "A note")
	if code != ExitOK {
		t.Fatalf("note add: exit = %d, stderr = %q", code, errw)
	}

	// The recent feed carries both rows, newest first.
	code, out, errw = runCLI(t, "activity", "recent", "--state-dir", dir)
	if code != ExitOK {
		t.Fatalf("activity recent: exit = %d, stderr = %q", code, errw)
	}
	data := listEnv(t, out)
	// The add command seeds the initial not_started observation, so the
	// feed holds the note, the in_progress and the not_started rows.
	if data.Total != 3 || len(data.Items) != 3 {
		t.Fatalf("recent = %+v, want 3 of 3", data)
	}
	if data.Items[0]["type"] != "note" {
		t.Fatalf("first recent item = %v, want the note (newest)", data.Items[0])
	}
	if data.Items[1]["type"] != "consumption" || data.Items[1]["value"] != "in_progress" {
		t.Fatalf("second recent item = %v, want in_progress", data.Items[1])
	}

	// The type filter keeps one row; the cursor paginates one at a time.
	code, out, errw = runCLI(t, "activity", "recent", "--state-dir", dir, "--type", "consumption", "--limit", "1")
	if code != ExitOK {
		t.Fatalf("activity recent filtered: exit = %d, stderr = %q", code, errw)
	}
	data = listEnv(t, out)
	if data.Total != 2 || len(data.Items) != 1 || data.NextCursor == nil {
		t.Fatalf("filtered recent = %+v, want 1 of 2 with a cursor", data)
	}
	code, out, errw = runCLI(t, "activity", "recent", "--state-dir", dir, "--limit", "1")
	if code != ExitOK {
		t.Fatalf("activity recent paged: exit = %d, stderr = %q", code, errw)
	}
	data = listEnv(t, out)
	if data.Total != 3 || len(data.Items) != 1 || data.NextCursor == nil {
		t.Fatalf("paged recent = %+v, want 1 of 3 with a cursor", data)
	}
	code, out, errw = runCLI(t, "activity", "recent", "--state-dir", dir, "--limit", "1", "--cursor", *data.NextCursor)
	if code != ExitOK {
		t.Fatalf("activity recent page 2: exit = %d, stderr = %q", code, errw)
	}
	data = listEnv(t, out)
	if data.Total != 3 || len(data.Items) != 1 {
		t.Fatalf("recent page 2 = %+v, want 1 of 3", data)
	}
	if data.Items[0]["type"] == "note" {
		t.Fatalf("recent page 2 = %+v, want the consumption (the note was page 1)", data)
	}

	// The histories span the entity's rows.
	code, out, errw = runCLI(t, "activity", "history", "edition", editionID, "--state-dir", dir)
	if code != ExitOK {
		t.Fatalf("activity history edition: exit = %d, stderr = %q", code, errw)
	}
	data = listEnv(t, out)
	if data.Total != 3 {
		t.Fatalf("edition history = %+v, want 3 of 3", data)
	}
	code, out, errw = runCLI(t, "activity", "history", "work", workID, "--state-dir", dir)
	if code != ExitOK {
		t.Fatalf("activity history work: exit = %d, stderr = %q", code, errw)
	}
	data = listEnv(t, out)
	if data.Total != 3 {
		t.Fatalf("work history = %+v, want 3 of 3", data)
	}

	// A missing entity is a stable not-found with exit 2.
	code, out, errw = runCLI(t, "activity", "history", "work", "00000000-0000-4000-8000-000000000000", "--state-dir", dir)
	if code != ExitConflict {
		t.Fatalf("missing work: exit = %d, want %d", code, ExitConflict)
	}
	env := libEnv(t, out)
	if env.OK || env.Error == nil || env.Error.Code != "not_found" {
		t.Fatalf("missing work envelope = %+v, want not_found", env)
	}
	// An unknown type and an out-of-bounds limit are invalid_argument.
	code, out, errw = runCLI(t, "activity", "recent", "--state-dir", dir, "--type", "nope")
	if code != ExitConflict {
		t.Fatalf("unknown type: exit = %d, want %d", code, ExitConflict)
	}
	if env := libEnv(t, out); env.OK || env.Error == nil || env.Error.Code != "invalid_argument" {
		t.Fatalf("unknown type envelope = %+v, want invalid_argument", env)
	}
	code, out, errw = runCLI(t, "activity", "recent", "--state-dir", dir, "--limit", "1000")
	if code != ExitConflict {
		t.Fatalf("limit over bound: exit = %d, want %d", code, ExitConflict)
	}
	if env := libEnv(t, out); env.OK || env.Error == nil || env.Error.Code != "invalid_argument" {
		t.Fatalf("limit envelope = %+v, want invalid_argument", env)
	}
	// Usage errors stay usage errors.
	for _, args := range [][]string{
		{"activity"},
		{"activity", "bogus", "--state-dir", dir},
		{"activity", "history", "season", "x", "--state-dir", dir},
		{"activity", "recent", "--state-dir", dir, "--nope"},
	} {
		full := append([]string{}, args...)
		code, out, errw = runCLI(t, full...)
		if code != ExitUsage {
			t.Fatalf("activity %v: exit = %d, want %d (stderr %q)", args, code, ExitUsage, errw)
		}
		if !strings.Contains(errw, "media-core activity") {
			t.Fatalf("activity %v: want the activity usage hint, got %q", args, errw)
		}
	}
}
