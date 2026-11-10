package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureDir is the canonical fixture directory frozen by this task.
const fixtureDir = "../../tests/contracts/fixtures"

// fixtureTypes maps every fixture file to its transport-visible type.
// A fixture file without a mapping fails the round-trip test so new
// shapes cannot enter the contract untyped.
var fixtureTypes = map[string]func() any{
	"list_success.json":         func() any { return new(Envelope) },
	"failure_conflict.json":     func() any { return new(Envelope) },
	"work_view.json":            func() any { return new(WorkView) },
	"edition_view.json":         func() any { return new(EditionView) },
	"edition_view_minimal.json": func() any { return new(EditionView) },
	"progress_record.json":      func() any { return new(ProgressRecord) },
	"mutation_progress.json":    func() any { return new(RecordProgressRequest) },
	"resolve_candidates.json":   func() any { return new(ResolveResult) },
}

// TestFixtureRoundTrips proves that every canonical fixture decodes into
// its frozen type and re-encodes to the identical bytes, so the fixtures
// are the byte-level contract.
func TestFixtureRoundTrips(t *testing.T) {
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("read fixture dir: %v", err)
	}
	var seen []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		seen = append(seen, entry.Name())
		factory, ok := fixtureTypes[entry.Name()]
		if !ok {
			t.Fatalf("fixture %s has no mapped type", entry.Name())
		}
		raw, err := os.ReadFile(filepath.Join(fixtureDir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		target := factory()
		if err := json.Unmarshal(raw, target); err != nil {
			t.Fatalf("unmarshal %s: %v", entry.Name(), err)
		}
		out, err := json.Marshal(target)
		if err != nil {
			t.Fatalf("marshal %s: %v", entry.Name(), err)
		}
		want := bytes.TrimRight(raw, "\n")
		if !bytes.Equal(out, want) {
			t.Errorf("%s round trip diverged:\n got: %s\nwant: %s", entry.Name(), out, want)
		}
	}
	for name := range fixtureTypes {
		found := false
		for _, s := range seen {
			if s == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("mapped fixture %s is missing from %s", name, fixtureDir)
		}
	}
}

// TestDocumentedEnvelopeExamplesFrozen pins the two envelope examples from
// docs/application-contract.md byte for byte.
func TestDocumentedEnvelopeExamplesFrozen(t *testing.T) {
	list := Success(ListPayload{Items: []any{}})
	got, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte(`{"schema_version":"1.0","ok":true,"data":{"items":[],"next_cursor":null,"total":0}}`)
	if !bytes.Equal(got, want) {
		t.Errorf("success envelope:\n got: %s\nwant: %s", got, want)
	}

	fail := Failure(NewAppError(ErrConflict, "Record changed; reload before retrying.").WithCurrentRevision(8))
	got, err = json.Marshal(fail)
	if err != nil {
		t.Fatal(err)
	}
	want = []byte(`{"schema_version":"1.0","ok":false,"error":{"code":"conflict","message":"Record changed; reload before retrying.","details":{"current_revision":8},"retryable":false}}`)
	if !bytes.Equal(got, want) {
		t.Errorf("failure envelope:\n got: %s\nwant: %s", got, want)
	}
}

// TestRejectsInvalidEnums proves unknown enum values never decode into the
// frozen DTOs.
func TestRejectsInvalidEnums(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		target any
	}{
		{"edition kind", `{"id":"x","work_id":"w","kind":"comic","language":null,"other_label":null,"revision":1,"titles":[],"consumption":null,"intention":null,"publication":null,"progress":{},"rating":null,"tags":[],"references":[],"notes":[]}`, new(EditionView)},
		{"consumption value", `{"id":"x","work_id":"w","kind":"anime","language":null,"other_label":null,"revision":1,"titles":[],"consumption":{"value":"on_hold","reason":null,"observed_at":"2026-01-01T00:00:00Z","recorded_at":"2026-01-01T00:00:00Z","actor":{"principal":"owner","origin":"user"},"supersedes_id":null},"intention":null,"publication":null,"progress":{},"rating":null,"tags":[],"references":[],"notes":[]}`, new(EditionView)},
		{"intention value", `{"id":"x","work_id":"w","kind":"anime","language":null,"other_label":null,"revision":1,"titles":[],"consumption":null,"intention":{"value":"maybe","reason":null,"observed_at":"2026-01-01T00:00:00Z","recorded_at":"2026-01-01T00:00:00Z","actor":{"principal":"owner","origin":"user"},"supersedes_id":null},"publication":null,"progress":{},"rating":null,"tags":[],"references":[],"notes":[]}`, new(EditionView)},
		{"publication value", `{"id":"x","work_id":"w","kind":"anime","language":null,"other_label":null,"revision":1,"titles":[],"consumption":null,"intention":null,"publication":{"value":"finished","source":null,"observed_at":"2026-01-01T00:00:00Z","recorded_at":"2026-01-01T00:00:00Z","provenance":"user","supersedes_id":null},"progress":{},"rating":null,"tags":[],"references":[],"notes":[]}`, new(EditionView)},
		{"title kind", `{"id":"w","revision":1,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","archived_at":null,"titles":[{"id":"t","text":"x","language":null,"kind":"transliterated","preferred":false,"provenance":"user"}],"editions":[]}`, new(WorkView)},
		{"progress unit", `{"id":"p","unit":"scene","position":"1","total":null,"label":null,"observed_at":"2026-01-01T00:00:00Z","recorded_at":"2026-01-01T00:00:00Z","actor":{"principal":"owner","origin":"user"},"supersedes_id":null}`, new(ProgressRecord)},
		{"reference role", `{"id":"r","provider":"p","external_id":null,"url":"https://example.org","role":"link","authoritative":false,"language":null,"provenance":"user","retired_at":null}`, new(ExternalReference)},
		{"actor origin", `{"request_id":"q","actor":{"principal":"owner","origin":"daemon"},"expected_revision":1,"edition_id":"e","unit":"chapter","position":"1","total":null,"label":null,"observed_at":"2026-01-01T00:00:00Z","reason":null,"supersedes_id":null}`, new(RecordProgressRequest)},
		{"resolve outcome", `{"outcome":"ambiguous","work_id":null,"candidate_token":null,"request_fingerprint":null}`, new(ResolveResult)},
	}
	for _, tc := range cases {
		if err := json.Unmarshal([]byte(tc.raw), tc.target); err == nil {
			t.Errorf("%s: invalid enum decoded without error", tc.name)
		}
	}
}

// TestRejectsNullEnums proves null is rejected where an enum value is
// required; unknown facts stay null one level up (as whole state objects).
func TestRejectsNullEnums(t *testing.T) {
	if err := json.Unmarshal([]byte(`{"id":"x","work_id":"w","kind":null,"language":null,"other_label":null,"revision":1,"titles":[],"consumption":null,"intention":null,"publication":null,"progress":{},"rating":null,"tags":[],"references":[],"notes":[]}`), new(EditionView)); err == nil {
		t.Error("null edition kind decoded without error")
	}
	if err := json.Unmarshal([]byte(`{"id":"x","work_id":"w","kind":"anime","language":null,"other_label":null,"revision":1,"titles":[],"consumption":{"value":null,"reason":null,"observed_at":"2026-01-01T00:00:00Z","recorded_at":"2026-01-01T00:00:00Z","actor":{"principal":"owner","origin":"user"},"supersedes_id":null},"intention":null,"publication":null,"progress":{},"rating":null,"tags":[],"references":[],"notes":[]}`), new(EditionView)); err == nil {
		t.Error("null consumption value decoded without error")
	}
}

// TestNullVersusZero proves unknown scalars round-trip as null (never an
// invented empty string or zero) and unavailable collections as empty.
func TestNullVersusZero(t *testing.T) {
	var v EditionView
	if err := json.Unmarshal(mustReadFixture(t, "edition_view_minimal.json"), &v); err != nil {
		t.Fatal(err)
	}
	if v.Language != nil || v.OtherLabel == nil || v.Consumption != nil || v.Rating != nil {
		t.Errorf("null-vs-zero pointers wrong: language=%v other_label=%v consumption=%v rating=%v",
			v.Language, v.OtherLabel, v.Consumption, v.Rating)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"language":null`, `"other_label":"podcast"`, `"consumption":null`,
		`"intention":null`, `"publication":null`, `"progress":{}`, `"rating":null`,
		`"titles":[]`, `"tags":[]`, `"references":[]`, `"notes":[]`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("re-encoded minimal edition missing %s:\n%s", want, out)
		}
	}

	var rich EditionView
	if err := json.Unmarshal(mustReadFixture(t, "edition_view.json"), &rich); err != nil {
		t.Fatal(err)
	}
	if rich.Rating == nil || rich.Rating.Value == nil || *rich.Rating.Value != 8 {
		t.Errorf("rich edition rating = %+v, want value 8", rich.Rating)
	}
	if rich.Intention != nil {
		t.Errorf("rich edition intention = %+v, want null (no recorded intention)", rich.Intention)
	}
	if rich.Progress.Episode == nil || rich.Progress.Episode.Total == nil || *rich.Progress.Episode.Total != "12" {
		t.Errorf("rich edition episode progress = %+v, want total 12", rich.Progress.Episode)
	}
}

// TestProgressDecimalsRoundTripExactly proves positions and totals are
// exact decimal strings, so trailing zeros and precision survive.
func TestProgressDecimalsRoundTripExactly(t *testing.T) {
	var p ProgressRecord
	if err := json.Unmarshal(mustReadFixture(t, "progress_record.json"), &p); err != nil {
		t.Fatal(err)
	}
	if p.Position != "42.50" || p.Total == nil || *p.Total != "120" {
		t.Fatalf("position/total = %q/%v, want exact strings 42.50/120", p.Position, p.Total)
	}
	out, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"position":"42.50"`) {
		t.Errorf("re-encoded progress lost the exact decimal:\n%s", out)
	}
}

// TestErrorMappings freezes the stable code to transport mapping shared by
// every adapter.
func TestErrorMappings(t *testing.T) {
	cases := []struct {
		code    ErrorCode
		http    int
		cliExit int
	}{
		{ErrInvalidArgument, 400, 2},
		{ErrNotFound, 404, 2},
		{ErrConflict, 409, 2},
		{ErrIdentityConflict, 409, 2},
		{ErrDuplicateCandidate, 409, 2},
		{ErrCycleDetected, 409, 2},
		{ErrUnauthorized, 401, 1},
		{ErrForbidden, 403, 1},
		{ErrRateLimited, 429, 1},
		{ErrLocked, 503, 4},
		{ErrSchemaMismatch, 500, 1},
		{ErrStorageError, 500, 1},
		{ErrConfigError, 500, 1},
		{ErrInternalError, 500, 1},
	}
	for _, tc := range cases {
		if !tc.code.Valid() {
			t.Errorf("code %s reported invalid", tc.code)
		}
		if got := tc.code.HTTPStatus(); got != tc.http {
			t.Errorf("code %s HTTPStatus = %d, want %d", tc.code, got, tc.http)
		}
		if got := tc.code.CLIExit(); got != tc.cliExit {
			t.Errorf("code %s CLIExit = %d, want %d", tc.code, got, tc.cliExit)
		}
	}
	if ErrorCode("bogus").Valid() {
		t.Error("unknown code reported valid")
	}
}

// TestErrorRetryability freezes the retryability default: only locked is
// retryable, so an uncertain commit is never auto-retried.
func TestErrorRetryability(t *testing.T) {
	if e := NewAppError(ErrLocked, "state is locked"); !e.Retryable {
		t.Errorf("locked error %v should be retryable", e)
	}
	for _, code := range []ErrorCode{ErrConflict, ErrStorageError, ErrInternalError, ErrInvalidArgument} {
		if e := NewAppError(code, "x"); e.Retryable {
			t.Errorf("error %s should not be retryable", code)
		}
	}
}

// TestEnvelopeExclusivity proves a success never carries an error and a
// failure never carries data: no success-masked error in any adapter.
func TestEnvelopeExclusivity(t *testing.T) {
	out, err := json.Marshal(Success(EditionView{}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `"error"`) {
		t.Errorf("success envelope contains an error key:\n%s", out)
	}
	if !strings.Contains(string(out), `"ok":true`) || !strings.Contains(string(out), `"data":`) {
		t.Errorf("success envelope shape wrong:\n%s", out)
	}

	out, err = json.Marshal(Failure(NewAppError(ErrNotFound, "no such work")))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `"data"`) {
		t.Errorf("failure envelope contains a data key:\n%s", out)
	}
	if !strings.Contains(string(out), `"ok":false`) || !strings.Contains(string(out), `"error":{`) {
		t.Errorf("failure envelope shape wrong:\n%s", out)
	}
}

// TestAppErrorWire proves audit context travels with the error while the
// wire error keeps the documented fields and revision detail.
func TestAppErrorWire(t *testing.T) {
	e := NewAppError(ErrConflict, "Record changed; reload before retrying.")
	e = e.WithCurrentRevision(8)
	e.RequestID = "req-1"
	e.Actor = "owner"
	if e.Error() != "conflict: Record changed; reload before retrying." {
		t.Errorf("Error() = %q", e.Error())
	}
	wire := e.WireError()
	rev, ok := wire.Details["current_revision"].(int64)
	if wire.Code != "conflict" || wire.Retryable || !ok || rev != 8 {
		t.Errorf("wire error = %+v, want conflict with current_revision 8 and not retryable", wire)
	}

	locked := NewAppError(ErrLocked, "maintenance in progress").WireError()
	if !locked.Retryable || locked.Code != "locked" {
		t.Errorf("locked wire error = %+v, want retryable", locked)
	}
	if got := locked.Details; len(got) != 0 {
		t.Errorf("locked wire details = %v, want none", got)
	}
}

// TestMutationBaseShape freezes the mutation request fields: request_id,
// actor and optional expected_revision, with the base embedded inline.
func TestMutationBaseShape(t *testing.T) {
	var m RecordProgressRequest
	if err := json.Unmarshal(mustReadFixture(t, "mutation_progress.json"), &m); err != nil {
		t.Fatal(err)
	}
	if m.RequestID == "" || m.Actor.Principal != "owner" || m.Actor.Origin != "user" || m.ExpectedRevision == nil || *m.ExpectedRevision != 5 {
		t.Errorf("mutation base = %+v, want request_id, user actor and expected_revision 5", m.MutationBase)
	}

	noRev := RecordProgressRequest{
		MutationBase: MutationBase{
			RequestID: "q",
			Actor:     Actor{Principal: "owner", Origin: "agent", Identity: "web-ui"},
		},
		EditionID: "e",
		Unit:      "chapter",
		Position:  "1",
	}
	out, err := json.Marshal(noRev)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "expected_revision") {
		t.Errorf("nil expected_revision must be omitted, got:\n%s", out)
	}
	if !strings.Contains(string(out), `"identity":"web-ui"`) {
		t.Errorf("actor identity missing:\n%s", out)
	}
}

func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixtureDir, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
}
