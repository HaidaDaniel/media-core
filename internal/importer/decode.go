// Package importer decodes and validates Release Watcher import envelopes
// into immutable events. It is pure: no database, config or state access.
// The Release Watcher contract is authoritative; this package pins the
// version 1.0 file/stdin envelope shape and the idempotency equality rule.
package importer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// SupportedSchemaMajor is the only major schema version this build imports.
// A different major is rejected before any write; nothing is persisted.
const SupportedSchemaMajor = 1

// DefaultNamespace is the stable producer namespace used when a caller does
// not supply one. It is the identity of the Release Watcher producer.
const DefaultNamespace = "release-watcher"

// DefaultEnvelopeBytes bounds the total decoded envelope. A larger input is
// rejected before any write.
const DefaultEnvelopeBytes int64 = 64 << 20

// DefaultMaxEvents bounds the number of events in one envelope. A larger
// batch is rejected before any write; a producer pages with next_cursor.
const DefaultMaxEvents = 10000

// EnvelopeError is a decode-time failure with a stable code so the
// application layer can map it to the contract error. Every decode error
// means the whole envelope writes nothing.
type EnvelopeError struct {
	Code    string
	Message string
}

func (e *EnvelopeError) Error() string { return e.Message }

// Decode error codes.
const (
	CodeBadJSON           = "bad_json"
	CodeTooLarge          = "too_large"
	CodeTooManyEvents     = "too_many_events"
	CodeMissingSchema     = "missing_schema"
	CodeBadSchema         = "bad_schema"
	CodeUnsupportedSchema = "unsupported_schema"
	CodeNotOK             = "not_ok"
	CodeMissingData       = "missing_data"
	CodeMissingEventID    = "missing_event_id"
	CodeMissingEventType  = "missing_event_type"
)

// Envelope is the Release Watcher v1.0 import envelope. The raw event bytes
// are preserved so the store can keep the exact source payload.
type Envelope struct {
	SchemaVersion string        `json:"schema_version"`
	OK            bool          `json:"ok"`
	Data          *EnvelopeData `json:"data"`
}

// EnvelopeData is the envelope payload: an opaque resume cursor and the
// bounded event batch.
type EnvelopeData struct {
	NextCursor *string     `json:"next_cursor"`
	Events     []eventWire `json:"events"`
}

// Decoded is a validated envelope ready to import: the envelope identity and
// the decoded events, each carrying its exact raw bytes and canonical form.
type Decoded struct {
	SchemaVersion string
	NextCursor    *string
	Events        []Event
}

// Event is one decoded, validated Release Watcher event. The typed fields
// are optional and additive; a missing value stays nil and the raw payload
// remains the source of truth, so unknown additive fields are preserved.
type Event struct {
	SchemaVersion   string  `json:"schema_version"`
	EventID         string  `json:"event_id"`
	EventType       string  `json:"event_type"`
	PublicationID   *string `json:"publication_id,omitempty"`
	ExternalWorkRef *string `json:"external_work_ref,omitempty"`
	ReleaseID       *string `json:"release_id,omitempty"`
	ReleaseNumber   *string `json:"release_number,omitempty"`
	ReleaseTitle    *string `json:"release_title,omitempty"`
	ReleaseURL      *string `json:"release_url,omitempty"`
	CurrentCount    *int    `json:"current_count,omitempty"`
	CountDelta      *int    `json:"count_delta,omitempty"`
	SourceTime      *string `json:"source_time,omitempty"`
	DetectionTime   *string `json:"detection_time,omitempty"`
	WorkHint        *string `json:"work_hint,omitempty"`
	EditionHint     *string `json:"edition_hint,omitempty"`

	// Raw is the exact event object as received, including any unknown
	// additive fields and unknown event types.
	Raw json.RawMessage `json:"-"`
	// Canonical is the canonical JSON form of the event object, used for
	// idempotency equality. It ignores object key order and whitespace.
	Canonical string `json:"-"`
}

// eventWire is the per-event decode shape. Unknown fields are dropped here
// but preserved in Event.Raw.
type eventWire struct {
	SchemaVersion   string  `json:"schema_version"`
	EventID         string  `json:"event_id"`
	EventType       string  `json:"event_type"`
	PublicationID   *string `json:"publication_id"`
	ExternalWorkRef *string `json:"external_work_ref"`
	ReleaseID       *string `json:"release_id"`
	ReleaseNumber   *string `json:"release_number"`
	ReleaseTitle    *string `json:"release_title"`
	ReleaseURL      *string `json:"release_url"`
	CurrentCount    *int    `json:"current_count"`
	CountDelta      *int    `json:"count_delta"`
	SourceTime      *string `json:"source_time"`
	DetectionTime   *string `json:"detection_time"`
	WorkHint        *string `json:"work_hint"`
	EditionHint     *string `json:"edition_hint"`
}

// envelopeWire is the raw decode shape that keeps each event as exact bytes.
type envelopeWire struct {
	SchemaVersion string `json:"schema_version"`
	OK            bool   `json:"ok"`
	Data          *struct {
		NextCursor *string           `json:"next_cursor"`
		Events     []json.RawMessage `json:"events"`
	} `json:"data"`
}

// Decode reads and validates one Release Watcher import envelope from r.
// Total input is bounded to maxBytes and the event count to maxEvents; a
// bound exceeded, a wrong major, ok=false, missing data, or a structurally
// invalid event is a hard error that writes nothing.
func Decode(r io.Reader, maxBytes int64, maxEvents int) (*Decoded, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultEnvelopeBytes
	}
	if maxEvents <= 0 {
		maxEvents = DefaultMaxEvents
	}
	buf, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, &EnvelopeError{Code: CodeBadJSON, Message: "read envelope: " + err.Error()}
	}
	if int64(len(buf)) > maxBytes {
		return nil, &EnvelopeError{Code: CodeTooLarge, Message: fmt.Sprintf("envelope exceeds %d bytes", maxBytes)}
	}
	var env envelopeWire
	if err := json.Unmarshal(buf, &env); err != nil {
		return nil, &EnvelopeError{Code: CodeBadJSON, Message: "parse envelope: " + err.Error()}
	}
	if err := checkMajor(env.SchemaVersion); err != nil {
		return nil, err
	}
	if !env.OK {
		return nil, &EnvelopeError{Code: CodeNotOK, Message: "envelope ok is false"}
	}
	if env.Data == nil {
		return nil, &EnvelopeError{Code: CodeMissingData, Message: "envelope data is missing"}
	}
	if len(env.Data.Events) > maxEvents {
		return nil, &EnvelopeError{Code: CodeTooManyEvents, Message: fmt.Sprintf("envelope has %d events; limit is %d", len(env.Data.Events), maxEvents)}
	}
	decoded := &Decoded{SchemaVersion: env.SchemaVersion, NextCursor: env.Data.NextCursor}
	decoded.Events = make([]Event, 0, len(env.Data.Events))
	for i, raw := range env.Data.Events {
		ev, err := decodeEvent(raw, i)
		if err != nil {
			return nil, err
		}
		decoded.Events = append(decoded.Events, ev)
	}
	return decoded, nil
}

// DecodeBytes is Decode for an in-memory envelope, used by tests and the
// application layer.
func DecodeBytes(b []byte, maxBytes int64, maxEvents int) (*Decoded, error) {
	return Decode(strings.NewReader(string(b)), maxBytes, maxEvents)
}

// decodeEvent validates one raw event and computes its canonical form.
func decodeEvent(raw json.RawMessage, index int) (Event, error) {
	var w eventWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return Event{}, &EnvelopeError{Code: CodeBadJSON, Message: fmt.Sprintf("event %d is not a JSON object: %v", index, err)}
	}
	if strings.TrimSpace(w.EventID) == "" {
		return Event{}, &EnvelopeError{Code: CodeMissingEventID, Message: fmt.Sprintf("event %d is missing event_id", index)}
	}
	if strings.TrimSpace(w.EventType) == "" {
		return Event{}, &EnvelopeError{Code: CodeMissingEventType, Message: fmt.Sprintf("event %d is missing event_type", index)}
	}
	if err := checkMajor(w.SchemaVersion); err != nil {
		return Event{}, &EnvelopeError{Code: err.(*EnvelopeError).Code, Message: fmt.Sprintf("event %d: %s", index, err.(*EnvelopeError).Message)}
	}
	canonical, err := CanonicalJSON(raw)
	if err != nil {
		return Event{}, &EnvelopeError{Code: CodeBadJSON, Message: fmt.Sprintf("event %d canonical form: %v", index, err)}
	}
	return Event{
		SchemaVersion:   w.SchemaVersion,
		EventID:         w.EventID,
		EventType:       w.EventType,
		PublicationID:   w.PublicationID,
		ExternalWorkRef: w.ExternalWorkRef,
		ReleaseID:       w.ReleaseID,
		ReleaseNumber:   w.ReleaseNumber,
		ReleaseTitle:    w.ReleaseTitle,
		ReleaseURL:      w.ReleaseURL,
		CurrentCount:    w.CurrentCount,
		CountDelta:      w.CountDelta,
		SourceTime:      w.SourceTime,
		DetectionTime:   w.DetectionTime,
		WorkHint:        w.WorkHint,
		EditionHint:     w.EditionHint,
		Raw:             raw,
		Canonical:       canonical,
	}, nil
}

// checkMajor validates a semantic version string and requires the supported
// major. An empty or malformed version is a decode failure.
func checkMajor(version string) error {
	if version == "" {
		return &EnvelopeError{Code: CodeMissingSchema, Message: "schema_version is missing"}
	}
	major, ok := schemaMajor(version)
	if !ok {
		return &EnvelopeError{Code: CodeBadSchema, Message: fmt.Sprintf("schema_version %q is not a semantic version", version)}
	}
	if major != SupportedSchemaMajor {
		return &EnvelopeError{Code: CodeUnsupportedSchema, Message: fmt.Sprintf("schema major %d is not supported; only %d is", major, SupportedSchemaMajor)}
	}
	return nil
}

// schemaMajor extracts the major component of a semantic version string.
func schemaMajor(version string) (int, bool) {
	parts := strings.SplitN(version, ".", 2)
	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return 0, false
	}
	return major, true
}

// CanonicalJSON returns the canonical form of a JSON value: object keys
// sorted, no insignificant whitespace, and numeric values normalized exactly.
// It ignores object key order and whitespace so semantically identical
// payloads compare equal, and it is the idempotency equality rule. Numbers
// are kept exact (a float64 would collapse distinct integers past 2^53)
// while semantically equal literals (1 and 1.0) render identically.
func CanonicalJSON(raw json.RawMessage) (string, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return "", err
	}
	return canonical(v)
}

// canonical renders one decoded JSON value in canonical form.
func canonical(v any) (string, error) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return "", err
			}
			vb, err := canonical(t[k])
			if err != nil {
				return "", err
			}
			b.Write(kb)
			b.WriteByte(':')
			b.WriteString(vb)
		}
		b.WriteByte('}')
		return b.String(), nil
	case []any:
		var b strings.Builder
		b.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			s, err := canonical(item)
			if err != nil {
				return "", err
			}
			b.WriteString(s)
		}
		b.WriteByte(']')
		return b.String(), nil
	case nil:
		return "null", nil
	case json.Number:
		return canonicalNumber(t)
	default:
		s, err := json.Marshal(t)
		if err != nil {
			return "", err
		}
		return string(s), nil
	}
}

// canonicalNumber renders one JSON number in an exact canonical form. It
// parses to a big.Rat so distinct large integers stay distinct while
// semantically equal literals (1 and 1.0) render identically.
func canonicalNumber(n json.Number) (string, error) {
	rat, ok := new(big.Rat).SetString(n.String())
	if !ok {
		return n.String(), nil
	}
	if rat.Denom().Int64() == 1 {
		return rat.Num().String(), nil
	}
	return ratToDecimal(rat), nil
}

// ratToDecimal renders the exact finite decimal for a rational whose
// denominator is a product of twos and fives — always true for a JSON number
// literal — trimming redundant trailing zeros.
func ratToDecimal(rat *big.Rat) string {
	num := new(big.Int).Set(rat.Num())
	den := new(big.Int).Set(rat.Denom())
	neg := num.Sign() < 0
	if neg {
		num.Abs(num)
	}
	two := big.NewInt(2)
	five := big.NewInt(5)
	d := new(big.Int).Set(den)
	var a, b int
	for new(big.Int).Mod(d, two).Sign() == 0 {
		d.Div(d, two)
		a++
	}
	for new(big.Int).Mod(d, five).Sign() == 0 {
		d.Div(d, five)
		b++
	}
	scale := a
	if b > scale {
		scale = b
	}
	scaled := new(big.Int).Set(num)
	if scale > a {
		scaled.Mul(scaled, new(big.Int).Exp(two, big.NewInt(int64(scale-a)), nil))
	}
	if scale > b {
		scaled.Mul(scaled, new(big.Int).Exp(five, big.NewInt(int64(scale-b)), nil))
	}
	s := scaled.String()
	if scale > 0 {
		for len(s) <= scale {
			s = "0" + s
		}
		intPart := s[:len(s)-scale]
		fracPart := strings.TrimRight(s[len(s)-scale:], "0")
		s = intPart
		if fracPart != "" {
			s += "." + fracPart
		}
	}
	if neg && s != "0" {
		s = "-" + s
	}
	return s
}
