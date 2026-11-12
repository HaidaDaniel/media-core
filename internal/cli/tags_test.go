package cli

import (
	"encoding/json"
	"testing"
)

type tagData struct {
	Revision int64    `json:"revision"`
	Tags     []string `json:"tags"`
}

func tagEnv(t *testing.T, out string) tagData {
	t.Helper()
	env := libEnv(t, out)
	if !env.OK || env.Error != nil {
		t.Fatalf("tag envelope = %+v, want ok", env)
	}
	b, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var data tagData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("data is not a view: %v\n%s", err, out)
	}
	return data
}

// TestTagAttachDetachCLI proves the tag command attaches and detaches on
// both targets, normalizes the name, and prints the updated view.
func TestTagAttachDetachCLI(t *testing.T) {
	dir := initLibState(t)
	work, ed := addWorkWithEdition(t, dir, "Tag Story")

	// The work starts at revision 1: adding the edition bumped it.
	code, out, errw := runCLI(t, "tag", "attach", "work", "--state-dir", dir, work, "--tag", "  Favorite ", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("tag attach work: exit = %d, stderr = %q", code, errw)
	}
	data := tagEnv(t, out)
	if data.Revision != 2 {
		t.Fatalf("work revision = %d, want 2", data.Revision)
	}

	code, out, errw = runCLI(t, "tag", "attach", "edition", "--state-dir", dir, ed, "--tag", "FAVORITE", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("tag attach edition: exit = %d, stderr = %q", code, errw)
	}
	data = tagEnv(t, out)
	if data.Revision != 1 || len(data.Tags) != 1 || data.Tags[0] != "favorite" {
		t.Fatalf("view = %+v, want [favorite] at revision 1", data)
	}

	// A repeated attach is a no-op.
	code, out, errw = runCLI(t, "tag", "attach", "edition", "--state-dir", dir, ed, "--tag", "favorite", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("tag attach no-op: exit = %d, stderr = %q", code, errw)
	}
	data = tagEnv(t, out)
	if data.Revision != 1 {
		t.Fatalf("revision = %d, want 1 after the no-op", data.Revision)
	}

	code, out, errw = runCLI(t, "tag", "detach", "edition", "--state-dir", dir, ed, "--tag", "favorite", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("tag detach: exit = %d, stderr = %q", code, errw)
	}
	data = tagEnv(t, out)
	if data.Revision != 2 || len(data.Tags) != 0 {
		t.Fatalf("view = %+v, want no tags at revision 2", data)
	}
}

// TestTagCLIInvalid proves the CLI rejects unknown verbs and targets and
// stale revisions with the contract exit codes.
func TestTagCLIInvalid(t *testing.T) {
	dir := initLibState(t)
	work, ed := addWorkWithEdition(t, dir, "Invalid Tags")

	if code, _, _ := runCLI(t, "tag", "rename", "work", "--state-dir", dir, work, "--tag", "favorite"); code != ExitUsage {
		t.Fatalf("unknown verb: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "tag", "attach", "series", "--state-dir", dir, work, "--tag", "favorite"); code != ExitUsage {
		t.Fatalf("unknown target: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "tag", "attach", "work", "--state-dir", dir, work, "--tag", "favorite", "--revision", "9"); code != ExitUsage {
		t.Fatalf("stale revision: exit = %d, want %d", code, ExitUsage)
	}

	// Prior state stays intact: the first valid attach still applies.
	code, out, errw := runCLI(t, "tag", "attach", "work", "--state-dir", dir, work, "--tag", "favorite", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("tag attach after failures: exit = %d, stderr = %q", code, errw)
	}
	data := tagEnv(t, out)
	if data.Revision != 2 {
		t.Fatalf("work revision = %d, want 2", data.Revision)
	}
	_ = ed
}

// TestListIntentionAndTagCLI proves the shared list filters expose the new
// dimensions: the saved intention filter picks the tagged-and-intended work
// and not the read-later-tagged work without an intention.
func TestListIntentionAndTagCLI(t *testing.T) {
	dir := initLibState(t)
	workSaved, edSaved := addWorkWithEdition(t, dir, "Saved Story")
	workLater, _ := addWorkWithEdition(t, dir, "Later Story")

	mustCLI(t, "intention", "set", "--state-dir", dir, edSaved, "--value", "saved", "--revision", "0")
	mustCLI(t, "tag", "attach", "edition", "--state-dir", dir, edSaved, "--tag", "favorite", "--revision", "1")
	// The work starts at revision 1: adding the edition bumped it.
	mustCLI(t, "tag", "attach", "work", "--state-dir", dir, workLater, "--tag", "read-later", "--revision", "1")

	// Saved picks only the saved work: the read-later tag alone is not an
	// intention.
	code, out, errw := runCLI(t, "list", "--state-dir", dir, "--intention", "saved", "--limit", "10")
	if code != ExitOK {
		t.Fatalf("list saved: exit = %d, stderr = %q", code, errw)
	}
	if !listContains(t, out, workSaved) || listContains(t, out, workLater) {
		t.Fatalf("saved listing does not contain exactly the saved work:\n%s", out)
	}

	// The tag filter picks the work through the edition tag.
	code, out, errw = runCLI(t, "list", "--state-dir", dir, "--tag", "favorite", "--limit", "10")
	if code != ExitOK {
		t.Fatalf("list favorite: exit = %d, stderr = %q", code, errw)
	}
	if !listContains(t, out, workSaved) {
		t.Fatalf("favorite listing misses the tagged work:\n%s", out)
	}

	// The work tag filter picks the work through its own tag.
	code, out, errw = runCLI(t, "list", "--state-dir", dir, "--tag", "read-later", "--limit", "10")
	if code != ExitOK {
		t.Fatalf("list read-later: exit = %d, stderr = %q", code, errw)
	}
	if !listContains(t, out, workLater) {
		t.Fatalf("read-later listing misses the work-tagged work:\n%s", out)
	}
}

func mustCLI(t *testing.T, args ...string) {
	t.Helper()
	code, out, errw := runCLI(t, args...)
	if code != ExitOK {
		t.Fatalf("%v: exit = %d, stderr = %q, out = %q", args, code, errw, out)
	}
}

func listContains(t *testing.T, out string, workID string) bool {
	t.Helper()
	var payload struct {
		Data struct {
			Items []json.RawMessage `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("listing is not an envelope: %v\n%s", err, out)
	}
	for _, item := range payload.Data.Items {
		var card struct {
			WorkID string `json:"work_id"`
		}
		if err := json.Unmarshal(item, &card); err != nil {
			t.Fatalf("item is not a work card: %v\n%s", err, item)
		}
		if card.WorkID == workID {
			return true
		}
	}
	return false
}
