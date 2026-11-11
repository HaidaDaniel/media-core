package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

type addEnvelope struct {
	SchemaVersion string `json:"schema_version"`
	OK            bool   `json:"ok"`
	Data          *struct {
		Created      bool    `json:"created"`
		Reason       *string `json:"reason"`
		WorkID       string  `json:"work_id"`
		WorkRevision int64   `json:"work_revision"`
		EditionID    *string `json:"edition_id"`
	} `json:"data"`
	Error *struct {
		Code      string         `json:"code"`
		Message   string         `json:"message"`
		Details   map[string]any `json:"details"`
		Retryable bool           `json:"retryable"`
	} `json:"error"`
}

func addEnvelopeFrom(t *testing.T, out string) addEnvelope {
	t.Helper()
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("stdout must be exactly one JSON document, got %q", out)
	}
	var env addEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout is not the JSON envelope: %v\n%s", err, out)
	}
	return env
}

// TestAddTitleOnlyCreates proves the add command creates a work offline and
// prints the created result.
func TestAddTitleOnlyCreates(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if code, _, errw := runCLI(t, "init", "--state-dir", dir); code != ExitOK {
		t.Fatalf("init: exit = %d, stderr = %q", code, errw)
	}
	code, out, errw := runCLI(t, "add", "--state-dir", dir, "--title", "CLI Story")
	if code != ExitOK {
		t.Fatalf("add exit = %d, stderr = %q", code, errw)
	}
	env := addEnvelopeFrom(t, out)
	if !env.OK || env.Data == nil || !env.Data.Created || env.Data.WorkID == "" || env.Data.EditionID != nil {
		t.Fatalf("envelope = %+v, want a created work without an edition", env)
	}
}

// TestAddCandidatesThenChoice proves the CLI surfaces candidates and
// follows up with an explicit create-distinct choice.
func TestAddCandidatesThenChoice(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if code, _, errw := runCLI(t, "init", "--state-dir", dir); code != ExitOK {
		t.Fatalf("init: exit = %d, stderr = %q", code, errw)
	}
	code, out, _ := runCLI(t, "add", "--state-dir", dir, "--title", "Duplicate Title")
	if code != ExitOK {
		t.Fatalf("first add exit = %d, stdout = %q", code, out)
	}
	again, out2, errw := runCLI(t, "add", "--state-dir", dir, "--title", "Duplicate Title")
	if again != ExitOK {
		t.Fatalf("second add exit = %d, stderr = %q", again, errw)
	}
	var envC struct {
		OK   bool `json:"ok"`
		Data struct {
			Outcome    string `json:"outcome"`
			Candidates []struct {
				WorkID string `json:"work_id"`
			} `json:"candidates"`
			CandidateToken *string `json:"candidate_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out2), &envC); err != nil {
		t.Fatalf("second add stdout is not the candidates document: %v\n%s", err, out2)
	}
	if !envC.OK || envC.Data.Outcome != "candidates" || len(envC.Data.Candidates) != 1 || envC.Data.CandidateToken == nil {
		t.Fatalf("candidates = %+v (raw: %s), want one candidate with a token", envC.Data, out2)
	}
	// The follow-up must carry the same request fingerprint as the probe
	// that produced the token, so the title-only request is repeated.
	third, out3, errw := runCLI(t, "add", "--state-dir", dir,
		"--title", "Duplicate Title",
		"--choice", "create_distinct",
		"--candidate-token", *envC.Data.CandidateToken)
	if third != ExitOK {
		t.Fatalf("create-distinct exit = %d, stderr = %q", third, errw)
	}
	env3 := addEnvelopeFrom(t, out3)
	if !env3.OK || !env3.Data.Created || env3.Data.WorkID == envC.Data.Candidates[0].WorkID {
		t.Fatalf("create-distinct = %+v, want a new work different from the candidate", env3)
	}
}

// TestAddExistingIdentityReturnsCreatedFalse proves an exact identity
// returns created false with reason already_exists.
func TestAddExistingIdentityReturnsCreatedFalse(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if code, _, errw := runCLI(t, "init", "--state-dir", dir); code != ExitOK {
		t.Fatalf("init: exit = %d, stderr = %q", code, errw)
	}
	code, out, _ := runCLI(t, "add", "--state-dir", dir,
		"--title", "Identity Story", "--kind", "manga",
		"--provider", "p", "--external-id", "e1")
	if code != ExitOK {
		t.Fatalf("first add exit = %d, stdout = %q", code, out)
	}
	first := addEnvelopeFrom(t, out)
	if !first.OK || !first.Data.Created {
		t.Fatalf("first add = %+v, want created", first)
	}
	code2, out2, errw := runCLI(t, "add", "--state-dir", dir,
		"--title", "Identity Story", "--kind", "manga",
		"--provider", "p", "--external-id", "e1")
	if code2 != ExitOK {
		t.Fatalf("second add exit = %d, stderr = %q", code2, errw)
	}
	second := addEnvelopeFrom(t, out2)
	if !second.OK || second.Data == nil || second.Data.Created {
		t.Fatalf("second add = %+v, want ok with created false", second)
	}
	if second.Data.Reason == nil || *second.Data.Reason != "already_exists" || second.Data.WorkID != first.Data.WorkID {
		t.Fatalf("second add = %+v (raw: %s), want already_exists for the same work", second, out2)
	}
	if second.Data.EditionID == nil {
		t.Fatalf("second add = %+v, want the matched edition", second)
	}
}

// TestAddContradictoryIdentityExitsConflict proves contradictory identity
// fields fail with exit 2 and a typed error.
func TestAddContradictoryIdentityExitsConflict(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if code, _, errw := runCLI(t, "init", "--state-dir", dir); code != ExitOK {
		t.Fatalf("init: exit = %d, stderr = %q", code, errw)
	}
	code, out, _ := runCLI(t, "add", "--state-dir", dir,
		"--title", "Kind Story", "--kind", "manga",
		"--provider", "p", "--external-id", "e1")
	if code != ExitOK {
		t.Fatalf("first add exit = %d, stdout = %q", code, out)
	}
	code2, out2, errw := runCLI(t, "add", "--state-dir", dir,
		"--title", "Kind Story", "--kind", "anime",
		"--provider", "p", "--external-id", "e1")
	if code2 != ExitConflict {
		t.Fatalf("contradictory add exit = %d, stderr = %q, want %d", code2, errw, ExitConflict)
	}
	env := addEnvelopeFrom(t, out2)
	if env.OK || env.Error == nil || env.Error.Code != "identity_conflict" {
		t.Fatalf("envelope = %+v, want an identity_conflict error", env)
	}
}

// TestAddURLWithoutKnownIdentityRequestsTitle proves a URL-only add without
// a known identity fails with exit 2 and asks for a title.
func TestAddURLWithoutKnownIdentityRequestsTitle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if code, _, errw := runCLI(t, "init", "--state-dir", dir); code != ExitOK {
		t.Fatalf("init: exit = %d, stderr = %q", code, errw)
	}
	code, out, errw := runCLI(t, "add", "--state-dir", dir, "--url", "https://example.com/unknown")
	if code != ExitConflict {
		t.Fatalf("url-only add exit = %d, stderr = %q, want %d", code, errw, ExitConflict)
	}
	env := addEnvelopeFrom(t, out)
	if env.OK || env.Error == nil || env.Error.Code != "invalid_argument" {
		t.Fatalf("envelope = %+v, want invalid_argument asking for a title", env)
	}
	if !strings.Contains(env.Error.Message, "title") {
		t.Fatalf("error message = %q, want it to ask for a title", env.Error.Message)
	}
}

func TestAddUnknownArgument(t *testing.T) {
	code, _, _ := runCLI(t, "add", "--bogus")
	if code != ExitUsage {
		t.Fatalf("add with unknown argument: exit = %d, want %d", code, ExitUsage)
	}
}
