// Package app freezes the v1 application contracts: the DTOs, stable
// errors and envelope shapes shared by the CLI, HTTP and MCP adapters.
// Adapters call the same services in-process and share these types; no
// adapter defines its own state semantics or masks failures as success.
package app

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

// SchemaVersion is the wire schema version of all v1 payloads.
const SchemaVersion = "1.0"

// ErrorCode is a stable, transport-visible failure code.
type ErrorCode string

// Stable v1 error codes.
const (
	ErrInvalidArgument    ErrorCode = "invalid_argument"
	ErrNotFound           ErrorCode = "not_found"
	ErrConflict           ErrorCode = "conflict"
	ErrIdentityConflict   ErrorCode = "identity_conflict"
	ErrDuplicateCandidate ErrorCode = "duplicate_candidate"
	ErrCycleDetected      ErrorCode = "cycle_detected"
	ErrUnauthorized       ErrorCode = "unauthorized"
	ErrForbidden          ErrorCode = "forbidden"
	ErrRateLimited        ErrorCode = "rate_limited"
	ErrLocked             ErrorCode = "locked"
	ErrSchemaMismatch     ErrorCode = "schema_mismatch"
	ErrStorageError       ErrorCode = "storage_error"
	ErrConfigError        ErrorCode = "config_error"
	ErrInternalError      ErrorCode = "internal_error"
)

// Valid reports whether the code is a defined v1 error code.
func (c ErrorCode) Valid() bool {
	switch c {
	case ErrInvalidArgument, ErrNotFound, ErrConflict, ErrIdentityConflict,
		ErrDuplicateCandidate, ErrCycleDetected, ErrUnauthorized, ErrForbidden,
		ErrRateLimited, ErrLocked, ErrSchemaMismatch, ErrStorageError,
		ErrConfigError, ErrInternalError:
		return true
	}
	return false
}

// HTTPStatus is the status used by the HTTP adapter for the code.
func (c ErrorCode) HTTPStatus() int {
	switch c {
	case ErrInvalidArgument:
		return 400
	case ErrNotFound:
		return 404
	case ErrUnauthorized:
		return 401
	case ErrForbidden:
		return 403
	case ErrConflict, ErrIdentityConflict, ErrDuplicateCandidate, ErrCycleDetected:
		return 409
	case ErrRateLimited:
		return 429
	case ErrLocked:
		return 503
	default:
		return 500
	}
}

// CLIExit is the process exit code used by the CLI adapter: 1 runtime,
// config or storage, 2 invalid input or conflict, 4 locked. Exit 3 is
// reserved for partial imports and is produced by the import use case, not
// by an error code.
func (c ErrorCode) CLIExit() int {
	switch c {
	case ErrInvalidArgument, ErrNotFound, ErrConflict, ErrIdentityConflict,
		ErrDuplicateCandidate, ErrCycleDetected:
		return 2
	case ErrLocked:
		return 4
	default:
		return 1
	}
}

// Error is the transport-visible failure object.
type Error struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	Retryable bool           `json:"retryable"`
}

// AppError is a stable-coded application failure. Audit context (request
// ID, actor, current revision) travels with the error for logging and
// retry prompts; the wire error always carries the real code.
type AppError struct {
	Code            ErrorCode
	Message         string
	Details         map[string]any
	Retryable       bool
	RequestID       string
	Actor           string
	CurrentRevision *int64
}

// Error implements the error interface.
func (e *AppError) Error() string {
	return string(e.Code) + ": " + e.Message
}

// NewAppError builds an AppError with the code's default retryability
// (locked is retryable, everything else is not).
func NewAppError(code ErrorCode, message string) *AppError {
	retryable := code == ErrLocked
	return &AppError{Code: code, Message: message, Retryable: retryable}
}

// WithCurrentRevision returns a copy annotated with the current revision
// so retries can prompt a reload.
func (e *AppError) WithCurrentRevision(revision int64) *AppError {
	cp := *e
	cp.CurrentRevision = &revision
	return &cp
}

// WireError renders the transport-visible error object; the current
// revision, when known, appears as details["current_revision"].
func (e *AppError) WireError() *Error {
	details := e.Details
	if e.CurrentRevision != nil {
		details = make(map[string]any, len(details)+1)
		for k, v := range e.Details {
			details[k] = v
		}
		details["current_revision"] = *e.CurrentRevision
	}
	return &Error{
		Code:      string(e.Code),
		Message:   e.Message,
		Details:   details,
		Retryable: e.Retryable,
	}
}

// Envelope is the single machine JSON document shared by all adapters.
// A success carries data and never an error; a failure carries the error
// and never data.
type Envelope struct {
	SchemaVersion string `json:"schema_version"`
	OK            bool   `json:"ok"`
	Data          *any   `json:"data,omitempty"`
	Error         *Error `json:"error,omitempty"`
}

// Success builds a success envelope. Views passed as data must be
// constructed with non-nil empty slices so unavailable collections
// serialize as [] rather than null.
func Success(data any) Envelope {
	d := data
	return Envelope{SchemaVersion: SchemaVersion, OK: true, Data: &d}
}

// Failure builds a failure envelope from a typed application error.
func Failure(err *AppError) Envelope {
	return Envelope{SchemaVersion: SchemaVersion, OK: false, Error: err.WireError()}
}

// ListPayload is the canonical paginated list data shape: opaque seek
// cursor, stable ordering and the total count before pagination.
type ListPayload struct {
	Items      []any   `json:"items"`
	NextCursor *string `json:"next_cursor"`
	Total      int64   `json:"total"`
}

// Actor identifies who performed an operation: the transport-set principal
// plus the asserted origin and identity.
type Actor struct {
	Principal string             `json:"principal"`
	Origin    domain.ActorOrigin `json:"origin"`
	Identity  string             `json:"identity,omitempty"`
}

// MutationBase carries the idempotency key, actor and expected revision
// accepted by every app mutation. The same key with a different payload is
// a conflict; failed validation does not reserve a key.
type MutationBase struct {
	RequestID        string `json:"request_id"`
	Actor            Actor  `json:"actor"`
	ExpectedRevision *int64 `json:"expected_revision,omitempty"`
}

// Title is an active sourced title of a work or edition. Unknown language
// is null, never an invented string.
type Title struct {
	ID         string           `json:"id"`
	Text       string           `json:"text"`
	Language   *string          `json:"language"`
	Kind       domain.TitleKind `json:"kind"`
	Preferred  bool             `json:"preferred"`
	Provenance string           `json:"provenance"`
}

// EditionSummary is one edition line inside a work view.
type EditionSummary struct {
	ID       string             `json:"id"`
	Kind     domain.EditionKind `json:"kind"`
	Language *string            `json:"language"`
	Revision int64              `json:"revision"`
}

// WorkView is the work detail payload: identity, active titles and
// edition summaries. Construct with non-nil slices.
type WorkView struct {
	ID         string           `json:"id"`
	Revision   int64            `json:"revision"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
	ArchivedAt *time.Time       `json:"archived_at"`
	Titles     []Title          `json:"titles"`
	Editions   []EditionSummary `json:"editions"`
}

// ProgressRecord is one effective progress observation. Position and total
// are exact decimal strings (fixed precision up to 3 fractional digits);
// a missing total is null, never zero or an estimate.
type ProgressRecord struct {
	ID           string              `json:"id"`
	Unit         domain.ProgressUnit `json:"unit"`
	Position     string              `json:"position"`
	Total        *string             `json:"total"`
	Label        *string             `json:"label"`
	ObservedAt   time.Time           `json:"observed_at"`
	RecordedAt   time.Time           `json:"recorded_at"`
	Actor        Actor               `json:"actor"`
	SupersedesID *string             `json:"supersedes_id"`
}

// ProgressByUnit freezes the five bounded units as explicit fields; a unit
// without an effective observation is null (omitted), never zero.
type ProgressByUnit struct {
	Chapter *ProgressRecord `json:"chapter,omitempty"`
	Volume  *ProgressRecord `json:"volume,omitempty"`
	Episode *ProgressRecord `json:"episode,omitempty"`
	Page    *ProgressRecord `json:"page,omitempty"`
	Percent *ProgressRecord `json:"percent,omitempty"`
}

// ConsumptionState is the current personal consumption observation.
type ConsumptionState struct {
	Value        domain.Consumption `json:"value"`
	Reason       *string            `json:"reason"`
	ObservedAt   time.Time          `json:"observed_at"`
	RecordedAt   time.Time          `json:"recorded_at"`
	Actor        Actor              `json:"actor"`
	SupersedesID *string            `json:"supersedes_id"`
}

// IntentionState is the current personal intention observation.
type IntentionState struct {
	Value        domain.Intention `json:"value"`
	Reason       *string          `json:"reason"`
	ObservedAt   time.Time        `json:"observed_at"`
	RecordedAt   time.Time        `json:"recorded_at"`
	Actor        Actor            `json:"actor"`
	SupersedesID *string          `json:"supersedes_id"`
}

// PublicationState is the current publication observation. A known claim
// carries its explicit source; unknown is an explicit value, not null.
type PublicationState struct {
	Value        domain.Publication `json:"value"`
	Source       *string            `json:"source"`
	ObservedAt   time.Time          `json:"observed_at"`
	RecordedAt   time.Time          `json:"recorded_at"`
	Provenance   string             `json:"provenance"`
	SupersedesID *string            `json:"supersedes_id"`
}

// RatingObservation is the current rating; a null value is an explicit
// clear, never zero.
type RatingObservation struct {
	Value        *int      `json:"value"`
	Comment      *string   `json:"comment"`
	ObservedAt   time.Time `json:"observed_at"`
	RecordedAt   time.Time `json:"recorded_at"`
	Actor        Actor     `json:"actor"`
	SupersedesID *string   `json:"supersedes_id"`
}

// ExternalReference is a sourced reference attached to an edition; it
// requires an external ID or an absolute http(s) URL.
type ExternalReference struct {
	ID            string               `json:"id"`
	Provider      string               `json:"provider"`
	ExternalID    *string              `json:"external_id"`
	URL           *string              `json:"url"`
	Role          domain.ReferenceRole `json:"role"`
	Authoritative bool                 `json:"authoritative"`
	Language      *string              `json:"language"`
	Provenance    string               `json:"provenance"`
	RetiredAt     *time.Time           `json:"retired_at"`
}

// Note is the current note revision on exactly one work or edition.
type Note struct {
	ID         string    `json:"id"`
	TargetID   string    `json:"target_id"`
	Text       string    `json:"text"`
	Spoiler    bool      `json:"spoiler"`
	Quote      *string   `json:"quote"`
	Location   *string   `json:"location"`
	Revision   int64     `json:"revision"`
	RecordedAt time.Time `json:"recorded_at"`
	Actor      Actor     `json:"actor"`
}

// EditionView is the edition detail payload. Unavailable collections are
// empty and unknown scalar facts are null. Construct with non-nil slices.
type EditionView struct {
	ID          string              `json:"id"`
	WorkID      string              `json:"work_id"`
	Kind        domain.EditionKind  `json:"kind"`
	Language    *string             `json:"language"`
	OtherLabel  *string             `json:"other_label"`
	Revision    int64               `json:"revision"`
	Titles      []Title             `json:"titles"`
	Consumption *ConsumptionState   `json:"consumption"`
	Intention   *IntentionState     `json:"intention"`
	Publication *PublicationState   `json:"publication"`
	Progress    ProgressByUnit      `json:"progress"`
	Rating      *RatingObservation  `json:"rating"`
	Tags        []string            `json:"tags"`
	References  []ExternalReference `json:"references"`
	Notes       []Note              `json:"notes"`
}

// ResolveOutcome is the outcome of ResolveAddition.
type ResolveOutcome string

// Resolve outcomes.
const (
	OutcomeReady      ResolveOutcome = "ready"
	OutcomeExisting   ResolveOutcome = "existing"
	OutcomeCandidates ResolveOutcome = "candidates"
)

// Valid reports whether the outcome is a defined v1 value.
func (o ResolveOutcome) Valid() bool {
	switch o {
	case OutcomeReady, OutcomeExisting, OutcomeCandidates:
		return true
	}
	return false
}

// UnmarshalJSON rejects unknown outcomes and null.
func (o *ResolveOutcome) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return fmt.Errorf("app: resolve outcome: unexpected null")
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v := ResolveOutcome(s)
	if !v.Valid() {
		return fmt.Errorf("app: resolve outcome: invalid value %q", s)
	}
	*o = v
	return nil
}

// Candidate is a possible identity match with its evidence; a match is
// never identity until the caller chooses.
type Candidate struct {
	WorkID    string   `json:"work_id"`
	EditionID *string  `json:"edition_id"`
	Evidence  []string `json:"evidence"`
}

// ResolveResult is the ResolveAddition payload. Candidates carry a
// short-lived token and a request fingerprint; stale decisions conflict.
type ResolveResult struct {
	Outcome            ResolveOutcome `json:"outcome"`
	WorkID             *string        `json:"work_id"`
	Candidates         []Candidate    `json:"candidates,omitempty"`
	CandidateToken     *string        `json:"candidate_token"`
	RequestFingerprint *string        `json:"request_fingerprint"`
}

// RecordProgressRequest is the canonical progress mutation request; it
// freezes the mutation base plus the progress payload shape.
type RecordProgressRequest struct {
	MutationBase
	EditionID    string              `json:"edition_id"`
	Unit         domain.ProgressUnit `json:"unit"`
	Position     string              `json:"position"`
	Total        *string             `json:"total"`
	Label        *string             `json:"label"`
	ObservedAt   time.Time           `json:"observed_at"`
	Reason       *string             `json:"reason"`
	SupersedesID *string             `json:"supersedes_id"`
}
