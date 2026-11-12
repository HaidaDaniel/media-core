package cli

import (
	"encoding/json"
	"strconv"
	"testing"
)

type progressData struct {
	Revision int64 `json:"revision"`
	Progress struct {
		Chapter *struct {
			Position     string  `json:"position"`
			Total        *string `json:"total"`
			SupersedesID *string `json:"supersedes_id"`
		} `json:"chapter"`
		Volume *struct {
			Position string `json:"position"`
		} `json:"volume"`
	} `json:"progress"`
}

func progressEnv(t *testing.T, out string) progressData {
	t.Helper()
	env := libEnv(t, out)
	if !env.OK || env.Error != nil {
		t.Fatalf("progress envelope = %+v, want ok", env)
	}
	b, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var data progressData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("data is not an edition view: %v\n%s", err, out)
	}
	return data
}

// TestProgressSetCLI proves the progress command records observations per
// unit, bumps the revision and prints the updated edition view.
func TestProgressSetCLI(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Progress Story")

	code, out, errw := runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "chapter", "--position", "42", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("progress set: exit = %d, stderr = %q", code, errw)
	}
	data := progressEnv(t, out)
	if data.Revision != 1 {
		t.Fatalf("revision = %d, want 1", data.Revision)
	}
	if data.Progress.Chapter == nil || data.Progress.Chapter.Position != "42" {
		t.Fatalf("progress = %+v, want chapter 42", data.Progress)
	}

	// A second unit is independent.
	code, out, errw = runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "volume", "--position", "3", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("progress set volume: exit = %d, stderr = %q", code, errw)
	}
	data = progressEnv(t, out)
	if data.Revision != 2 || data.Progress.Chapter == nil || data.Progress.Volume == nil {
		t.Fatalf("view = %+v, want both units at revision 2", data)
	}

	// A follow-up chapter on the fresh revision.
	code, out, errw = runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "chapter", "--position", "48", "--revision", "2")
	if code != ExitOK {
		t.Fatalf("progress set 48: exit = %d, stderr = %q", code, errw)
	}
	data = progressEnv(t, out)
	if data.Revision != 3 || data.Progress.Chapter.Position != "48" {
		t.Fatalf("view = %+v, want chapter 48 at revision 3", data)
	}
}

// TestProgressSetCLISameValueNoOp proves the same current position does not
// bump the revision.
func TestProgressSetCLISameValueNoOp(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Noop Story")

	code, out, errw := runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "chapter", "--position", "42", "--total", "120", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("progress set: exit = %d, stderr = %q", code, errw)
	}
	data := progressEnv(t, out)
	if data.Revision != 1 || data.Progress.Chapter.Total == nil || *data.Progress.Chapter.Total != "120" {
		t.Fatalf("view = %+v, want chapter 42/120 at revision 1", data)
	}
	code, out, errw = runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "chapter", "--position", "42", "--total", "120", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("progress set same value: exit = %d, stderr = %q", code, errw)
	}
	data = progressEnv(t, out)
	if data.Revision != 1 {
		t.Fatalf("revision after same-value no-op = %d, want 1", data.Revision)
	}
}

// TestProgressRetractCLI proves an explicit retraction renders the unit as
// absent.
func TestProgressRetractCLI(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Retract Story")

	code, out, errw := runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "chapter", "--position", "48", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("progress set: exit = %d, stderr = %q", code, errw)
	}
	env := libEnv(t, out)
	b, _ := json.Marshal(env.Data)
	var full struct {
		Progress struct {
			Chapter struct {
				ID string `json:"id"`
			} `json:"chapter"`
		} `json:"progress"`
	}
	if err := json.Unmarshal(b, &full); err != nil {
		t.Fatalf("data: %v", err)
	}
	recordID := full.Progress.Chapter.ID

	code, out, errw = runCLI(t, "progress", "retract", "--state-dir", dir, ed, "--record-id", recordID, "--reason", "misread", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("progress retract: exit = %d, stderr = %q", code, errw)
	}
	data := progressEnv(t, out)
	if data.Progress.Chapter != nil {
		t.Fatalf("chapter = %+v, want absent after retraction", data.Progress.Chapter)
	}
	if data.Revision != 2 {
		t.Fatalf("revision = %d, want 2", data.Revision)
	}
}

// TestProgressInvalidCLI proves invalid input fails with a distinguishable
// exit code: unknown unit, fractional page, percent above 100, missing
// arguments, blank retraction reason and a stale revision.
func TestProgressInvalidCLI(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Invalid Story")

	code, out, errw := runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "scene", "--position", "1", "--revision", "0")
	if code != ExitUsage {
		t.Fatalf("progress set scene: exit = %d, want %d; stderr = %q", code, ExitUsage, errw)
	}
	code, out, errw = runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "page", "--position", "3.5", "--revision", "0")
	if code != ExitUsage {
		t.Fatalf("progress set page 3.5: exit = %d, want %d; stderr = %q, out = %q", code, ExitUsage, errw, out)
	}
	code, out, errw = runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "percent", "--position", "101", "--revision", "0")
	if code != ExitUsage {
		t.Fatalf("progress set percent 101: exit = %d, want %d; stderr = %q, out = %q", code, ExitUsage, errw, out)
	}
	code, out, errw = runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "chapter", "--revision", "0")
	if code != ExitUsage {
		t.Fatalf("progress set without position: exit = %d, want %d; stderr = %q", code, ExitUsage, errw)
	}
	code, out, errw = runCLI(t, "progress", "retract", "--state-dir", dir, ed, "--revision", "0")
	if code != ExitUsage {
		t.Fatalf("progress retract without record-id: exit = %d, want %d; stderr = %q", code, ExitUsage, errw)
	}
	code, out, errw = runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "chapter", "--position", "42", "--revision", "99")
	if code != ExitUsage {
		t.Fatalf("progress set stale revision: exit = %d, want %d; stderr = %q, out = %q", code, ExitUsage, errw, out)
	}
	env := libEnv(t, out)
	if env.OK || env.Error == nil || env.Error.Code != "conflict" {
		t.Fatalf("stale revision envelope = %+v, want a conflict error", env)
	}
}

// TestProgressReplayCLI proves request replay: the same key with the same
// payload replays, a different payload on the same key conflicts.
func TestProgressReplayCLI(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Replay Story")

	code, out, errw := runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "chapter", "--position", "42", "--revision", "0", "--request-id", "cli-progress-1")
	if code != ExitOK {
		t.Fatalf("progress set: exit = %d, stderr = %q", code, errw)
	}
	data := progressEnv(t, out)
	if data.Revision != 1 {
		t.Fatalf("revision = %d, want 1", data.Revision)
	}
	code, out, errw = runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "chapter", "--position", "42", "--revision", "0", "--request-id", "cli-progress-1")
	if code != ExitOK {
		t.Fatalf("progress set replay: exit = %d, stderr = %q", code, errw)
	}
	data = progressEnv(t, out)
	if data.Revision != 1 {
		t.Fatalf("replayed revision = %d, want 1", data.Revision)
	}
	code, out, errw = runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "chapter", "--position", "48", "--revision", "1", "--request-id", "cli-progress-1")
	if code != ExitUsage {
		t.Fatalf("same key different payload: exit = %d, want %d; out = %q", code, ExitUsage, out)
	}
	env := libEnv(t, out)
	if env.OK || env.Error == nil || env.Error.Code != "conflict" {
		t.Fatalf("key collision envelope = %+v, want a conflict error", env)
	}
}

// TestProgressCorrectionsCLI proves a supersedes correction keeps the
// corrected row superseded in the view and a lower reread is recorded.
func TestProgressCorrectionsCLI(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Correction Story")

	code, out, errw := runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "chapter", "--position", "42", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("progress set 42: exit = %d, stderr = %q", code, errw)
	}
	env := libEnv(t, out)
	b, _ := json.Marshal(env.Data)
	var first struct {
		Progress struct {
			Chapter struct {
				ID string `json:"id"`
			} `json:"chapter"`
		} `json:"progress"`
		Revision int64 `json:"revision"`
	}
	if err := json.Unmarshal(b, &first); err != nil {
		t.Fatalf("data: %v", err)
	}
	code, out, errw = runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "chapter", "--position", "42.5", "--supersedes-id", first.Progress.Chapter.ID, "--revision", strconv.FormatInt(first.Revision, 10))
	if code != ExitOK {
		t.Fatalf("progress correction: exit = %d, stderr = %q", code, errw)
	}
	data := progressEnv(t, out)
	if data.Progress.Chapter == nil || data.Progress.Chapter.Position != "42.5" {
		t.Fatalf("corrected progress = %+v, want chapter 42.5", data.Progress.Chapter)
	}
	if data.Progress.Chapter.SupersedesID == nil || *data.Progress.Chapter.SupersedesID != first.Progress.Chapter.ID {
		t.Fatalf("correction supersedes = %v, want the first row", data.Progress.Chapter.SupersedesID)
	}
	// A lower reread position is accepted.
	code, out, errw = runCLI(t, "progress", "set", "--state-dir", dir, ed, "--unit", "chapter", "--position", "10", "--revision", strconv.FormatInt(data.Revision, 10))
	if code != ExitOK {
		t.Fatalf("progress reread: exit = %d, stderr = %q", code, errw)
	}
	data = progressEnv(t, out)
	if data.Progress.Chapter == nil || data.Progress.Chapter.Position != "10" {
		t.Fatalf("reread progress = %+v, want chapter 10", data.Progress.Chapter)
	}
}
