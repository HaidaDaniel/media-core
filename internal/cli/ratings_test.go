package cli

import (
	"encoding/json"
	"testing"
)

type ratingData struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
	Consumption *struct {
		Value string `json:"value"`
	} `json:"consumption"`
	Rating *struct {
		Value        *int    `json:"value"`
		Comment      *string `json:"comment"`
		SupersedesID *string `json:"supersedes_id"`
	} `json:"rating"`
}

func ratingEnv(t *testing.T, out string) ratingData {
	t.Helper()
	env := libEnv(t, out)
	if !env.OK || env.Error != nil {
		t.Fatalf("rate envelope = %+v, want ok", env)
	}
	b, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var data ratingData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("data is not an edition view: %v\n%s", err, out)
	}
	return data
}

// TestRatingCLI proves the rate command: a value renders in the edition view
// without touching consumption, a change appends a new current rating, and
// the explicit clear renders as absence.
func TestRatingCLI(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Rating Story")

	code, out, errw := runCLI(t, "rate", "--state-dir", dir, ed, "--value", "8",
		"--comment", "great", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("rate 8: exit = %d, stderr = %q", code, errw)
	}
	data := ratingEnv(t, out)
	if data.Revision != 1 {
		t.Fatalf("revision = %d, want 1", data.Revision)
	}
	if data.Rating == nil || data.Rating.Value == nil || *data.Rating.Value != 8 ||
		data.Rating.Comment == nil || *data.Rating.Comment != "great" {
		t.Fatalf("rating = %+v, want 8 with the comment", data.Rating)
	}
	// A rating never alters the consumption state: the added edition keeps
	// its default not_started status.
	if data.Consumption == nil || data.Consumption.Value != "not_started" {
		t.Fatalf("consumption = %v, want the untouched default not_started", data.Consumption)
	}

	// The change appends a new current rating.
	code, out, errw = runCLI(t, "rate", "--state-dir", dir, ed, "--value", "2",
		"--comment", "disappointed", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("rate 2: exit = %d, stderr = %q", code, errw)
	}
	data = ratingEnv(t, out)
	if data.Revision != 2 || data.Rating == nil || data.Rating.Value == nil || *data.Rating.Value != 2 {
		t.Fatalf("view = %+v, want 2 at revision 2", data)
	}

	// The explicit clear renders as absence, never zero.
	code, out, errw = runCLI(t, "rate", "--state-dir", dir, ed, "--clear", "--revision", "2")
	if code != ExitOK {
		t.Fatalf("rate clear: exit = %d, stderr = %q", code, errw)
	}
	data = ratingEnv(t, out)
	if data.Revision != 3 || data.Rating != nil {
		t.Fatalf("view = %+v, want the clear rendered as absence at revision 3", data)
	}
}

// TestRatingCLIInvalid proves the CLI rejects fractional, out-of-bounds and
// ambiguous input with the contract exit codes and a stale revision is a
// conflict that leaves the prior state intact.
func TestRatingCLIInvalid(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Invalid Ratings")

	if code, _, _ := runCLI(t, "rate", "--state-dir", dir, ed, "--value", "8.5", "--revision", "0"); code != ExitUsage {
		t.Fatalf("fractional value: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "rate", "--state-dir", dir, ed, "--value", "0", "--revision", "0"); code != ExitUsage {
		t.Fatalf("value 0: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "rate", "--state-dir", dir, ed, "--value", "11", "--revision", "0"); code != ExitUsage {
		t.Fatalf("value 11: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "rate", "--state-dir", dir, ed); code != ExitUsage {
		t.Fatalf("missing value and clear: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "rate", "--state-dir", dir, ed, "--value", "5", "--clear", "--revision", "0"); code != ExitUsage {
		t.Fatalf("value and clear together: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "rate", "--state-dir", dir, ed, "--clear", "--comment", "x", "--revision", "0"); code != ExitUsage {
		t.Fatalf("clear with a comment: exit = %d, want %d", code, ExitUsage)
	}

	// A valid rating applies on the fresh revision.
	code, out, errw := runCLI(t, "rate", "--state-dir", dir, ed, "--value", "7", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("rate 7: exit = %d, stderr = %q", code, errw)
	}

	// A stale revision is a conflict-class failure and leaves the prior
	// state intact.
	code, _, errw = runCLI(t, "rate", "--state-dir", dir, ed, "--value", "9", "--revision", "0")
	if code != ExitConflict {
		t.Fatalf("stale revision: exit = %d, want %d (stderr %q)", code, ExitConflict, errw)
	}
	code, out, errw = runCLI(t, "rate", "--state-dir", dir, ed, "--value", "6", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("rate after the conflict: exit = %d, stderr = %q", code, errw)
	}
	data := ratingEnv(t, out)
	if data.Rating == nil || data.Rating.Value == nil || *data.Rating.Value != 6 {
		t.Fatalf("rating = %+v, want 6 (the prior state stayed intact)", data.Rating)
	}
}

// TestRatingFilterAndSortCLI proves list --rating matches only editions whose
// current rating is at least the value (unrated and cleared never match)
// and list --sort rating orders rated works first with missing ratings
// last.
func TestRatingFilterAndSortCLI(t *testing.T) {
	dir := initLibState(t)
	w8, ed8 := addWorkWithEdition(t, dir, "Eight Story")
	w5, ed5 := addWorkWithEdition(t, dir, "Five Story")
	wU, _ := addWorkWithEdition(t, dir, "Unrated Story")
	wC, edC := addWorkWithEdition(t, dir, "Cleared Story")

	mustRate := func(t *testing.T, ed string, value, revision string) {
		t.Helper()
		code, _, errw := runCLI(t, "rate", "--state-dir", dir, ed, "--value", value, "--revision", revision)
		if code != ExitOK {
			t.Fatalf("rate %s: exit = %d, stderr = %q", value, code, errw)
		}
	}
	mustRate(t, ed8, "8", "0")
	mustRate(t, ed5, "5", "0")
	mustRate(t, edC, "3", "0")
	code, out, errw := runCLI(t, "rate", "--state-dir", dir, edC, "--clear", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("rate clear: exit = %d, stderr = %q", code, errw)
	}

	// Minimum 5 picks the eight and five works; unrated and cleared never
	// match.
	code, out, errw = runCLI(t, "list", "--state-dir", dir, "--rating", "5", "--limit", "10")
	if code != ExitOK {
		t.Fatalf("list --rating 5: exit = %d, stderr = %q", code, errw)
	}
	data := listEnv(t, out)
	if data.Total != 2 || len(data.Items) != 2 {
		t.Fatalf("minimum 5 list = %+v, want the eight and five works only", data)
	}
	for _, item := range data.Items {
		if w := item["work_id"].(string); w != w8 && w != w5 {
			t.Fatalf("minimum 5 list contains %s, want only the rated works", w)
		}
	}

	// A non-numeric minimum rating is a usage error; an out-of-bounds
	// minimum rating is a stable invalid-argument failure.
	if code, _, _ := runCLI(t, "list", "--state-dir", dir, "--rating", "high"); code != ExitUsage {
		t.Fatalf("non-numeric rating: exit = %d, want %d", code, ExitUsage)
	}
	code, out, errw = runCLI(t, "list", "--state-dir", dir, "--rating", "11")
	if code != ExitUsage {
		t.Fatalf("rating 11: exit = %d, want %d", code, ExitUsage)
	}
	env := libEnv(t, out)
	if env.OK || env.Error == nil || env.Error.Code != "invalid_argument" {
		t.Fatalf("rating 11 envelope = %+v, want invalid_argument", env)
	}

	// The rating sort orders by the best current rating, missing ratings
	// last.
	code, out, errw = runCLI(t, "list", "--state-dir", dir, "--sort", "rating", "--limit", "10")
	if code != ExitOK {
		t.Fatalf("list --sort rating: exit = %d, stderr = %q", code, errw)
	}
	data = listEnv(t, out)
	if len(data.Items) != 4 {
		t.Fatalf("rating sort list = %+v, want all four works", data)
	}
	byWork := map[string]int{}
	for i, item := range data.Items {
		byWork[item["work_id"].(string)] = i
	}
	if byWork[w8] != 0 || byWork[w5] != 1 {
		t.Fatalf("rating sort order = %+v, want eight first and five second", byWork)
	}
	if byWork[wU] < 2 || byWork[wC] < 2 {
		t.Fatalf("rating sort order = %+v, want the unrated and cleared works last", byWork)
	}
}
