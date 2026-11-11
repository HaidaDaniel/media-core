package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

// Identity, reference and journal failures. Application services map these
// to the stable wire error codes; the store stays transport-agnostic.
var (
	ErrIdentityConflict = errors.New("store: external identity already reserved")
	ErrInvalidReference = errors.New("store: invalid reference")
	ErrInvalidURL       = errors.New("store: invalid url")
	ErrStaleDecision    = errors.New("store: candidate decision is stale")
	ErrNoWorkSelected   = errors.New("store: no work selected for the decision")
)

// NormalizeURL normalizes an absolute http(s) URL for identity comparison:
// the scheme and host are lowercased, default ports and the fragment are
// removed, and path case, trailing slash, query order/values and
// percent-encoding are preserved. http and https stay distinct, as do
// mirrors and tracking query strings; every other scheme is rejected.
func NormalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("%w: scheme %q is not http(s)", ErrInvalidURL, u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%w: missing host", ErrInvalidURL)
	}
	host := strings.ToLower(u.Host)
	if h, p, err := net.SplitHostPort(host); err == nil {
		if (scheme == "http" && p == "80") || (scheme == "https" && p == "443") {
			host = h
		}
	}
	normalized := scheme + "://" + host + u.EscapedPath()
	if u.RawQuery != "" {
		normalized += "?" + u.RawQuery
	}
	return normalized, nil
}

// ReferenceInput is one external identity attached at creation: a provider
// plus an external ID, or an absolute http(s) URL. Exactly one of ExternalID
// or URL must be set; an external ID requires its provider.
type ReferenceInput struct {
	Provider   *string
	ExternalID *string
	URL        *string
}

// validateReference checks the reference shape and returns the normalized
// URL when the identity is a URL.
func validateReference(in ReferenceInput) (normURL *string, err error) {
	hasID, hasURL := in.ExternalID != nil, in.URL != nil
	if hasID == hasURL {
		return nil, fmt.Errorf("%w: exactly one of external_id or url is required", ErrInvalidReference)
	}
	if hasID {
		if in.Provider == nil || strings.TrimSpace(*in.Provider) == "" {
			return nil, fmt.Errorf("%w: external id requires a provider", ErrInvalidReference)
		}
		if len(*in.Provider) > 64 {
			return nil, fmt.Errorf("%w: provider exceeds 64 characters", ErrInvalidReference)
		}
		if strings.TrimSpace(*in.ExternalID) == "" {
			return nil, fmt.Errorf("%w: external id is empty", ErrInvalidReference)
		}
		return nil, nil
	}
	normalized, err := NormalizeURL(*in.URL)
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

// IdentityMatch is one active external identity reservation.
type IdentityMatch struct {
	EditionID     string
	Provider      *string
	ExternalID    *string
	NormalizedURL *string
}

// queryer is satisfied by *sql.DB and *sql.Tx so identity and candidate
// checks run identically outside and inside the write transaction.
type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (d *Database) findIdentity(ctx context.Context, q queryer, in ReferenceInput) (*IdentityMatch, error) {
	var (
		m    IdentityMatch
		prov sql.NullString
		ext  sql.NullString
		norm sql.NullString
	)
	if in.ExternalID != nil {
		err := q.QueryRowContext(ctx, `
			SELECT edition_id, provider, external_id, normalized_url
			FROM external_identities
			WHERE retired_at IS NULL AND provider = ? AND external_id = ?
			LIMIT 1`, *in.Provider, *in.ExternalID).
			Scan(&m.EditionID, &prov, &ext, &norm)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("store: find external identity: %w", err)
		}
	} else {
		normalized, err := NormalizeURL(*in.URL)
		if err != nil {
			return nil, err
		}
		err = q.QueryRowContext(ctx, `
			SELECT edition_id, provider, external_id, normalized_url
			FROM external_identities
			WHERE retired_at IS NULL AND normalized_url = ?
			LIMIT 1`, normalized).
			Scan(&m.EditionID, &prov, &ext, &norm)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("store: find external identity: %w", err)
		}
	}
	if prov.Valid {
		m.Provider = &prov.String
	}
	if ext.Valid {
		m.ExternalID = &ext.String
	}
	if norm.Valid {
		m.NormalizedURL = &norm.String
	}
	return &m, nil
}

// FindExternalIdentity returns the active reservation matching the
// reference identity, or nil when unreserved.
func (d *Database) FindExternalIdentity(ctx context.Context, in ReferenceInput) (*IdentityMatch, error) {
	return d.findIdentity(ctx, d.db, in)
}

// TitleCandidate is one active title whose search key exactly equals the
// requested key. A candidate is a possible match, never identity.
type TitleCandidate struct {
	WorkID    string
	EditionID *string
	Text      string
	Kind      domain.TitleKind
	Language  *string
	Target    string // "work" or "edition"
}

func titleCandidatesOn(ctx context.Context, q queryer, key string) ([]TitleCandidate, error) {
	out := []TitleCandidate{}
	rows, err := q.QueryContext(ctx, `
		SELECT work_id, text, kind, language
		FROM work_titles
		WHERE retired_at IS NULL AND search_key = ?
		ORDER BY work_id`, key)
	if err != nil {
		return nil, fmt.Errorf("store: find work title candidates: %w", err)
	}
	for rows.Next() {
		var (
			c        TitleCandidate
			language sql.NullString
			kind     string
		)
		if err := rows.Scan(&c.WorkID, &c.Text, &kind, &language); err != nil {
			rows.Close()
			return nil, fmt.Errorf("store: scan work title candidate: %w", err)
		}
		c.Kind = domain.TitleKind(kind)
		c.Target = "work"
		if language.Valid {
			c.Language = &language.String
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	rows, err = q.QueryContext(ctx, `
		SELECT e.work_id, et.edition_id, et.text, et.kind, et.language
		FROM edition_titles et
		JOIN editions e ON e.id = et.edition_id
		WHERE et.retired_at IS NULL AND et.search_key = ?
		ORDER BY e.work_id, et.edition_id`, key)
	if err != nil {
		return nil, fmt.Errorf("store: find edition title candidates: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			c        TitleCandidate
			language sql.NullString
			kind     string
			edition  string
		)
		if err := rows.Scan(&c.WorkID, &edition, &c.Text, &kind, &language); err != nil {
			return nil, fmt.Errorf("store: scan edition title candidate: %w", err)
		}
		c.Kind = domain.TitleKind(kind)
		c.EditionID = &edition
		c.Target = "edition"
		if language.Valid {
			c.Language = &language.String
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// FindTitleCandidates lists the active work and edition titles whose search
// key exactly equals key; exact normalized titles are candidate matches.
func (d *Database) FindTitleCandidates(ctx context.Context, key string) ([]TitleCandidate, error) {
	return titleCandidatesOn(ctx, d.db, key)
}

// candidateKey is the canonical identity of one candidate inside a
// candidate decision.
func candidateKey(c TitleCandidate) string {
	if c.EditionID != nil {
		return c.WorkID + "|" + *c.EditionID
	}
	return c.WorkID
}

// JournalEntry is one stored idempotency reservation with its result.
type JournalEntry struct {
	RequestID   string
	Operation   string
	RequestHash string
	ResultJSON  string
	Actor       string
	CreatedAt   time.Time
}

// JournalInput is the reservation recorded in the same transaction as the
// changes; a failed validation never records a key. ResultJSON is filled by
// the store with the created-entry result before the row is inserted, so
// the result and the key are committed together.
type JournalInput struct {
	RequestID   string
	Operation   string
	RequestHash string
	ResultJSON  string
	Actor       string
}

// entryResultJSON is the created-entry result recorded in the journal and
// returned verbatim on exact replay.
func entryResultJSON(workID string, workRevision int64, editionID *string) string {
	r := map[string]any{"created": true, "work_id": workID, "work_revision": workRevision}
	if editionID != nil {
		r["edition_id"] = *editionID
	}
	b, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	return string(b)
}

// LookupJournal returns the stored entry for a request ID, or nil when the
// key is unused.
func (d *Database) LookupJournal(ctx context.Context, requestID string) (*JournalEntry, error) {
	var (
		e         JournalEntry
		createdAt string
	)
	err := d.db.QueryRowContext(ctx,
		`SELECT operation, request_hash, result_json, actor, created_at
		 FROM mutation_journal WHERE request_id = ?`, requestID).
		Scan(&e.Operation, &e.RequestHash, &e.ResultJSON, &e.Actor, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: read journal: %w", err)
	}
	e.RequestID = requestID
	e.CreatedAt, err = parseInstant(createdAt)
	if err != nil {
		return nil, fmt.Errorf("store: parse journal created_at: %w", err)
	}
	return &e, nil
}

func recordJournalTx(ctx context.Context, tx *sql.Tx, j JournalInput, now time.Time) error {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO mutation_journal (request_id, operation, request_hash, result_json, actor, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		j.RequestID, j.Operation, j.RequestHash, j.ResultJSON, j.Actor, instantUTC(now)); err != nil {
		return fmt.Errorf("store: record journal: %w", err)
	}
	return nil
}

// MatchedEdition carries the identity-matched edition's identity facts for
// the application's contradiction check.
type MatchedEdition struct {
	EditionID string
	WorkID    string
	Kind      domain.EditionKind
	Language  *string
}

func matchedEditionOn(ctx context.Context, q queryer, editionID string) (*MatchedEdition, error) {
	var (
		m        MatchedEdition
		kind     string
		language sql.NullString
	)
	err := q.QueryRowContext(ctx,
		`SELECT id, work_id, kind, language FROM editions WHERE id = ?`, editionID).
		Scan(&m.EditionID, &m.WorkID, &kind, &language)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrEditionNotFound, editionID)
	}
	if err != nil {
		return nil, fmt.Errorf("store: read matched edition: %w", err)
	}
	m.Kind = domain.EditionKind(kind)
	if language.Valid {
		m.Language = &language.String
	}
	return &m, nil
}

// CandidateDecision is a short-lived decision on a resolved candidate set.
// The store verifies the expiry and that the recomputed set still matches;
// the application verifies the request fingerprint.
type CandidateDecision struct {
	Candidates  []string
	ExpiresUnix int64
	NowUnix     int64
}

// checkDecision verifies the decision against the candidates recomputed
// inside the write transaction.
func checkDecision(dec *CandidateDecision, candidates []TitleCandidate) error {
	if dec.NowUnix >= dec.ExpiresUnix {
		return fmt.Errorf("%w: the candidate decision expired", ErrStaleDecision)
	}
	want := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		want[candidateKey(c)] = true
	}
	got := make(map[string]bool, len(dec.Candidates))
	for _, k := range dec.Candidates {
		got[k] = true
	}
	if len(want) != len(got) {
		return fmt.Errorf("%w: the candidate set changed", ErrStaleDecision)
	}
	for k := range want {
		if !got[k] {
			return fmt.Errorf("%w: the candidate set changed", ErrStaleDecision)
		}
	}
	return nil
}

// EntryOutcome distinguishes the results of one atomic entry attempt.
type EntryOutcome int

const (
	// EntryCreated: the work and optional edition were created with their
	// identity reservations and journal row.
	EntryCreated EntryOutcome = iota
	// EntryIdentityMatch: an active identity matched; nothing was written.
	EntryIdentityMatch
	// EntryCandidates: exact title candidates block a create without a
	// decision; nothing was written.
	EntryCandidates
)

// EntryAttempt is one atomic addition attempt. The identity and candidate
// rechecks and every write share one write transaction.
type EntryAttempt struct {
	Titles     []TitleInput
	Kind       *domain.EditionKind
	Language   *string
	OtherLabel *string
	References []ReferenceInput
	Journal    *JournalInput
	// WorkID set: add the edition under this existing work (a kind is
	// required); otherwise a new work is created.
	WorkID *string
	// Decision verifies a short-lived candidate decision against the
	// recomputed set; nil with BlockOnCandidates reports candidates instead
	// of creating.
	Decision *CandidateDecision
	// BlockOnCandidates makes an exact title match report candidates
	// instead of creating when no decision is supplied.
	BlockOnCandidates bool
}

// EntryResult reports one atomic attempt.
type EntryResult struct {
	Outcome    EntryOutcome
	Identity   *IdentityMatch
	Matched    *MatchedEdition
	Candidates []TitleCandidate
	Work       Work
	Edition    *Edition
}

// validateAttempt rejects malformed attempts before any database access.
func validateAttempt(a EntryAttempt) error {
	if len(a.Titles) == 0 && len(a.References) == 0 && a.WorkID == nil {
		return fmt.Errorf("%w: a title, an identity or an explicit work is required", ErrNoTitle)
	}
	preferred := 0
	for _, t := range a.Titles {
		if err := validateTitleInput(t); err != nil {
			return err
		}
		if t.Preferred {
			preferred++
		}
	}
	if preferred > 1 {
		return fmt.Errorf("%w: at most one preferred title", ErrInvalidTitle)
	}
	if a.Kind != nil {
		if err := domain.ValidateEditionAttrs(*a.Kind, a.Language, a.OtherLabel); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidEdition, err)
		}
	}
	for _, r := range a.References {
		if _, err := validateReference(r); err != nil {
			return err
		}
	}
	if a.WorkID != nil && a.Kind == nil {
		return fmt.Errorf("%w: an edition requires a kind", ErrInvalidEdition)
	}
	return nil
}

// AttemptEntry runs one atomic addition attempt in a single write
// transaction: it rechecks active identity reservations and exact title
// candidates, and either reports the match or creates the work and optional
// edition, reserves the identities, and records the journal row. Identity
// and candidate rechecks always run, so a concurrent reservation or title
// between resolve and create is caught in the same transaction as the
// writes.
func (d *Database) AttemptEntry(ctx context.Context, now time.Time, a EntryAttempt) (EntryResult, error) {
	if err := validateAttempt(a); err != nil {
		return EntryResult{}, err
	}
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return EntryResult{}, err
	}
	defer tx.Rollback()
	nowS := instantUTC(now)

	// 1. An active identity reservation always wins.
	for _, ref := range a.References {
		m, err := d.findIdentity(ctx, tx, ref)
		if err != nil {
			return EntryResult{}, err
		}
		if m != nil {
			matched, err := matchedEditionOn(ctx, tx, m.EditionID)
			if err != nil {
				return EntryResult{}, err
			}
			return EntryResult{Outcome: EntryIdentityMatch, Identity: m, Matched: matched}, nil
		}
	}

	// 2. Exact title candidates, deduplicated across the requested titles.
	candidates := []TitleCandidate{}
	seen := map[string]bool{}
	for _, t := range a.Titles {
		key := domain.SearchKey(t.Text)
		found, err := titleCandidatesOn(ctx, tx, key)
		if err != nil {
			return EntryResult{}, err
		}
		for _, c := range found {
			if !seen[candidateKey(c)] {
				seen[candidateKey(c)] = true
				candidates = append(candidates, c)
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].WorkID != candidates[j].WorkID {
			return candidates[i].WorkID < candidates[j].WorkID
		}
		return candidateKey(candidates[i]) < candidateKey(candidates[j])
	})
	if len(candidates) > 0 {
		if a.Decision == nil {
			if a.BlockOnCandidates {
				return EntryResult{Outcome: EntryCandidates, Candidates: candidates}, nil
			}
			return EntryResult{}, fmt.Errorf("%w: an exact title match requires a candidate decision", ErrStaleDecision)
		}
		if err := checkDecision(a.Decision, candidates); err != nil {
			return EntryResult{}, err
		}
	} else if a.Decision != nil {
		// The decision was issued for a non-empty set; a concurrent
		// retirement emptied it, so it is stale.
		return EntryResult{}, fmt.Errorf("%w: the candidate set changed", ErrStaleDecision)
	}

	// 3. Writes. Reaching here means a creation is about to happen, so
	// references need an edition to attach to; a pure identity lookup above
	// already returned for a matched identity.
	if len(a.References) > 0 && a.Kind == nil && a.WorkID == nil {
		return EntryResult{}, fmt.Errorf("%w: references require an explicit edition kind", ErrInvalidReference)
	}
	var (
		workID       string
		edition      *Edition
		workRevision int64
	)
	if a.WorkID != nil {
		var revision int64
		err := tx.QueryRowContext(ctx,
			`SELECT revision FROM works WHERE id = ?`, *a.WorkID).Scan(&revision)
		if errors.Is(err, sql.ErrNoRows) {
			return EntryResult{}, fmt.Errorf("%w: %s", ErrWorkNotFound, *a.WorkID)
		}
		if err != nil {
			return EntryResult{}, fmt.Errorf("store: read work for edition: %w", err)
		}
		edition, err = createEditionTx(ctx, tx, *a.WorkID, *a.Kind, a.Language, a.OtherLabel, a.Titles, nowS)
		if err != nil {
			return EntryResult{}, err
		}
		if err := bumpWorkRevision(tx, ctx, *a.WorkID, revision, now); err != nil {
			return EntryResult{}, err
		}
		workRevision = revision + 1
		workID = *a.WorkID
	} else {
		if len(a.Titles) == 0 {
			return EntryResult{}, fmt.Errorf("%w: a new entry requires at least one title", ErrNoTitle)
		}
		workID, err = domain.NewUUID()
		if err != nil {
			return EntryResult{}, err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO works (id, revision, created_at, updated_at) VALUES (?, 0, ?, ?)`,
			workID, nowS, nowS); err != nil {
			return EntryResult{}, fmt.Errorf("store: insert work: %w", err)
		}
		preferred := 0
		for _, t := range a.Titles {
			if t.Preferred {
				preferred++
			}
		}
		for i, t := range a.Titles {
			id, err := domain.NewUUID()
			if err != nil {
				return EntryResult{}, err
			}
			isPreferred := t.Preferred
			if preferred == 0 && i == 0 {
				isPreferred = true
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO work_titles (id, work_id, seq, text, search_key, language, kind, preferred, provenance, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, workID, i+1, t.Text, domain.SearchKey(t.Text), t.Language, string(t.Kind), boolInt(isPreferred), t.Provenance, nowS); err != nil {
				return EntryResult{}, fmt.Errorf("store: insert work title: %w", err)
			}
		}
		if a.Kind != nil {
			edition, err = createEditionTx(ctx, tx, workID, *a.Kind, a.Language, a.OtherLabel, a.Titles, nowS)
			if err != nil {
				return EntryResult{}, err
			}
			if err := bumpWorkRevision(tx, ctx, workID, 0, now); err != nil {
				return EntryResult{}, err
			}
			workRevision = 1
		}
	}
	if len(a.References) > 0 {
		if edition == nil {
			return EntryResult{}, fmt.Errorf("%w: references require an edition", ErrInvalidReference)
		}
		for _, ref := range a.References {
			normURL, err := validateReference(ref)
			if err != nil {
				return EntryResult{}, err
			}
			id, err := domain.NewUUID()
			if err != nil {
				return EntryResult{}, err
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO external_identities (id, edition_id, provider, external_id, normalized_url, created_at)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				id, edition.ID, ref.Provider, ref.ExternalID, normURL, nowS); err != nil {
				return EntryResult{}, fmt.Errorf("%w: %v", ErrIdentityConflict, err)
			}
		}
	}
	if a.Journal != nil {
		j := *a.Journal
		editionID := (*string)(nil)
		if edition != nil {
			id := edition.ID
			editionID = &id
		}
		j.ResultJSON = entryResultJSON(workID, workRevision, editionID)
		if err := recordJournalTx(ctx, tx, j, now); err != nil {
			return EntryResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return EntryResult{}, fmt.Errorf("store: commit entry: %w", err)
	}
	work, err := d.GetWork(ctx, workID)
	if err != nil {
		return EntryResult{}, err
	}
	res := EntryResult{Outcome: EntryCreated, Work: work, Candidates: candidates}
	if edition != nil {
		e, err := d.GetEdition(ctx, edition.ID)
		if err != nil {
			return EntryResult{}, err
		}
		res.Edition = &e
	}
	return res, nil
}

// createEditionTx inserts one edition with its titles under a work inside
// the caller's transaction; the edition starts at revision 0.
func createEditionTx(ctx context.Context, tx *sql.Tx, workID string, kind domain.EditionKind, language, otherLabel *string, titles []TitleInput, nowS string) (*Edition, error) {
	editionID, err := domain.NewUUID()
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO editions (id, work_id, kind, other_label, language, revision, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
		editionID, workID, string(kind), otherLabel, language, nowS, nowS); err != nil {
		return nil, fmt.Errorf("store: insert edition: %w", err)
	}
	preferred := 0
	for _, t := range titles {
		if t.Preferred {
			preferred++
		}
	}
	for i, t := range titles {
		id, err := domain.NewUUID()
		if err != nil {
			return nil, err
		}
		isPreferred := t.Preferred
		if preferred == 0 && i == 0 {
			isPreferred = true
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO edition_titles (id, edition_id, seq, text, search_key, language, kind, preferred, provenance, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, editionID, i+1, t.Text, domain.SearchKey(t.Text), t.Language, string(t.Kind), boolInt(isPreferred), t.Provenance, nowS); err != nil {
			return nil, fmt.Errorf("store: insert edition title: %w", err)
		}
	}
	return &Edition{ID: editionID, WorkID: workID, Kind: kind, Language: language, OtherLabel: otherLabel}, nil
}
