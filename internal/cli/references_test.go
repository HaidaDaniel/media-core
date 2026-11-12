package cli

import (
	"encoding/json"
	"testing"
)

type referenceData struct {
	ID         string `json:"id"`
	Revision   int64  `json:"revision"`
	References []struct {
		ID            string  `json:"id"`
		Provider      string  `json:"provider"`
		ExternalID    *string `json:"external_id"`
		URL           *string `json:"url"`
		Role          string  `json:"role"`
		Authoritative bool    `json:"authoritative"`
		Language      *string `json:"language"`
		Provenance    string  `json:"provenance"`
		RetiredAt     *string `json:"retired_at"`
	} `json:"references"`
}

func referenceEnv(t *testing.T, out string) referenceData {
	t.Helper()
	env := libEnv(t, out)
	if !env.OK || env.Error != nil {
		t.Fatalf("reference envelope = %+v, want ok", env)
	}
	b, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var data referenceData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("data is not an edition view: %v\n%s", err, out)
	}
	return data
}

// TestReferenceCLI proves the reference command: a URL reference and a
// provider external-ID reference both render in the edition view, and the
// retirement drops them from the view.
func TestReferenceCLI(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Reference Story")

	code, out, errw := runCLI(t, "reference", "add", "--state-dir", dir, ed,
		"--provider", "example", "--url", "https://example.org/anime/r",
		"--role", "publication", "--authoritative", "--language", "ja",
		"--provenance", "manual", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("reference add url: exit = %d, stderr = %q", code, errw)
	}
	data := referenceEnv(t, out)
	if data.Revision != 1 || len(data.References) != 1 {
		t.Fatalf("view = %+v, want one reference at revision 1", data)
	}
	r := data.References[0]
	if r.Provider != "example" || r.URL == nil || *r.URL != "https://example.org/anime/r" ||
		r.Role != "publication" || !r.Authoritative || r.Language == nil || *r.Language != "ja" ||
		r.Provenance != "manual" || r.ExternalID != nil {
		t.Fatalf("reference = %+v, want the published reference", r)
	}

	// A provider external-ID reference on the same edition.
	code, out, errw = runCLI(t, "reference", "add", "--state-dir", dir, ed,
		"--provider", "manga_updates", "--external-id", "42",
		"--role", "metadata", "--provenance", "manual", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("reference add id: exit = %d, stderr = %q", code, errw)
	}
	data = referenceEnv(t, out)
	if data.Revision != 2 || len(data.References) != 2 {
		t.Fatalf("view = %+v, want two references at revision 2", data)
	}

	// The retirement drops the reference from the view.
	code, out, errw = runCLI(t, "reference", "retire", "--state-dir", dir, ed,
		"--reference-id", r.ID, "--reason", "wrong site", "--revision", "2")
	if code != ExitOK {
		t.Fatalf("reference retire: exit = %d, stderr = %q", code, errw)
	}
	data = referenceEnv(t, out)
	if data.Revision != 3 || len(data.References) != 1 || data.References[0].ExternalID == nil {
		t.Fatalf("view = %+v, want the surviving reference at revision 3", data)
	}
}

// TestReferenceCLIInvalid proves the CLI rejects malformed invocations with
// usage errors, dangerous schemes and foreign-edition identities as stable
// failures that leave the prior state intact.
func TestReferenceCLIInvalid(t *testing.T) {
	dir := initLibState(t)
	_, ed := addWorkWithEdition(t, dir, "Invalid References")
	_, ed2 := addWorkWithEdition(t, dir, "Invalid References 2")

	if code, _, _ := runCLI(t, "reference", "rename", "--state-dir", dir, ed); code != ExitUsage {
		t.Fatalf("unknown verb: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "reference", "add", "--state-dir", dir, ed, "--url", "https://example.org/x", "--role", "metadata", "--provenance", "manual"); code != ExitUsage {
		t.Fatalf("missing provider: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "reference", "add", "--state-dir", dir, ed, "--provider", "p", "--external-id", "1", "--url", "https://example.org/x", "--role", "metadata", "--provenance", "manual"); code != ExitUsage {
		t.Fatalf("both identity forms: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "reference", "add", "--state-dir", dir, ed, "--provider", "p", "--role", "metadata", "--provenance", "manual"); code != ExitUsage {
		t.Fatalf("neither identity form: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "reference", "add", "--state-dir", dir, ed, "--provider", "p", "--url", "https://example.org/x", "--provenance", "manual"); code != ExitUsage {
		t.Fatalf("missing role: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "reference", "add", "--state-dir", dir, ed, "--provider", "p", "--url", "https://example.org/x", "--role", "hologram", "--provenance", "manual"); code != ExitUsage {
		t.Fatalf("unknown role: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "reference", "add", "--state-dir", dir, ed, "--provider", "p", "--url", "https://example.org/x", "--role", "metadata"); code != ExitUsage {
		t.Fatalf("missing provenance: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "reference", "retire", "--state-dir", dir, ed, "--revision", "0"); code != ExitUsage {
		t.Fatalf("missing reference id: exit = %d, want %d", code, ExitUsage)
	}

	// A dangerous scheme is a stable invalid-argument failure, not a fetch.
	code, out, errw := runCLI(t, "reference", "add", "--state-dir", dir, ed,
		"--provider", "p", "--url", "javascript:alert(1)", "--role", "metadata", "--provenance", "manual", "--revision", "0")
	if code != ExitUsage {
		t.Fatalf("dangerous scheme: exit = %d, want %d (stderr %q)", code, ExitUsage, errw)
	}
	env := libEnv(t, out)
	if env.OK || env.Error == nil || env.Error.Code != "invalid_argument" {
		t.Fatalf("dangerous scheme envelope = %+v, want invalid_argument", env)
	}

	// A valid reference applies; the same identity on another edition is
	// an identity_conflict that leaves the prior state intact.
	code, out, errw = runCLI(t, "reference", "add", "--state-dir", dir, ed,
		"--provider", "example", "--url", "https://example.org/shared", "--role", "publication", "--provenance", "manual", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("reference add: exit = %d, stderr = %q", code, errw)
	}
	data := referenceEnv(t, out)
	code, out, errw = runCLI(t, "reference", "add", "--state-dir", dir, ed2,
		"--provider", "example", "--url", "https://example.org/shared", "--role", "metadata", "--provenance", "manual", "--revision", "0")
	if code != ExitUsage {
		t.Fatalf("foreign-edition identity: exit = %d, want %d (stderr %q)", code, ExitUsage, errw)
	}
	env = libEnv(t, out)
	if env.OK || env.Error == nil || env.Error.Code != "identity_conflict" {
		t.Fatalf("foreign-edition identity envelope = %+v, want identity_conflict", env)
	}
	// The first edition keeps its reference.
	code, out, errw = runCLI(t, "show", "--state-dir", dir, "edition", ed)
	if code != ExitOK {
		t.Fatalf("show after conflict: exit = %d, stderr = %q", code, errw)
	}
	data = referenceEnv(t, out)
	if len(data.References) != 1 || data.References[0].URL == nil || *data.References[0].URL != "https://example.org/shared" {
		t.Fatalf("prior edition references = %+v, want intact after the conflict", data.References)
	}
}
