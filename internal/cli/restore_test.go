package cli

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type restoreData struct {
	StateDir string `json:"state_dir"`
	Schema   int    `json:"schema"`
	DB       struct {
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
	} `json:"db"`
}

func restoreDataFrom(t *testing.T, out string) restoreData {
	t.Helper()
	env := libEnv(t, out)
	if !env.OK || env.Error != nil {
		t.Fatalf("envelope = %+v, want ok", env)
	}
	b, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var data restoreData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("data is not a restore payload: %v\n%s", err, out)
	}
	return data
}

// addWorkWithEditionID seeds one work and edition with an explicit request
// id so the round-trip can replay it against the restored state.
func addWorkWithEditionID(t *testing.T, dir, title, requestID string) (string, string) {
	t.Helper()
	code, out, errw := runCLI(t, "add", "--state-dir", dir, "--request-id", requestID, "--title", title, "--kind", "manga")
	if code != ExitOK {
		t.Fatalf("add %q: exit = %d, stderr = %q", title, code, errw)
	}
	env := libEnv(t, out)
	b, _ := json.Marshal(env.Data)
	var created struct {
		WorkID    string  `json:"work_id"`
		EditionID *string `json:"edition_id"`
	}
	if err := json.Unmarshal(b, &created); err != nil || created.EditionID == nil {
		t.Fatalf("add data = %s, want a work and an edition id", out)
	}
	return created.WorkID, *created.EditionID
}

// listTitles returns the work card titles of the first list page.
func listTitles(t *testing.T, stateDir, workID string) []string {
	t.Helper()
	code, out, errw := runCLI(t, "list", "--state-dir", stateDir)
	if code != ExitOK {
		t.Fatalf("list: exit = %d, stderr = %q", code, errw)
	}
	data := listEnv(t, out)
	if data.Total != 1 || len(data.Items) != 1 {
		t.Fatalf("list = %+v, want exactly one work", data)
	}
	card := data.Items[0]
	if card["work_id"] != workID {
		t.Fatalf("work_id = %v, want %q", card["work_id"], workID)
	}
	rawTitles, _ := card["titles"].([]any)
	var titles []string
	for _, rt := range rawTitles {
		title, _ := rt.(map[string]any)
		text, _ := title["text"].(string)
		titles = append(titles, text)
	}
	return titles
}

// TestRestoreCLI runs the M1 subset scenario F round trip: a state with a
// work, its titles, an edition and progress plus the request journal is
// archived and restored into a fresh state directory, where the
// application-visible state must match, including an idempotent replay of
// the journaled request.
func TestRestoreCLI(t *testing.T) {
	src := initLibState(t)
	workID, editionID := addWorkWithEditionID(t, src, "Restore Fixture", "restore-req-1")
	code, out, errw := runCLI(t, "progress", "set", "--state-dir", src, editionID, "--unit", "chapter", "--position", "42", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("progress: exit = %d, stderr = %q", code, errw)
	}

	backupDir := filepath.Join(t.TempDir(), "backups")
	code, out, errw = runCLI(t, "backup", "create", "--state-dir", src, "--dest", backupDir, "--name", "fixture")
	if code != ExitOK {
		t.Fatalf("backup: exit = %d, stderr = %q", code, errw)
	}
	archive := backupDataFrom(t, out).Archive

	dest := filepath.Join(t.TempDir(), "state-dest")
	code, out, errw = runCLI(t, "restore", archive, "--state-dir", dest)
	if code != ExitOK {
		t.Fatalf("restore: exit = %d, stderr = %q", code, errw)
	}
	data := restoreDataFrom(t, out)
	if data.StateDir != dest || data.Schema <= 0 || data.DB.Name != "media-core.db" || data.DB.Size <= 0 {
		t.Fatalf("restore payload = %+v, want state dir, schema and db", data)
	}
	if ok, _ := regexp.MatchString(`^[0-9a-f]{64}$`, data.DB.SHA256); !ok {
		t.Fatalf("sha256 = %q, want 64 hex chars", data.DB.SHA256)
	}

	// The application-visible state matches in the restored destination.
	var titles []string
	for _, title := range listTitles(t, dest, workID) {
		titles = append(titles, title)
	}
	found := false
	for _, title := range titles {
		if title == "Restore Fixture" {
			found = true
		}
	}
	if !found {
		t.Fatalf("restored titles = %v, want Restore Fixture", titles)
	}
	code, out, errw = runCLI(t, "show", "edition", editionID, "--state-dir", dest)
	if code != ExitOK {
		t.Fatalf("show edition: exit = %d, stderr = %q", code, errw)
	}
	var view struct {
		Progress struct {
			Chapter *struct {
				Position string `json:"position"`
			} `json:"chapter"`
		} `json:"progress"`
	}
	env := libEnv(t, out)
	b, _ := json.Marshal(env.Data)
	if err := json.Unmarshal(b, &view); err != nil {
		t.Fatalf("show edition data: %v\n%s", err, out)
	}
	if view.Progress.Chapter == nil || view.Progress.Chapter.Position != "42" {
		t.Fatalf("restored progress = %+v, want chapter position 42", view.Progress)
	}

	// The journaled request key survived the round trip: the same key with
	// a different payload is a key-collision conflict, and no entry is
	// created.
	code, out, errw = runCLI(t, "add", "--state-dir", dest, "--request-id", "restore-req-1", "--title", "Other Fixture", "--kind", "manga")
	if code != ExitConflict {
		t.Fatalf("same-key different-payload add: exit = %d, want %d, stderr = %q", code, ExitConflict, errw)
	}
	env = libEnv(t, out)
	if env.Error == nil || env.Error.Code != "conflict" {
		t.Fatalf("collision envelope = %+v, want code conflict", env)
	}
	if data := listEnv(t, runCLIOut(t, "list", "--state-dir", dest)); data.Total != 1 {
		t.Fatalf("list after collision = %+v, want one work", data)
	}

	// An existing destination needs explicit force; force replaces.
	code, out, errw = runCLI(t, "restore", archive, "--state-dir", dest)
	if code != ExitConflict {
		t.Fatalf("restore without force: exit = %d, want %d, stderr = %q", code, ExitConflict, errw)
	}
	env = libEnv(t, out)
	if env.Error == nil || env.Error.Code != "conflict" {
		t.Fatalf("restore without force envelope = %+v, want code conflict", env)
	}
	code, out, errw = runCLI(t, "restore", archive, "--state-dir", dest, "--force")
	if code != ExitOK {
		t.Fatalf("restore with force: exit = %d, stderr = %q", code, errw)
	}
	if got := listEnv(t, runCLIOut(t, "list", "--state-dir", dest)).Total; got != 1 {
		t.Fatalf("list after forced restore = %d works, want 1", got)
	}

	// Usage errors keep the usage exit code.
	for _, args := range [][]string{
		{"restore"},
		{"restore", "a", "b"},
		{"restore", archive, "--state-dir"},
	} {
		full := append([]string{}, args...)
		code, _, errw = runCLI(t, full...)
		if code != ExitUsage {
			t.Fatalf("%v: exit = %d, want %d, stderr = %q", args, code, ExitUsage, errw)
		}
		if !strings.Contains(errw, "media-core restore") {
			t.Fatalf("%v: stderr = %q, want the restore usage hint", args, errw)
		}
	}
}

func runCLIOut(t *testing.T, args ...string) string {
	t.Helper()
	code, out, errw := runCLI(t, args...)
	if code != ExitOK {
		t.Fatalf("%v: exit = %d, stderr = %q", args, code, errw)
	}
	return out
}
