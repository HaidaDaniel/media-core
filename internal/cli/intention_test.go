package cli

import (
	"encoding/json"
	"testing"
)

type intentionData struct {
	Revision  int64 `json:"revision"`
	Intention *struct {
		Value  string  `json:"value"`
		Reason *string `json:"reason"`
	} `json:"intention"`
}

func intentionEnv(t *testing.T, out string) intentionData {
	t.Helper()
	env := libEnv(t, out)
	if !env.OK || env.Error != nil {
		t.Fatalf("intention envelope = %+v, want ok", env)
	}
	b, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var data intentionData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("data is not an edition view: %v\n%s", err, out)
	}
	return data
}

// TestIntentionSetCLI proves the intention command records the current
// intention, renders none as a null intention, and bumps the revision.
func TestIntentionSetCLI(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Intention Story")

	code, out, errw := runCLI(t, "intention", "set", "--state-dir", dir, ed, "--value", "saved", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("intention set: exit = %d, stderr = %q", code, errw)
	}
	data := intentionEnv(t, out)
	if data.Revision != 1 || data.Intention == nil || data.Intention.Value != "saved" {
		t.Fatalf("view = %+v, want saved at revision 1", data)
	}

	// A same-value no-op does not bump the revision.
	code, out, errw = runCLI(t, "intention", "set", "--state-dir", dir, ed, "--value", "saved", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("intention set no-op: exit = %d, stderr = %q", code, errw)
	}
	data = intentionEnv(t, out)
	if data.Revision != 1 {
		t.Fatalf("revision = %d, want 1 after the no-op", data.Revision)
	}

	// The retraction to none renders as a null intention.
	code, out, errw = runCLI(t, "intention", "set", "--state-dir", dir, ed, "--value", "none", "--reason", "changed my mind", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("intention set none: exit = %d, stderr = %q", code, errw)
	}
	data = intentionEnv(t, out)
	if data.Revision != 2 || data.Intention != nil {
		t.Fatalf("view = %+v, want a null intention at revision 2", data)
	}
}

// TestIntentionSetCLIInvalid proves the CLI rejects unknown values, stale
// revisions and missing targets with the contract exit codes and leaves
// prior state intact.
func TestIntentionSetCLIInvalid(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Invalid Intention")

	if code, _, _ := runCLI(t, "intention", "set", "--state-dir", dir, ed, "--value", "maybe", "--revision", "0"); code != ExitUsage {
		t.Fatalf("unknown value: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "intention", "set", "--state-dir", dir, ed, "--value", "saved", "--revision", "99"); code != ExitUsage {
		t.Fatalf("stale revision: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "intention", "set", "--state-dir", dir, "00000000-0000-4000-8000-000000000000", "--value", "saved", "--revision", "0"); code != ExitUsage {
		t.Fatalf("missing edition: exit = %d, want %d", code, ExitUsage)
	}

	// Prior state stays intact: the first valid set still applies.
	code, out, errw := runCLI(t, "intention", "set", "--state-dir", dir, ed, "--value", "saved", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("intention set after failures: exit = %d, stderr = %q", code, errw)
	}
	data := intentionEnv(t, out)
	if data.Revision != 1 || data.Intention == nil || data.Intention.Value != "saved" {
		t.Fatalf("view = %+v, want saved at revision 1", data)
	}
}
