package app

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// Service runs the library use cases over one store handle.
type Service struct {
	db  *store.Database
	now func() time.Time
}

// NewService wires a service over the given store handle.
func NewService(db *store.Database) *Service {
	return &Service{db: db, now: time.Now}
}

// fingerprintPayload is the canonical addition payload a candidate decision
// binds to: the titles, edition attributes and references. The idempotency
// key, actor, choice, work and token are deliberately excluded: the key is
// the identity of the mutation, and the choice/work are decided later.
type fingerprintPayload struct {
	Titles     []store.TitleInput     `json:"titles"`
	Kind       *domain.EditionKind    `json:"kind"`
	Language   *string                `json:"language"`
	OtherLabel *string                `json:"other_label"`
	References []store.ReferenceInput `json:"references"`
}

// journalPayload extends the fingerprint with the decision parameters: the
// same key with a different choice or work is a different payload.
type journalPayload struct {
	fingerprintPayload
	WorkID *string `json:"work_id"`
	Choice *string `json:"choice"`
}

// fingerprint is the request fingerprint carried by a candidate token.
func fingerprint(req AdditionRequest) string {
	b, err := json.Marshal(fingerprintPayload{
		Titles:     req.Titles,
		Kind:       req.Kind,
		Language:   req.Language,
		OtherLabel: req.OtherLabel,
		References: req.References,
	})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// journalRequestHash is the canonical mutation hash stored in the journal.
func journalRequestHash(req AdditionRequest) string {
	b, err := json.Marshal(journalPayload{
		fingerprintPayload: fingerprintPayload{
			Titles:     req.Titles,
			Kind:       req.Kind,
			Language:   req.Language,
			OtherLabel: req.OtherLabel,
			References: req.References,
		},
		WorkID: req.WorkID,
		Choice: choiceString(req.Choice),
	})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func choiceString(c *AdditionChoice) *string {
	if c == nil {
		return nil
	}
	s := string(*c)
	return &s
}

// candidateTokenPayload binds a fingerprint and candidate set to a short
// decision window; the token is opaque base64url JSON.
type candidateTokenPayload struct {
	Fingerprint string   `json:"fingerprint"`
	Candidates  []string `json:"candidates"`
	ExpiresUnix int64    `json:"expires_at"`
}

// CandidateTTL bounds how long a candidate decision stays valid.
const CandidateTTL = 10 * time.Minute

// issueCandidateToken encodes a fresh decision token for the resolved
// candidates.
func issueCandidateToken(fp string, candidates []store.TitleCandidate, now time.Time) string {
	keys := make([]string, 0, len(candidates))
	for _, c := range candidates {
		keys = append(keys, candidateKeyOf(c))
	}
	sort.Strings(keys)
	b, _ := json.Marshal(candidateTokenPayload{
		Fingerprint: fp,
		Candidates:  keys,
		ExpiresUnix: now.Add(CandidateTTL).Unix(),
	})
	return base64.RawURLEncoding.EncodeToString(b)
}

// parseCandidateToken decodes and sanity-checks a decision token.
func parseCandidateToken(token string) (*candidateTokenPayload, error) {
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("app: candidate token: not valid base64url")
	}
	var p candidateTokenPayload
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("app: candidate token: malformed")
	}
	if p.Fingerprint == "" || len(p.Candidates) == 0 || p.ExpiresUnix <= 0 {
		return nil, fmt.Errorf("app: candidate token: malformed")
	}
	return &p, nil
}

// candidateKeyOf mirrors the store's canonical candidate key.
func candidateKeyOf(c store.TitleCandidate) string {
	if c.EditionID != nil {
		return c.WorkID + "|" + *c.EditionID
	}
	return c.WorkID
}

// canonicalCandidateSet orders the candidate keys of a store candidate list.
func canonicalCandidateSet(candidates []store.TitleCandidate) []string {
	keys := make([]string, 0, len(candidates))
	seen := map[string]bool{}
	for _, c := range candidates {
		k := candidateKeyOf(c)
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]string(nil), a...)
	bb := append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}

// mapStoreError maps store failures to stable-coded application errors.
func mapStoreError(err error, req MutationBase) *AppError {
	switch {
	case errors.Is(err, store.ErrLocked):
		return NewAppError(ErrLocked, "the database is locked by another process")
	case errors.Is(err, store.ErrWorkNotFound), errors.Is(err, store.ErrEditionNotFound), errors.Is(err, store.ErrConsumptionNotFound):
		return &AppError{Code: ErrNotFound, Message: err.Error(), RequestID: req.RequestID, Actor: req.Actor.Principal}
	case errors.Is(err, store.ErrRevisionMismatch):
		return &AppError{Code: ErrConflict, Message: "record changed; reload before retrying", RequestID: req.RequestID, Actor: req.Actor.Principal}
	case errors.Is(err, store.ErrSupersedeConflict):
		return &AppError{Code: ErrConflict, Message: "the named observation was already corrected; reload before retrying", RequestID: req.RequestID, Actor: req.Actor.Principal}
	case errors.Is(err, store.ErrIdentityConflict):
		return &AppError{Code: ErrIdentityConflict, Message: err.Error(), RequestID: req.RequestID, Actor: req.Actor.Principal}
	case errors.Is(err, store.ErrStaleDecision):
		return &AppError{Code: ErrConflict, Message: "the candidate decision is stale; resolve again", RequestID: req.RequestID, Actor: req.Actor.Principal}
	case errors.Is(err, store.ErrNoWorkSelected):
		return &AppError{Code: ErrInvalidArgument, Message: err.Error(), RequestID: req.RequestID, Actor: req.Actor.Principal}
	case errors.Is(err, store.ErrInvalidTitle), errors.Is(err, store.ErrInvalidEdition),
		errors.Is(err, store.ErrInvalidConsumption), errors.Is(err, store.ErrInvalidReference),
		errors.Is(err, store.ErrInvalidURL), errors.Is(err, store.ErrNoTitle):
		return &AppError{Code: ErrInvalidArgument, Message: err.Error(), RequestID: req.RequestID, Actor: req.Actor.Principal}
	default:
		return &AppError{Code: ErrStorageError, Message: "storage failure", RequestID: req.RequestID, Actor: req.Actor.Principal}
	}
}

// invalidArgument builds an invalid-argument application error.
func invalidArgument(message string, req MutationBase) *AppError {
	return &AppError{Code: ErrInvalidArgument, Message: message, RequestID: req.RequestID, Actor: req.Actor.Principal}
}

// conflict builds a conflict application error.
func conflict(message string, req MutationBase) *AppError {
	return &AppError{Code: ErrConflict, Message: message, RequestID: req.RequestID, Actor: req.Actor.Principal}
}

// identityConflict builds an identity-conflict application error.
func identityConflict(message string, req MutationBase) *AppError {
	return &AppError{Code: ErrIdentityConflict, Message: message, RequestID: req.RequestID, Actor: req.Actor.Principal}
}

func isBlank(s string) bool { return strings.TrimSpace(s) == "" }
