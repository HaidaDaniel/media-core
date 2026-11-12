package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// AdditionChoice is the caller's explicit resolution of a candidate match.
type AdditionChoice string

// Choice values.
const (
	ChoiceUseExisting    AdditionChoice = "use_existing"
	ChoiceAddEdition     AdditionChoice = "add_edition"
	ChoiceCreateDistinct AdditionChoice = "create_distinct"
)

// Valid reports whether the choice is a defined v1 value.
func (c AdditionChoice) Valid() bool {
	switch c {
	case ChoiceUseExisting, ChoiceAddEdition, ChoiceCreateDistinct:
		return true
	}
	return false
}

// AdditionRequest is the manual library-add payload shared by resolve and
// create. Resolve uses the payload; create additionally uses the mutation
// base and, when candidates were found, an explicit choice with its token.
// Title-only requests are always creatable offline; a URL or external ID
// without a matching identity still requires a title.
type AdditionRequest struct {
	MutationBase
	Titles         []store.TitleInput
	WorkID         *string
	Kind           *domain.EditionKind
	Language       *string
	OtherLabel     *string
	References     []store.ReferenceInput
	Choice         *AdditionChoice
	CandidateToken *string
}

// AdditionResult is the create outcome. Created is false with reason
// already_exists when the addition resolved to an existing entry; no
// personal state is implied.
type AdditionResult struct {
	Created      bool    `json:"created"`
	Reason       *string `json:"reason,omitempty"`
	WorkID       string  `json:"work_id"`
	WorkRevision int64   `json:"work_revision"`
	EditionID    *string `json:"edition_id"`
}

func alreadyExists(workID string, workRevision int64, editionID *string) AdditionResult {
	reason := "already_exists"
	return AdditionResult{Created: false, Reason: &reason, WorkID: workID, WorkRevision: workRevision, EditionID: editionID}
}

// validateAddition checks the payload before any database access. Failed
// validation never reserves an idempotency key.
func validateAddition(req AdditionRequest) error {
	if len(req.Titles) == 0 && len(req.References) == 0 && req.WorkID == nil {
		return fmt.Errorf("a title, a known identity or an explicit work is required")
	}
	preferred := 0
	for _, t := range req.Titles {
		if strings.TrimSpace(t.Text) == "" {
			return fmt.Errorf("title text is empty")
		}
		if !t.Kind.Valid() {
			return fmt.Errorf("unknown title kind %q", t.Kind)
		}
		if t.Language != nil && strings.TrimSpace(*t.Language) == "" {
			return fmt.Errorf("title language must be null or non-empty")
		}
		if strings.TrimSpace(t.Provenance) == "" {
			return fmt.Errorf("title provenance is required")
		}
		if t.Preferred {
			preferred++
		}
	}
	if preferred > 1 {
		return fmt.Errorf("at most one preferred title")
	}
	if req.Kind != nil {
		if err := domain.ValidateEditionAttrs(*req.Kind, req.Language, req.OtherLabel); err != nil {
			return err
		}
	}
	for _, r := range req.References {
		hasID, hasURL := r.ExternalID != nil, r.URL != nil
		if hasID == hasURL {
			return fmt.Errorf("a reference requires exactly one of external_id or url")
		}
		if hasID {
			if r.Provider == nil || strings.TrimSpace(*r.Provider) == "" {
				return fmt.Errorf("an external id requires a provider")
			}
			if strings.TrimSpace(*r.ExternalID) == "" {
				return fmt.Errorf("external id is empty")
			}
		} else if _, err := store.NormalizeURL(*r.URL); err != nil {
			return err
		}
	}
	if req.WorkID != nil && (req.Choice == nil || (*req.Choice != ChoiceAddEdition && *req.Choice != ChoiceUseExisting)) {
		return fmt.Errorf("work_id is used with the add_edition or use_existing choice")
	}
	if req.Choice != nil {
		if !req.Choice.Valid() {
			return fmt.Errorf("unknown choice %q", *req.Choice)
		}
		if req.CandidateToken == nil {
			return fmt.Errorf("a choice requires the candidate token from the resolve result")
		}
	} else if req.CandidateToken != nil {
		return fmt.Errorf("a candidate token requires a choice")
	}
	if req.Choice != nil && *req.Choice == ChoiceAddEdition {
		if req.WorkID == nil {
			return fmt.Errorf("the add_edition choice requires an explicit work_id")
		}
		if req.Kind == nil {
			return fmt.Errorf("the add_edition choice requires an explicit edition kind")
		}
	}
	return nil
}

// checkIdentityContradiction compares the request's explicit identity fields
// with the matched edition. Unknown (null) language is not a contradiction;
// kind and work are always concrete.
func checkIdentityContradiction(req AdditionRequest, m *store.MatchedEdition) *AppError {
	if req.WorkID != nil && *req.WorkID != m.WorkID {
		return identityConflict("the identity belongs to another work", req.MutationBase)
	}
	if req.Kind != nil && *req.Kind != m.Kind {
		return identityConflict("the identity belongs to an edition of a different kind", req.MutationBase)
	}
	if req.Language != nil && m.Language != nil && *req.Language != *m.Language {
		return identityConflict("the identity belongs to an edition of a different language", req.MutationBase)
	}
	return nil
}

// existingResult builds the no-write result for an identity match.
func existingResult(ctx context.Context, s *Service, m *store.MatchedEdition) (AdditionResult, error) {
	w, err := s.db.GetWork(ctx, m.WorkID)
	if err != nil {
		return AdditionResult{}, mapStoreError(err, MutationBase{})
	}
	return alreadyExists(w.ID, w.Revision, &m.EditionID), nil
}

// ResolveAddition resolves a manual addition without writing. An active
// exact identity wins (existing, or identity_conflict when the request's
// explicit fields contradict it); otherwise exact normalized title matches
// return candidates with a short-lived decision token; otherwise the
// addition is ready to create.
func (s *Service) ResolveAddition(ctx context.Context, req AdditionRequest) (ResolveResult, error) {
	if err := validateAddition(req); err != nil {
		return ResolveResult{}, invalidArgument(err.Error(), req.MutationBase)
	}
	for _, ref := range req.References {
		m, err := s.db.FindExternalIdentity(ctx, ref)
		if err != nil {
			return ResolveResult{}, mapStoreError(err, req.MutationBase)
		}
		if m == nil {
			continue
		}
		matched, err := s.matchedEdition(ctx, m.EditionID)
		if err != nil {
			return ResolveResult{}, mapStoreError(err, req.MutationBase)
		}
		if err := checkIdentityContradiction(req, matched); err != nil {
			return ResolveResult{}, err
		}
		w, err := s.db.GetWork(ctx, matched.WorkID)
		if err != nil {
			return ResolveResult{}, mapStoreError(err, req.MutationBase)
		}
		return ResolveResult{Outcome: OutcomeExisting, WorkID: &w.ID}, nil
	}
	if len(req.Titles) > 0 {
		candidates, err := s.collectCandidates(ctx, req)
		if err != nil {
			return ResolveResult{}, mapStoreError(err, req.MutationBase)
		}
		if len(candidates) > 0 {
			fp := fingerprint(req)
			return ResolveResult{
				Outcome:            OutcomeCandidates,
				Candidates:         toAppCandidates(candidates),
				CandidateToken:     strPtr(issueCandidateToken(fp, candidates, s.now())),
				RequestFingerprint: &fp,
			}, nil
		}
	}
	if len(req.Titles) == 0 {
		return ResolveResult{}, invalidArgument("a title is required when the identity is not known", req.MutationBase)
	}
	if len(req.References) > 0 && req.Kind == nil {
		// Ready means the addition can be created now; references need an
		// edition to attach to. A pure identity lookup above already
		// returned for a matched identity.
		return ResolveResult{}, invalidArgument("references require an explicit edition kind", req.MutationBase)
	}
	fp := fingerprint(req)
	return ResolveResult{Outcome: OutcomeReady, RequestFingerprint: &fp}, nil
}

// matchedEdition loads the identity facts of an edition.
func (s *Service) matchedEdition(ctx context.Context, editionID string) (*store.MatchedEdition, error) {
	e, err := s.db.GetEdition(ctx, editionID)
	if err != nil {
		return nil, err
	}
	return &store.MatchedEdition{EditionID: e.ID, WorkID: e.WorkID, Kind: e.Kind, Language: e.Language}, nil
}

// collectCandidates lists the distinct exact title candidates of the
// request's titles.
func (s *Service) collectCandidates(ctx context.Context, req AdditionRequest) ([]store.TitleCandidate, error) {
	out := []store.TitleCandidate{}
	seen := map[string]bool{}
	for _, t := range req.Titles {
		found, err := s.db.FindTitleCandidates(ctx, domain.SearchKey(t.Text))
		if err != nil {
			return nil, err
		}
		for _, c := range found {
			if !seen[candidateKeyOf(c)] {
				seen[candidateKeyOf(c)] = true
				out = append(out, c)
			}
		}
	}
	return out, nil
}

// toAppCandidates renders store candidates as the frozen Candidate DTO with
// human-readable evidence.
func toAppCandidates(candidates []store.TitleCandidate) []Candidate {
	out := make([]Candidate, 0, len(candidates))
	for _, c := range candidates {
		evidence := evidenceFor(c)
		out = append(out, Candidate{WorkID: c.WorkID, EditionID: c.EditionID, Evidence: evidence})
	}
	return out
}

func evidenceFor(c store.TitleCandidate) []string {
	detail := string(c.Kind)
	if c.Language != nil {
		detail += ", " + *c.Language
	}
	if c.Target == "edition" {
		return []string{fmt.Sprintf("edition title %q (%s)", c.Text, detail)}
	}
	return []string{fmt.Sprintf("work title %q (%s)", c.Text, detail)}
}

func strPtr(s string) *string { return &s }

// CreateLibraryEntry creates a library entry (or reports an existing one)
// with exact identity replay. The canonical request hash and the result are
// stored in the same transaction as the changes; an exact retry returns the
// stored result, the same key with a different payload conflicts, and a
// failed validation never reserves the key. Candidate decisions are
// rechecked inside the write transaction; stale decisions conflict without
// writes.
func (s *Service) CreateLibraryEntry(ctx context.Context, req AdditionRequest) (AdditionResult, error) {
	if strings.TrimSpace(req.RequestID) == "" {
		return AdditionResult{}, invalidArgument("request_id is required", req.MutationBase)
	}
	if strings.TrimSpace(req.Actor.Principal) == "" {
		return AdditionResult{}, invalidArgument("actor.principal is required", req.MutationBase)
	}
	// Exact replay: the stored result wins, including after a timeout or a
	// restart. The same key with a different payload is a conflict.
	hash := journalRequestHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return AdditionResult{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		if entry.RequestHash != hash {
			return AdditionResult{}, conflict("the idempotency key was already used with a different payload", req.MutationBase)
		}
		return replayResult(entry, req.MutationBase)
	}
	if err := validateAddition(req); err != nil {
		return AdditionResult{}, invalidArgument(err.Error(), req.MutationBase)
	}
	fp := fingerprint(req)
	if req.Choice != nil {
		dec, err := parseCandidateToken(*req.CandidateToken)
		if err != nil {
			return AdditionResult{}, invalidArgument(err.Error(), req.MutationBase)
		}
		if dec.Fingerprint != fp {
			return AdditionResult{}, conflict("the request changed since the candidates were resolved", req.MutationBase)
		}
		if s.now().Unix() >= dec.ExpiresUnix {
			// A concurrent identical create may have committed after the
			// pre-check lookup; its stored result wins over the expiry.
			if found, r, lerr := s.lookupStoredResult(ctx, req, hash); lerr != nil {
				return AdditionResult{}, lerr
			} else if found {
				return r, nil
			}
			return AdditionResult{}, conflict("the candidate decision expired; resolve again", req.MutationBase)
		}
		switch *req.Choice {
		case ChoiceUseExisting:
			return s.useExisting(ctx, req, dec)
		case ChoiceAddEdition, ChoiceCreateDistinct:
			workID := req.WorkID
			if *req.Choice == ChoiceCreateDistinct {
				workID = nil
			}
			return s.createEntry(ctx, req, &store.CandidateDecision{
				Candidates:  dec.Candidates,
				ExpiresUnix: dec.ExpiresUnix,
				NowUnix:     s.now().Unix(),
			}, workID)
		}
	}
	return s.createEntry(ctx, req, nil, nil)
}

// replayResult decodes a stored journal result.
func replayResult(entry *store.JournalEntry, req MutationBase) (AdditionResult, error) {
	var r AdditionResult
	if err := json.Unmarshal([]byte(entry.ResultJSON), &r); err != nil {
		return AdditionResult{}, &AppError{Code: ErrStorageError, Message: "stored mutation result is corrupt", RequestID: req.RequestID, Actor: req.Actor.Principal}
	}
	return r, nil
}

// lookupStoredResult settles a same-key result committed after this call's
// pre-check lookup: a matching hash replays the stored result, a different
// hash is a key-collision conflict, and no row reports found=false.
func (s *Service) lookupStoredResult(ctx context.Context, req AdditionRequest, hash string) (bool, AdditionResult, error) {
	entry, err := s.db.LookupJournal(ctx, req.RequestID)
	if err != nil {
		return false, AdditionResult{}, mapStoreError(err, req.MutationBase)
	}
	if entry == nil {
		return false, AdditionResult{}, nil
	}
	if entry.RequestHash != hash {
		return false, AdditionResult{}, conflict("the idempotency key was already used with a different payload", req.MutationBase)
	}
	r, err := replayResult(entry, req.MutationBase)
	return true, r, err
}

// useExisting confirms a candidate work still exists and returns it without
// writing. The outcome reserves the request key so an exact retry replays
// and a different payload on the same key conflicts.
func (s *Service) useExisting(ctx context.Context, req AdditionRequest, dec *candidateTokenPayload) (AdditionResult, error) {
	candidates, err := s.collectCandidates(ctx, req)
	if err != nil {
		return AdditionResult{}, mapStoreError(err, req.MutationBase)
	}
	if !sameStringSet(dec.Candidates, canonicalCandidateSet(candidates)) {
		return AdditionResult{}, conflict("the candidate set changed; resolve again", req.MutationBase)
	}
	workID, selErr := selectWork(req, candidates)
	if selErr != nil {
		return AdditionResult{}, selErr
	}
	w, err := s.db.GetWork(ctx, workID)
	if err != nil {
		return AdditionResult{}, mapStoreError(err, req.MutationBase)
	}
	res := alreadyExists(w.ID, w.Revision, nil)
	b, err := json.Marshal(res)
	if err != nil {
		return AdditionResult{}, &AppError{Code: ErrInternalError, Message: "encode existing result", RequestID: req.RequestID, Actor: req.Actor.Principal}
	}
	hash := journalRequestHash(req)
	if err := s.db.ReserveJournal(ctx, s.now(), store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "create_library_entry",
		RequestHash: hash,
		ResultJSON:  string(b),
		Actor:       req.Actor.Principal,
	}); err != nil {
		// A concurrent identical call may have won the key first.
		if found, r, lerr := s.lookupStoredResult(ctx, req, hash); lerr != nil {
			return AdditionResult{}, lerr
		} else if found {
			return r, nil
		}
		return AdditionResult{}, mapStoreError(err, req.MutationBase)
	}
	return res, nil
}

// selectWork picks the work a use_existing decision refers to: an explicit
// work_id must be among the candidates, otherwise exactly one distinct
// candidate work may exist.
func selectWork(req AdditionRequest, candidates []store.TitleCandidate) (string, *AppError) {
	works := map[string]bool{}
	for _, c := range candidates {
		works[c.WorkID] = true
	}
	if req.WorkID != nil {
		if works[*req.WorkID] {
			return *req.WorkID, nil
		}
		return "", conflict("the selected work is no longer a candidate; resolve again", req.MutationBase)
	}
	for w := range works {
		if len(works) == 1 {
			return w, nil
		}
	}
	return "", invalidArgument("work_id is required to disambiguate the candidates", req.MutationBase)
}

// createEntry runs the atomic store attempt and maps its outcome. The
// journal reservation (key, canonical hash, result) is written in the same
// transaction as the changes; a concurrent identical create that wins the
// journal key is recovered as a replay of the stored result.
func (s *Service) createEntry(ctx context.Context, req AdditionRequest, dec *store.CandidateDecision, workID *string) (AdditionResult, error) {
	hash := journalRequestHash(req)
	res, err := s.db.AttemptEntry(ctx, s.now(), store.EntryAttempt{
		Titles:     req.Titles,
		Kind:       req.Kind,
		Language:   req.Language,
		OtherLabel: req.OtherLabel,
		References: req.References,
		Journal: &store.JournalInput{
			RequestID:   req.RequestID,
			Operation:   "create_library_entry",
			RequestHash: hash,
			Actor:       req.Actor.Principal,
			Origin:      string(req.Actor.Origin),
			Identity:    req.Actor.Identity,
		},
		WorkID:            workID,
		Decision:          dec,
		BlockOnCandidates: dec == nil,
	})
	if err != nil {
		return s.settleAttempt(ctx, req, hash, err)
	}
	switch res.Outcome {
	case store.EntryCreated:
		r := AdditionResult{Created: true, WorkID: res.Work.ID, WorkRevision: res.Work.Revision}
		if res.Edition != nil {
			id := res.Edition.ID
			r.EditionID = &id
		}
		return r, nil
	case store.EntryIdentityMatch:
		if err := checkIdentityContradiction(req, res.Matched); err != nil {
			return AdditionResult{}, err
		}
		// A concurrent identical create may have committed after the
		// pre-check lookup; its stored result is the exact replay and
		// wins over the existing report.
		if found, r, lerr := s.lookupStoredResult(ctx, req, hash); lerr != nil {
			return AdditionResult{}, lerr
		} else if found {
			return r, nil
		}
		return existingResult(ctx, s, res.Matched)
	case store.EntryCandidates:
		// The same replay settlement applies when the in-transaction
		// candidate recheck meets the winner's committed titles.
		if found, r, lerr := s.lookupStoredResult(ctx, req, hash); lerr != nil {
			return AdditionResult{}, lerr
		} else if found {
			return r, nil
		}
		return AdditionResult{}, &AppError{
			Code:      ErrDuplicateCandidate,
			Message:   "exact title matches exist; resolve the candidates and choose",
			RequestID: req.RequestID,
			Actor:     req.Actor.Principal,
		}
	}
	return AdditionResult{}, mapStoreError(fmt.Errorf("store: unexpected entry outcome"), req.MutationBase)
}

// settleAttempt consults the journal after a failed attempt: a concurrent
// commit of the same key replays its stored result, a different payload on
// the same key is a key-collision conflict, and anything else maps the
// attempt error.
func (s *Service) settleAttempt(ctx context.Context, req AdditionRequest, hash string, err error) (AdditionResult, error) {
	found, r, lerr := s.lookupStoredResult(ctx, req, hash)
	if lerr != nil {
		return AdditionResult{}, lerr
	}
	if found {
		return r, nil
	}
	return AdditionResult{}, mapStoreError(err, req.MutationBase)
}
