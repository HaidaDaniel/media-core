package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

type libEnvelope struct {
	OK   bool `json:"ok"`
	Data any  `json:"data"`

	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type listData struct {
	Items      []map[string]any `json:"items"`
	NextCursor *string          `json:"next_cursor"`
	Total      int64            `json:"total"`
}

func initLibState(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "state")
	code, _, errw := runCLI(t, "init", "--state-dir", dir)
	if code != ExitOK {
		t.Fatalf("init: exit = %d, stderr = %q", code, errw)
	}
	return dir
}

func libEnv(t *testing.T, out string) libEnvelope {
	t.Helper()
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("stdout must be exactly one JSON document, got %q", out)
	}
	var env libEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout is not the JSON envelope: %v\n%s", err, out)
	}
	return env
}

func listEnv(t *testing.T, out string) listData {
	t.Helper()
	env := libEnv(t, out)
	if !env.OK || env.Error != nil {
		t.Fatalf("list envelope = %+v, want ok", env)
	}
	b, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var data listData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("data is not a list payload: %v\n%s", err, out)
	}
	return data
}

func addWorkID(t *testing.T, dir, title string) string {
	t.Helper()
	code, out, errw := runCLI(t, "add", "--state-dir", dir, "--title", title)
	if code != ExitOK {
		t.Fatalf("add %q: exit = %d, stderr = %q", title, code, errw)
	}
	env := libEnv(t, out)
	b, _ := json.Marshal(env.Data)
	var created struct {
		WorkID string `json:"work_id"`
	}
	if err := json.Unmarshal(b, &created); err != nil || created.WorkID == "" {
		t.Fatalf("add data = %s, want a work id", out)
	}
	return created.WorkID
}

// TestCatalogListCLI proves the list command returns work cards with the
// distinct-work total and stable pagination.
func TestCatalogListCLI(t *testing.T) {
	dir := initLibState(t)
	workA := addWorkID(t, dir, "Alpha Story")
	addWorkID(t, dir, "Beta Story")
	workC := addWorkID(t, dir, "Gamma Story")

	code, out, errw := runCLI(t, "list", "--state-dir", dir, "--limit", "1")
	if code != ExitOK {
		t.Fatalf("list: exit = %d, stderr = %q", code, errw)
	}
	data := listEnv(t, out)
	if data.Total != 3 || len(data.Items) != 1 || data.NextCursor == nil {
		t.Fatalf("list = %+v, want total 3, one card and a cursor", data)
	}
	if data.Items[0]["work_id"] != workA {
		t.Fatalf("first card = %v, want alpha", data.Items[0])
	}

	code, out, errw = runCLI(t, "list", "--state-dir", dir, "--limit", "1", "--cursor", *data.NextCursor)
	if code != ExitOK {
		t.Fatalf("list page 2: exit = %d, stderr = %q", code, errw)
	}
	// The cursor points at the probe row (Beta), so page 2 holds only Gamma.
	page2 := listEnv(t, out)
	if page2.Total != 3 || len(page2.Items) != 1 || page2.Items[0]["work_id"] != workC {
		t.Fatalf("page 2 = %+v, want the gamma work only", page2)
	}
}

// TestCatalogListFiltersCLI proves query and kind filters pass through the
// CLI and unknown kinds are usage errors.
func TestCatalogListFiltersCLI(t *testing.T) {
	dir := initLibState(t)
	work := addWorkID(t, dir, "Filtered Story")

	code, out, _ := runCLI(t, "list", "--state-dir", dir, "--query", "filtered")
	if code != ExitOK {
		t.Fatalf("list query: exit = %d", code)
	}
	data := listEnv(t, out)
	if data.Total != 1 || data.Items[0]["work_id"] != work {
		t.Fatalf("query filter = %+v, want the one work", data)
	}

	code, out, _ = runCLI(t, "list", "--state-dir", dir, "--query", "zzz-not-present")
	if code != ExitOK {
		t.Fatalf("list no match: exit = %d", code)
	}
	if data = listEnv(t, out); data.Total != 0 || len(data.Items) != 0 {
		t.Fatalf("no-match list = %+v, want empty", data)
	}

	code, _, errw := runCLI(t, "list", "--state-dir", dir, "--kind", "hologram")
	if code != ExitUsage {
		t.Fatalf("unknown kind: exit = %d, stderr = %q, want %d", code, errw, ExitUsage)
	}
	code, _, errw = runCLI(t, "list", "--state-dir", dir, "--sort", "rating")
	if code != ExitUsage {
		t.Fatalf("unknown sort: exit = %d, stderr = %q, want %d", code, errw, ExitUsage)
	}
}

// TestCatalogShowCLI proves show returns the detail views and a missing id
// is a stable not-found with exit 2.
func TestCatalogShowCLI(t *testing.T) {
	dir := initLibState(t)
	work := addWorkID(t, dir, "Showable Story")

	code, out, _ := runCLI(t, "show", "--state-dir", dir, "work", work)
	if code != ExitOK {
		t.Fatalf("show work: exit = %d", code)
	}
	b, _ := json.Marshal(libEnv(t, out).Data)
	var view struct {
		ID     string `json:"id"`
		Titles []struct {
			Text string `json:"text"`
		} `json:"titles"`
		Editions []any `json:"editions"`
	}
	if err := json.Unmarshal(b, &view); err != nil {
		t.Fatalf("work view: %v\n%s", err, out)
	}
	if view.ID != work || len(view.Titles) != 1 || view.Titles[0].Text != "Showable Story" {
		t.Fatalf("work view = %+v, want the work with its title", view)
	}
	if view.Editions == nil {
		t.Fatal("work view editions = nil, want an empty collection")
	}

	code, out, _ = runCLI(t, "show", "--state-dir", dir, "work", "no-such-work")
	if code != ExitConflict {
		t.Fatalf("show missing work: exit = %d, want %d", code, ExitConflict)
	}
	env := libEnv(t, out)
	if env.OK || env.Error == nil || env.Error.Code != "not_found" {
		t.Fatalf("missing work = %+v, want not_found", env)
	}

	code, out, _ = runCLI(t, "show", "--state-dir", dir, "edition", "no-such-edition")
	if code != ExitConflict || libEnv(t, out).Error == nil || libEnv(t, out).Error.Code != "not_found" {
		t.Fatalf("show missing edition: exit = %d, out = %s, want not_found", code, out)
	}

	code, _, errw := runCLI(t, "show", "--state-dir", dir, "season")
	if code != ExitUsage {
		t.Fatalf("show unknown target: exit = %d, stderr = %q, want %d", code, errw, ExitUsage)
	}
}

// TestLibraryTitleCLI proves the title correction commands: add, retire and
// prefer, with the stale revision as a distinguishable conflict.
func TestLibraryTitleCLI(t *testing.T) {
	dir := initLibState(t)
	work := addWorkID(t, dir, "Title Story")

	// Read the current revision and title id from the work view.
	code, out, _ := runCLI(t, "show", "--state-dir", dir, "work", work)
	if code != ExitOK {
		t.Fatalf("show: exit = %d", code)
	}
	b, _ := json.Marshal(libEnv(t, out).Data)
	var view struct {
		Revision int64 `json:"revision"`
		Titles   []struct {
			ID string `json:"id"`
		} `json:"titles"`
	}
	if err := json.Unmarshal(b, &view); err != nil {
		t.Fatalf("work view: %v", err)
	}
	originalID := view.Titles[0].ID

	// Add a title.
	code, out, errw := runCLI(t, "title", "--state-dir", dir, "work", "add", work,
		"--revision", "0", "--text", "Title Corrected", "--provenance", "manual",
		"--request-id", "cli-title-add")
	if code != ExitOK {
		t.Fatalf("title add: exit = %d, stderr = %q", code, errw)
	}
	b, _ = json.Marshal(libEnv(t, out).Data)
	var afterAdd struct {
		Revision int64 `json:"revision"`
		Titles   []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"titles"`
	}
	if err := json.Unmarshal(b, &afterAdd); err != nil {
		t.Fatalf("title add view: %v", err)
	}
	var addedID string
	if afterAdd.Revision != 1 || len(afterAdd.Titles) != 2 {
		t.Fatalf("after add = %+v, want two titles at revision 1", afterAdd)
	}
	for _, title := range afterAdd.Titles {
		if title.Text == "Title Corrected" {
			addedID = title.ID
		}
	}
	if addedID == "" {
		t.Fatalf("added title id missing in %+v", afterAdd)
	}

	// Prefer the new title.
	code, out, errw = runCLI(t, "title", "--state-dir", dir, "work", "prefer", work,
		"--revision", "1", "--title-id", addedID, "--request-id", "cli-title-prefer")
	if code != ExitOK {
		t.Fatalf("title prefer: exit = %d, stderr = %q", code, errw)
	}

	// Retire the original.
	code, out, errw = runCLI(t, "title", "--state-dir", dir, "work", "retire", work,
		"--revision", "2", "--title-id", originalID, "--request-id", "cli-title-retire")
	if code != ExitOK {
		t.Fatalf("title retire: exit = %d, stderr = %q", code, errw)
	}
	b, _ = json.Marshal(libEnv(t, out).Data)
	var afterRetire struct {
		Titles []struct {
			Text string `json:"text"`
		} `json:"titles"`
	}
	if err := json.Unmarshal(b, &afterRetire); err != nil {
		t.Fatalf("title retire view: %v", err)
	}
	if len(afterRetire.Titles) != 1 || afterRetire.Titles[0].Text != "Title Corrected" {
		t.Fatalf("after retire = %+v, want only the corrected title", afterRetire)
	}

	// A stale revision is a conflict with exit 2.
	code, out, _ = runCLI(t, "title", "--state-dir", dir, "work", "retire", work,
		"--revision", "0", "--title-id", addedID, "--request-id", "cli-title-stale")
	if code != ExitConflict {
		t.Fatalf("stale title retire: exit = %d, want %d", code, ExitConflict)
	}
	env := libEnv(t, out)
	if env.Error == nil || env.Error.Code != "conflict" {
		t.Fatalf("stale retire = %+v, want conflict", env)
	}

	// Retiring the last remaining title fails validation.
	code, out, _ = runCLI(t, "title", "--state-dir", dir, "work", "retire", work,
		"--revision", "3", "--title-id", addedID, "--request-id", "cli-title-last")
	if code != ExitConflict || libEnv(t, out).Error == nil || libEnv(t, out).Error.Code != "invalid_argument" {
		t.Fatalf("retire last title: exit = %d, out = %s, want invalid_argument", code, out)
	}
}

// TestLibraryEditionCLI proves edition metadata updates, archive and
// unarchive through the CLI.
func TestLibraryEditionCLI(t *testing.T) {
	dir := initLibState(t)
	work := addWorkID(t, dir, "Edition Story")
	// Probe: resolving the same title and kind surfaces the existing work
	// with a candidate token.
	code, out, _ := runCLI(t, "add", "--state-dir", dir,
		"--title", "Edition Story", "--kind", "manga")
	if code != ExitOK {
		t.Fatalf("probe add: exit = %d, stderr = %q", code, out)
	}
	var probe struct {
		OK   bool `json:"ok"`
		Data struct {
			Outcome        string  `json:"outcome"`
			CandidateToken *string `json:"candidate_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &probe); err != nil || !probe.OK {
		t.Fatalf("probe add = %s, want candidates", out)
	}
	if probe.Data.Outcome != "candidates" || probe.Data.CandidateToken == nil {
		t.Fatalf("probe add = %s, want a candidate token", out)
	}
	code, out, _ = runCLI(t, "add", "--state-dir", dir,
		"--title", "Edition Story", "--kind", "manga", "--work-id", work,
		"--choice", "add_edition", "--candidate-token", *probe.Data.CandidateToken,
		"--request-id", "cli-edition-add")
	if code != ExitOK {
		t.Fatalf("add edition: exit = %d, stderr = %q", code, out)
	}
	b, _ := json.Marshal(libEnv(t, out).Data)
	var created struct {
		EditionID *string `json:"edition_id"`
	}
	if err := json.Unmarshal(b, &created); err != nil || created.EditionID == nil {
		t.Fatalf("add edition data = %s, want an edition id", out)
	}
	edition := *created.EditionID

	// Update the kind.
	code, out, errw := runCLI(t, "edition", "--state-dir", dir, "update", edition,
		"--revision", "0", "--kind", "anime", "--request-id", "cli-edition-update")
	if code != ExitOK {
		t.Fatalf("edition update: exit = %d, stderr = %q", code, errw)
	}
	b, _ = json.Marshal(libEnv(t, out).Data)
	var updated struct {
		Kind     string `json:"kind"`
		Revision int64  `json:"revision"`
	}
	if err := json.Unmarshal(b, &updated); err != nil {
		t.Fatalf("edition update view: %v", err)
	}
	if updated.Kind != "anime" || updated.Revision != 1 {
		t.Fatalf("updated edition = %+v, want anime at revision 1", updated)
	}

	// Archive.
	code, out, errw = runCLI(t, "edition", "--state-dir", dir, "archive", edition,
		"--revision", "1", "--request-id", "cli-edition-archive")
	if code != ExitOK {
		t.Fatalf("edition archive: exit = %d, stderr = %q", code, errw)
	}
	b, _ = json.Marshal(libEnv(t, out).Data)
	var archived struct {
		Revision int64 `json:"revision"`
		Titles   []any `json:"titles"`
	}
	if err := json.Unmarshal(b, &archived); err != nil {
		t.Fatalf("edition archive view: %v", err)
	}
	if archived.Revision != 2 || len(archived.Titles) != 1 {
		t.Fatalf("archived edition = %+v, want the title preserved at revision 2", archived)
	}

	// Unarchive.
	code, out, errw = runCLI(t, "edition", "--state-dir", dir, "unarchive", edition,
		"--revision", "2", "--request-id", "cli-edition-unarchive")
	if code != ExitOK {
		t.Fatalf("edition unarchive: exit = %d, stderr = %q", code, errw)
	}

	// A stale revision conflicts.
	code, out, _ = runCLI(t, "edition", "--state-dir", dir, "update", edition,
		"--revision", "0", "--kind", "manga", "--request-id", "cli-edition-stale")
	if code != ExitConflict || libEnv(t, out).Error == nil || libEnv(t, out).Error.Code != "conflict" {
		t.Fatalf("stale edition update: exit = %d, out = %s, want conflict", code, out)
	}

	// Update without any field is a usage error.
	code, _, errw = runCLI(t, "edition", "--state-dir", dir, "update", edition, "--revision", "3")
	if code != ExitUsage {
		t.Fatalf("empty edition update: exit = %d, stderr = %q, want %d", code, errw, ExitUsage)
	}
}

// TestLibraryListCursorCLI proves a cursor from a different listing is
// rejected with exit 2.
func TestLibraryListCursorCLI(t *testing.T) {
	dir := initLibState(t)
	for _, title := range []string{"One", "Two", "Three"} {
		addWorkID(t, dir, title)
	}
	code, out, _ := runCLI(t, "list", "--state-dir", dir, "--limit", "1")
	if code != ExitOK {
		t.Fatalf("list: exit = %d", code)
	}
	cursor := listEnv(t, out).NextCursor
	if cursor == nil {
		t.Fatal("list: want a next cursor")
	}
	// Same filter, different limit: the cursor no longer applies.
	code, out, _ = runCLI(t, "list", "--state-dir", dir, "--limit", "2", "--cursor", *cursor)
	if code != ExitConflict {
		t.Fatalf("cursor with different limit: exit = %d, want %d", code, ExitConflict)
	}
	env := libEnv(t, out)
	if env.Error == nil || env.Error.Code != "invalid_argument" {
		t.Fatalf("cursor mismatch = %+v, want invalid_argument", env)
	}
}
