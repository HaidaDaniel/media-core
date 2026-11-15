package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureDir = "../../tests/contracts/release-watcher"

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtureDir, name))
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	return b
}

func TestEventDecodeValidEnvelope(t *testing.T) {
	d, err := DecodeBytes(loadFixture(t, "envelope_valid.json"), DefaultEnvelopeBytes, DefaultMaxEvents)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if d.SchemaVersion != "1.0" {
		t.Fatalf("schema version = %q, want 1.0", d.SchemaVersion)
	}
	if d.NextCursor == nil || *d.NextCursor != "evt_000000000000000002" {
		t.Fatalf("next_cursor = %v, want evt_000000000000000002", d.NextCursor)
	}
	if len(d.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(d.Events))
	}
	ev := d.Events[0]
	if ev.EventID != "evt_000000000000000001" {
		t.Fatalf("event_id = %q", ev.EventID)
	}
	if ev.EventType != "release.added" {
		t.Fatalf("event_type = %q", ev.EventType)
	}
	if ev.PublicationID == nil || *ev.PublicationID != "pub_mu_1234" {
		t.Fatalf("publication_id = %v", ev.PublicationID)
	}
	if ev.CurrentCount == nil || *ev.CurrentCount != 12 {
		t.Fatalf("current_count = %v", ev.CurrentCount)
	}
	if ev.CountDelta == nil || *ev.CountDelta != 1 {
		t.Fatalf("count_delta = %v", ev.CountDelta)
	}
	if ev.ReleaseTitle == nil || *ev.ReleaseTitle != "Chapter 12" {
		t.Fatalf("release_title = %v", ev.ReleaseTitle)
	}
	if len(ev.Raw) == 0 {
		t.Fatal("raw payload is empty; the exact source payload must be preserved")
	}
	if ev.Canonical == "" {
		t.Fatal("canonical payload is empty")
	}
}

// TestEventDecodeReplayIsCanonicalEqual pins the idempotency rule: the same
// event with reordered keys and extra whitespace is canonical-equal, so a
// replay is a no-op.
func TestEventDecodeReplayIsCanonicalEqual(t *testing.T) {
	valid, err := DecodeBytes(loadFixture(t, "envelope_valid.json"), DefaultEnvelopeBytes, DefaultMaxEvents)
	if err != nil {
		t.Fatalf("Decode valid: %v", err)
	}
	replay, err := DecodeBytes(loadFixture(t, "envelope_replay.json"), DefaultEnvelopeBytes, DefaultMaxEvents)
	if err != nil {
		t.Fatalf("Decode replay: %v", err)
	}
	if valid.Events[0].Canonical != replay.Events[0].Canonical {
		t.Fatalf("canonical forms differ; replay must be canonical-equal:\nvalid:  %s\nreplay: %s",
			valid.Events[0].Canonical, replay.Events[0].Canonical)
	}
}

// TestEventDecodeConflictIsCanonicalDifferent pins the conflict rule: the same
// event_id with a different payload is canonical-different.
func TestEventDecodeConflictIsCanonicalDifferent(t *testing.T) {
	valid, err := DecodeBytes(loadFixture(t, "envelope_valid.json"), DefaultEnvelopeBytes, DefaultMaxEvents)
	if err != nil {
		t.Fatalf("Decode valid: %v", err)
	}
	conflict, err := DecodeBytes(loadFixture(t, "envelope_conflict.json"), DefaultEnvelopeBytes, DefaultMaxEvents)
	if err != nil {
		t.Fatalf("Decode conflict: %v", err)
	}
	if valid.Events[0].EventID != conflict.Events[0].EventID {
		t.Fatalf("conflict fixture must reuse the event_id; got %q vs %q",
			conflict.Events[0].EventID, valid.Events[0].EventID)
	}
	if valid.Events[0].Canonical == conflict.Events[0].Canonical {
		t.Fatal("conflicting payload must be canonical-different")
	}
}

// TestEventDecodeUnknownTypeKeptPending pins that an unknown event type is
// accepted (decoded) and kept for the pending queue, never projected.
func TestEventDecodeUnknownTypeKeptPending(t *testing.T) {
	d, err := DecodeBytes(loadFixture(t, "envelope_unknown_type.json"), DefaultEnvelopeBytes, DefaultMaxEvents)
	if err != nil {
		t.Fatalf("Decode unknown type: %v", err)
	}
	if len(d.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(d.Events))
	}
	if d.Events[0].EventType != "release.flagged" {
		t.Fatalf("event_type = %q, want release.flagged", d.Events[0].EventType)
	}
}

// TestEventDecodeUnknownFieldsPreserved pins that additive fields Media Core does
// not know are preserved in the raw payload.
func TestEventDecodeUnknownFieldsPreserved(t *testing.T) {
	raw := loadFixture(t, "envelope_unknown_fields.json")
	d, err := DecodeBytes(raw, DefaultEnvelopeBytes, DefaultMaxEvents)
	if err != nil {
		t.Fatalf("Decode unknown fields: %v", err)
	}
	if len(d.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(d.Events))
	}
	ev := d.Events[0]
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(ev.Raw, &probe); err != nil {
		t.Fatalf("raw is not a JSON object: %v", err)
	}
	if _, ok := probe["producer_extra"]; !ok {
		t.Fatal("additive field producer_extra must be preserved in the raw payload")
	}
	if _, ok := probe["tags"]; !ok {
		t.Fatal("additive field tags must be preserved in the raw payload")
	}
}

// TestEventDecodeUnsupportedMajorWritesNothing pins that a different schema major
// is rejected before any write.
func TestEventDecodeUnsupportedMajorWritesNothing(t *testing.T) {
	_, err := DecodeBytes(loadFixture(t, "envelope_unsupported_major.json"), DefaultEnvelopeBytes, DefaultMaxEvents)
	if err == nil {
		t.Fatal("unsupported major must be rejected")
	}
	var envErr *EnvelopeError
	if !errors.As(err, &envErr) {
		t.Fatalf("expected EnvelopeError, got %T: %v", err, err)
	}
	if envErr.Code != CodeUnsupportedSchema {
		t.Fatalf("code = %q, want %q", envErr.Code, CodeUnsupportedSchema)
	}
}

func TestEventDecodeRejectsNotOK(t *testing.T) {
	_, err := DecodeBytes([]byte(`{"schema_version":"1.0","ok":false,"data":{"events":[]}}`),
		DefaultEnvelopeBytes, DefaultMaxEvents)
	if err == nil {
		t.Fatal("ok=false must be rejected")
	}
	var envErr *EnvelopeError
	if !errors.As(err, &envErr) || envErr.Code != CodeNotOK {
		t.Fatalf("expected not_ok, got %v", err)
	}
}

func TestEventDecodeRejectsMissingData(t *testing.T) {
	_, err := DecodeBytes([]byte(`{"schema_version":"1.0","ok":true}`),
		DefaultEnvelopeBytes, DefaultMaxEvents)
	if err == nil {
		t.Fatal("missing data must be rejected")
	}
	var envErr *EnvelopeError
	if !errors.As(err, &envErr) || envErr.Code != CodeMissingData {
		t.Fatalf("expected missing_data, got %v", err)
	}
}

func TestEventDecodeRejectsBadJSON(t *testing.T) {
	_, err := DecodeBytes([]byte(`{not json`), DefaultEnvelopeBytes, DefaultMaxEvents)
	if err == nil {
		t.Fatal("bad JSON must be rejected")
	}
	var envErr *EnvelopeError
	if !errors.As(err, &envErr) || envErr.Code != CodeBadJSON {
		t.Fatalf("expected bad_json, got %v", err)
	}
}

func TestEventDecodeRejectsMissingEventID(t *testing.T) {
	_, err := DecodeBytes([]byte(`{"schema_version":"1.0","ok":true,"data":{"events":[{"schema_version":"1.0","event_type":"release.added"}]}}`),
		DefaultEnvelopeBytes, DefaultMaxEvents)
	if err == nil {
		t.Fatal("missing event_id must be rejected")
	}
	var envErr *EnvelopeError
	if !errors.As(err, &envErr) || envErr.Code != CodeMissingEventID {
		t.Fatalf("expected missing_event_id, got %v", err)
	}
}

func TestEventDecodeRejectsMissingEventType(t *testing.T) {
	_, err := DecodeBytes([]byte(`{"schema_version":"1.0","ok":true,"data":{"events":[{"schema_version":"1.0","event_id":"evt_1"}]}}`),
		DefaultEnvelopeBytes, DefaultMaxEvents)
	if err == nil {
		t.Fatal("missing event_type must be rejected")
	}
	var envErr *EnvelopeError
	if !errors.As(err, &envErr) || envErr.Code != CodeMissingEventType {
		t.Fatalf("expected missing_event_type, got %v", err)
	}
}

func TestEventDecodeRejectsTooManyEvents(t *testing.T) {
	events := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		events = append(events, fmt.Sprintf(`{"schema_version":"1.0","event_id":"evt_%d","event_type":"release.added"}`, i))
	}
	body := `{"schema_version":"1.0","ok":true,"data":{"events":[` + strings.Join(events, ",") + `]}}`
	_, err := DecodeBytes([]byte(body), DefaultEnvelopeBytes, 2)
	if err == nil {
		t.Fatal("too many events must be rejected")
	}
	var envErr *EnvelopeError
	if !errors.As(err, &envErr) || envErr.Code != CodeTooManyEvents {
		t.Fatalf("expected too_many_events, got %v", err)
	}
}

func TestEventDecodeRejectsTooLarge(t *testing.T) {
	body := `{"schema_version":"1.0","ok":true,"data":{"events":[]},"pad":"` + strings.Repeat("x", 100) + `"}`
	_, err := DecodeBytes([]byte(body), 50, DefaultMaxEvents)
	if err == nil {
		t.Fatal("oversized envelope must be rejected")
	}
	var envErr *EnvelopeError
	if !errors.As(err, &envErr) || envErr.Code != CodeTooLarge {
		t.Fatalf("expected too_large, got %v", err)
	}
}

// TestEventDecodeCanonicalIgnoresKeyOrderWhitespace is a direct unit test of the
// equality rule.
func TestEventDecodeCanonicalIgnoresKeyOrderWhitespace(t *testing.T) {
	a, err := CanonicalJSON(mustRaw(t, `{"b":1,"a":{"d":true,"c":2},"e":[1,2,3]}`))
	if err != nil {
		t.Fatalf("CanonicalJSON a: %v", err)
	}
	b, err := CanonicalJSON(mustRaw(t, `{"e": [1,2,3], "a": { "c" : 2 , "d": true }, "b" : 1 }`))
	if err != nil {
		t.Fatalf("CanonicalJSON b: %v", err)
	}
	if a != b {
		t.Fatalf("canonical forms differ for key-order/whitespace variants:\n%s\n%s", a, b)
	}
	want := `{"a":{"c":2,"d":true},"b":1,"e":[1,2,3]}`
	if a != want {
		t.Fatalf("canonical = %q, want %q", a, want)
	}
}

// TestEventDecodeCanonicalNumericExactness pins that canonical numbers are
// exact: semantically equal literals render identically (1 == 1.0) while
// distinct large integers stay distinct past 2^53, where a float64 would
// collapse them.
func TestEventDecodeCanonicalNumericExactness(t *testing.T) {
	one, err := CanonicalJSON(mustRaw(t, `1`))
	if err != nil {
		t.Fatalf("canonical 1: %v", err)
	}
	oneDotZero, err := CanonicalJSON(mustRaw(t, `1.0`))
	if err != nil {
		t.Fatalf("canonical 1.0: %v", err)
	}
	if one != oneDotZero || one != "1" {
		t.Fatalf("canonical(1)=%q canonical(1.0)=%q, want both %q", one, oneDotZero, "1")
	}
	lo, err := CanonicalJSON(mustRaw(t, `9007199254740992`))
	if err != nil {
		t.Fatalf("canonical 2^53: %v", err)
	}
	hi, err := CanonicalJSON(mustRaw(t, `9007199254740993`))
	if err != nil {
		t.Fatalf("canonical 2^53+1: %v", err)
	}
	if lo == hi {
		t.Fatalf("distinct large integers collapsed to the same canonical form %q", lo)
	}
	if lo != "9007199254740992" || hi != "9007199254740993" {
		t.Fatalf("large integer canonicals = %q, %q, want the exact literals", lo, hi)
	}
	// A non-integer decimal renders as an exact trimmed decimal.
	half, err := CanonicalJSON(mustRaw(t, `0.5`))
	if err != nil {
		t.Fatalf("canonical 0.5: %v", err)
	}
	if half != "0.5" {
		t.Fatalf("canonical(0.5) = %q, want 0.5", half)
	}
}

func mustRaw(t *testing.T, s string) json.RawMessage {
	t.Helper()
	return json.RawMessage(s)
}
