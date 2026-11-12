package cli

import (
	"encoding/json"
	"testing"
)

func addWorkWithEdition(t *testing.T, dir, title string) (string, string) {
	t.Helper()
	code, out, errw := runCLI(t, "add", "--state-dir", dir, "--title", title, "--kind", "manga")
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

type statusData struct {
	Revision    int64 `json:"revision"`
	Consumption *struct {
		Value  string  `json:"value"`
		Reason *string `json:"reason"`
	} `json:"consumption"`
}

func statusEnv(t *testing.T, out string) statusData {
	t.Helper()
	env := libEnv(t, out)
	if !env.OK || env.Error != nil {
		t.Fatalf("status envelope = %+v, want ok", env)
	}
	b, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var data statusData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("data is not an edition view: %v\n%s", err, out)
	}
	return data
}

// TestStatusSetCLI proves the status command records an observation, bumps
// the revision and prints the updated edition view.
func TestStatusSetCLI(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Status Story")

	code, out, errw := runCLI(t, "status", "set", "--state-dir", dir, ed, "--value", "in_progress", "--reason", "started", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("status set: exit = %d, stderr = %q", code, errw)
	}
	data := statusEnv(t, out)
	if data.Revision != 1 {
		t.Fatalf("revision = %d, want 1", data.Revision)
	}
	if data.Consumption == nil || data.Consumption.Value != "in_progress" {
		t.Fatalf("consumption = %+v, want in_progress", data.Consumption)
	}
	if data.Consumption.Reason == nil || *data.Consumption.Reason != "started" {
		t.Fatalf("reason = %+v, want started", data.Consumption.Reason)
	}

	// A follow-up transition on the fresh revision.
	code, out, errw = runCLI(t, "status", "set", "--state-dir", dir, ed, "--value", "completed", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("status set completed: exit = %d, stderr = %q", code, errw)
	}
	data = statusEnv(t, out)
	if data.Revision != 2 || data.Consumption == nil || data.Consumption.Value != "completed" {
		t.Fatalf("view = %+v, want completed at revision 2", data)
	}
}

// TestStatusSetCLISameValueNoOp proves the same current value does not bump
// the revision.
func TestStatusSetCLISameValueNoOp(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Noop Story")

	code, out, errw := runCLI(t, "status", "set", "--state-dir", dir, ed, "--value", "in_progress", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("status set: exit = %d, stderr = %q", code, errw)
	}
	data := statusEnv(t, out)
	if data.Revision != 1 {
		t.Fatalf("revision = %d, want 1", data.Revision)
	}
	code, out, errw = runCLI(t, "status", "set", "--state-dir", dir, ed, "--value", "in_progress", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("status set same value: exit = %d, stderr = %q", code, errw)
	}
	data = statusEnv(t, out)
	if data.Revision != 1 {
		t.Fatalf("revision after same-value no-op = %d, want 1", data.Revision)
	}
}

// TestStatusRetractUnknownCLI proves an explicit retraction to unknown
// renders as the absence of a consumption state.
func TestStatusRetractUnknownCLI(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Retract Story")

	code, out, errw := runCLI(t, "status", "set", "--state-dir", dir, ed, "--value", "completed", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("status set: exit = %d, stderr = %q", code, errw)
	}
	code, out, errw = runCLI(t, "status", "set", "--state-dir", dir, ed, "--value", "unknown", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("status set unknown: exit = %d, stderr = %q", code, errw)
	}
	data := statusEnv(t, out)
	if data.Consumption != nil {
		t.Fatalf("consumption = %+v, want null after retraction", data.Consumption)
	}
}

// TestStatusInvalidCLI proves invalid input is a usage failure: unknown
// values (including the forbidden on_hold), a missing value and a stale
// revision.
func TestStatusInvalidCLI(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Invalid Story")

	for _, value := range []string{"on_hold", "planned", "watching"} {
		code, out, errw := runCLI(t, "status", "set", "--state-dir", dir, ed, "--value", value, "--revision", "0")
		if code != ExitUsage {
			t.Fatalf("status set %q: exit = %d, want %d; stderr = %q, out = %q", value, code, ExitUsage, errw, out)
		}
	}
	code, out, errw := runCLI(t, "status", "set", "--state-dir", dir, ed, "--revision", "0")
	if code != ExitUsage {
		t.Fatalf("status set without value: exit = %d, want %d; stderr = %q", code, ExitUsage, errw)
	}
	code, out, errw = runCLI(t, "status", "set", "--state-dir", dir, ed, "--value", "in_progress", "--revision", "99")
	if code != ExitUsage {
		t.Fatalf("status set stale revision: exit = %d, want %d; stderr = %q, out = %q", code, ExitUsage, errw, out)
	}
	env := libEnv(t, out)
	if env.OK || env.Error == nil || env.Error.Code != "conflict" {
		t.Fatalf("stale revision envelope = %+v, want a conflict error", env)
	}
}

// TestStatusReplayCLI proves request replay: the same key with the same
// payload replays, a different payload on the same key conflicts.
func TestStatusReplayCLI(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Replay Story")

	code, out, errw := runCLI(t, "status", "set", "--state-dir", dir, ed, "--value", "in_progress", "--revision", "0", "--request-id", "cli-replay-1")
	if code != ExitOK {
		t.Fatalf("status set: exit = %d, stderr = %q", code, errw)
	}
	data := statusEnv(t, out)
	if data.Revision != 1 {
		t.Fatalf("revision = %d, want 1", data.Revision)
	}
	code, out, errw = runCLI(t, "status", "set", "--state-dir", dir, ed, "--value", "in_progress", "--revision", "0", "--request-id", "cli-replay-1")
	if code != ExitOK {
		t.Fatalf("status set replay: exit = %d, stderr = %q", code, errw)
	}
	data = statusEnv(t, out)
	if data.Revision != 1 {
		t.Fatalf("replayed revision = %d, want 1", data.Revision)
	}
	code, out, errw = runCLI(t, "status", "set", "--state-dir", dir, ed, "--value", "completed", "--revision", "1", "--request-id", "cli-replay-1")
	if code != ExitUsage {
		t.Fatalf("same key different payload: exit = %d, want %d; out = %q", code, ExitUsage, out)
	}
	env := libEnv(t, out)
	if env.OK || env.Error == nil || env.Error.Code != "conflict" {
		t.Fatalf("key collision envelope = %+v, want a conflict error", env)
	}
}

// TestListConsumptionFilterCLI proves the list command's consumption
// dimension, including the unknown selection.
func TestListConsumptionFilterCLI(t *testing.T) {
	dir := initLibState(t)
	workA, edA := addWorkWithEdition(t, dir, "Alpha List")
	workB, edB := addWorkWithEdition(t, dir, "Beta List")
	workC, edC := addWorkWithEdition(t, dir, "Gamma List")
	workE, _ := addWorkWithEdition(t, dir, "Epsilon List")
	workD := addWorkID(t, dir, "Delta List")

	code, _, errw := runCLI(t, "status", "set", "--state-dir", dir, edA, "--value", "completed", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("status set A: exit = %d, stderr = %q", code, errw)
	}
	code, _, errw = runCLI(t, "status", "set", "--state-dir", dir, edC, "--value", "in_progress", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("status set C: exit = %d, stderr = %q", code, errw)
	}
	// B is retracted back to unknown; E keeps its manual not_started
	// default; D has no edition at all.
	code, _, errw = runCLI(t, "status", "set", "--state-dir", dir, edB, "--value", "unknown", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("status set B unknown: exit = %d, stderr = %q", code, errw)
	}

	listIDs := func(args ...string) map[string]bool {
		t.Helper()
		code, out, errw := runCLI(t, append([]string{"list", "--state-dir", dir}, args...)...)
		if code != ExitOK {
			t.Fatalf("list %v: exit = %d, stderr = %q", args, code, errw)
		}
		data := listEnv(t, out)
		got := map[string]bool{}
		for _, item := range data.Items {
			got[item["work_id"].(string)] = true
		}
		return got
	}
	want := func(got map[string]bool, works ...string) {
		t.Helper()
		if len(got) != len(works) {
			t.Fatalf("works = %v, want exactly %v", got, works)
		}
		for _, w := range works {
			if !got[w] {
				t.Fatalf("works = %v, want to include %s", got, w)
			}
		}
	}

	want(listIDs("--consumption", "completed"), workA)
	want(listIDs("--consumption", "completed", "--consumption", "in_progress"), workA, workC)
	// The unknown selection matches the retracted edition; an
	// edition-scoped dimension never matches work D, which has no edition.
	want(listIDs("--consumption", "unknown"), workB)
	want(listIDs("--consumption", "not_started"), workE)
	want(listIDs(), workA, workB, workC, workE, workD)
}
