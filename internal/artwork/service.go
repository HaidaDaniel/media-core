package artwork

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/HaidaDaniel/media-core/internal/store"
)

var (
	ErrDisabled       = errors.New("artwork: discovery is disabled")
	ErrInvalidInput   = errors.New("artwork: invalid request")
	ErrUnknownPreview = errors.New("artwork: preview not found or expired")
)

type previewEntry struct {
	Candidate ScoredCandidate
	Data      []byte
	MIME      string
	Expires   time.Time
}

// PreviewStore holds at most a few freshly fetched candidate images. Tokens
// are random and short-lived; no user-supplied URL can be resolved through
// this store.
type PreviewStore struct {
	mu    sync.Mutex
	items map[string]previewEntry
	order []string
	max   int
	ttl   time.Duration
}

func NewPreviewStore() *PreviewStore {
	return &PreviewStore{items: map[string]previewEntry{}, max: 24, ttl: 10 * time.Minute}
}

func (p *PreviewStore) Put(c ScoredCandidate, data []byte, mime string, now time.Time) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	token := hex.EncodeToString(b[:])
	p.mu.Lock()
	defer p.mu.Unlock()
	p.items[token] = previewEntry{Candidate: c, Data: data, MIME: mime, Expires: now.Add(p.ttl)}
	p.order = append(p.order, token)
	for len(p.order) > p.max {
		oldest := p.order[0]
		p.order = p.order[1:]
		delete(p.items, oldest)
	}
	return token
}

func (p *PreviewStore) Get(token string, now time.Time) (previewEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.items[token]
	if !ok || now.After(entry.Expires) {
		delete(p.items, token)
		return previewEntry{}, false
	}
	return entry, true
}

// Service orchestrates the explicit discovery flows. It never starts a
// background worker: every method runs within one user request.
type Service struct {
	registry *Registry
	client   *safeClient
	cache    *Cache
	previews *PreviewStore
	db       *store.Database
	now      func() time.Time
}

func NewService(db *store.Database, registry *Registry, client *safeClient, cache *Cache) *Service {
	return &Service{registry: registry, client: client, cache: cache, previews: NewPreviewStore(), db: db, now: time.Now}
}

func (s *Service) Providers() []ProviderInfo { return s.registry.Providers() }

func validateSearchInput(in SearchInput) error {
	if len([]rune(in.Title)) < 1 || len([]rune(in.Title)) > 200 {
		return fmt.Errorf("%w: title must be 1..200 characters", ErrInvalidInput)
	}
	if !in.Kind.Valid() || in.Kind == KindOther {
		return fmt.Errorf("%w: unsupported media kind", ErrInvalidInput)
	}
	if len(in.Alternatives) > 5 || len(in.References) > 5 {
		return fmt.Errorf("%w: at most 5 alternatives and 5 references", ErrInvalidInput)
	}
	for _, list := range [][]string{in.Alternatives, in.References} {
		for _, v := range list {
			if len([]rune(v)) > 200 {
				return fmt.Errorf("%w: alternative or reference exceeds 200 characters", ErrInvalidInput)
			}
		}
	}
	return nil
}

// Search queries the providers that support the requested kind, matches the
// results against every known title and attachment, and prepares local image
// previews for the first few candidates. Provider failures are isolated; only
// a total failure is reported, and even then the caller may continue without
// a cover.
func (s *Service) Search(ctx context.Context, in SearchInput) ([]ScoredCandidate, error) {
	canonical, ok := ParseKind(string(in.Kind))
	if !ok {
		return []ScoredCandidate{}, nil
	}
	in.Kind = canonical
	if err := validateSearchInput(in); err != nil {
		return nil, err
	}
	providers := s.registry.forKind(in.Kind)
	if len(providers) == 0 {
		return []ScoredCandidate{}, nil
	}
	type providerResult struct {
		found []Candidate
		err   error
	}
	results := make([]providerResult, len(providers))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)
	for i, provider := range providers {
		wg.Add(1)
		go func(index int, p Provider) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			found, err := p.Search(ctx, in.Title, in.Kind, searchLimit)
			results[index] = providerResult{found: found, err: err}
		}(i, provider)
	}
	wg.Wait()
	all := []Candidate{}
	failures := 0
	for _, result := range results {
		if result.err != nil {
			failures++
			continue
		}
		all = append(all, result.found...)
	}
	scored := Match(in, all)
	for i := range scored {
		if boost := referenceBoost(scored[i].SourceURL, in.References); boost {
			scored[i].Score = 1
			scored[i].Reason = "existing external reference"
		}
	}
	if len(scored) > 8 {
		scored = scored[:8]
	}
	previewCount := 3
	if len(scored) < previewCount {
		previewCount = len(scored)
	}
	var previewWG sync.WaitGroup
	previewSem := make(chan struct{}, previewCount)
	for i := 0; i < previewCount; i++ {
		if scored[i].ImageURL == "" {
			continue
		}
		hosts := imageHostsFor(scored[i].Provider)
		if len(hosts) == 0 {
			scored[i].ImageURL = ""
			continue
		}
		previewWG.Add(1)
		go func(index int, hosts map[string]bool) {
			defer previewWG.Done()
			previewSem <- struct{}{}
			defer func() { <-previewSem }()
			res, err := s.client.fetch(ctx, scored[index].Provider+"-image", scored[index].ImageURL, hosts, maxImageBytes, "image/jpeg,image/png,image/gif")
			if err != nil {
				scored[index].ImageURL = ""
				return
			}
			if _, err := s.cache.validateImage(res.Body, res.ContentType); err != nil {
				scored[index].ImageURL = ""
				return
			}
			token := s.previews.Put(scored[index], res.Body, res.ContentType, s.now())
			if token == "" {
				scored[index].ImageURL = ""
				return
			}
			scored[index].PreviewToken = token
			scored[index].ImageURL = "/api/v1/artwork/preview/" + token + "/image"
		}(i, hosts)
	}
	previewWG.Wait()
	selectable := scored[:0]
	for _, candidate := range scored {
		if candidate.PreviewToken != "" {
			selectable = append(selectable, candidate)
		}
	}
	scored = selectable
	if len(scored) == 0 && failures == len(providers) {
		return []ScoredCandidate{}, ErrUpstream
	}
	return scored, nil
}

func referenceBoost(source string, references []string) bool {
	for _, raw := range references {
		if raw == "" {
			continue
		}
		if source == raw {
			return true
		}
	}
	return false
}

// Select stores a user-confirmed candidate. The candidate is identified only
// by a short-lived preview token; the server never accepts a URL from the
// browser. An existing user selection is replaced only on explicit request.
func (s *Service) Select(ctx context.Context, workID, editionID, token string, replace bool) (store.ArtworkRecord, error) {
	if workID == "" {
		return store.ArtworkRecord{}, fmt.Errorf("%w: work id is required", ErrInvalidInput)
	}
	entry, ok := s.previews.Get(token, s.now())
	if !ok {
		return store.ArtworkRecord{}, ErrUnknownPreview
	}
	c := entry.Candidate
	ref := Ref(c.Provider, c.SourceURL)
	protected, err := s.db.AllLocalRefs(ctx)
	if err != nil {
		return store.ArtworkRecord{}, err
	}
	keep := map[string]bool{}
	for _, r := range protected {
		keep[r] = true
	}
	if _, err := s.cache.Put(ref, entry.Data, entry.MIME, keep); err != nil {
		return store.ArtworkRecord{}, err
	}
	var edition *string
	if editionID != "" {
		edition = &editionID
	}
	now := s.now()
	return s.db.SaveArtwork(ctx, store.ArtworkInput{
		WorkID: workID, EditionID: edition, Provider: c.Provider,
		ExternalID: c.ExternalID, Title: c.Title, SourceURL: c.SourceURL,
		AttributionURL: c.AttributionURL, Kind: c.Kind, LocalRef: ref,
		MIMEType: entry.MIME, ByteSize: int64(len(entry.Data)),
		Provenance: "artwork-search:" + c.Provider,
	}, now, replace)
}

// Preview returns the bytes of a short-lived search preview.
func (s *Service) Preview(token string) ([]byte, string, bool) {
	entry, ok := s.previews.Get(token, s.now())
	if !ok {
		return nil, "", false
	}
	return entry.Data, entry.MIME, true
}

// MissingItem is one work processed by the explicit Find missing covers
// action. Its status describes what the user can do next.
type MissingItem struct {
	Work       store.MissingArtworkWork `json:"work"`
	Status     string                   `json:"status"`
	Candidates []ScoredCandidate        `json:"candidates"`
}

// MissingPage is one keyset page of the explicit Find missing covers action.
type MissingPage struct {
	Items      []MissingItem
	NextCursor string
	Total      int
}

type missingCursorPayload struct {
	V         int    `json:"v"`
	CreatedAt string `json:"created_at"`
	ID        string `json:"id"`
}

func encodeMissingCursor(c store.MissingArtworkCursor) string {
	raw, err := json.Marshal(missingCursorPayload{V: 1, CreatedAt: store.InstantKey(c.CreatedAt), ID: c.ID})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeMissingCursor(raw string) (*store.MissingArtworkCursor, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw) > 1024 {
		return nil, fmt.Errorf("%w: cursor is too long", ErrInvalidInput)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: cursor is malformed", ErrInvalidInput)
	}
	var payload missingCursorPayload
	if err := json.Unmarshal(decoded, &payload); err != nil || payload.V != 1 || payload.ID == "" {
		return nil, fmt.Errorf("%w: cursor is malformed", ErrInvalidInput)
	}
	created, err := time.Parse(time.RFC3339Nano, payload.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("%w: cursor is malformed", ErrInvalidInput)
	}
	return &store.MissingArtworkCursor{CreatedAt: created, ID: payload.ID}, nil
}

// FindMissing processes one small keyset batch of works. It is called only by
// an explicit user action; no page render invokes it.
func (s *Service) FindMissing(ctx context.Context, cursor string, limit int) (MissingPage, error) {
	if limit < 1 || limit > 5 {
		return MissingPage{}, fmt.Errorf("%w: limit must be 1..5", ErrInvalidInput)
	}
	after, err := decodeMissingCursor(cursor)
	if err != nil {
		return MissingPage{}, err
	}
	works, total, err := s.db.ListWorksMissingArtwork(ctx, after, limit)
	if err != nil {
		return MissingPage{}, err
	}
	items := make([]MissingItem, 0, len(works))
	for _, w := range works {
		item := MissingItem{Work: w, Candidates: []ScoredCandidate{}, Status: "not_found"}
		if ctx.Err() != nil {
			item.Status = "timeout"
			items = append(items, item)
			continue
		}
		kind, ok := ParseKind(w.Kind)
		if !ok {
			item.Status = "error"
			items = append(items, item)
			continue
		}
		candidates, err := s.Search(ctx, SearchInput{Title: w.Title, Kind: kind})
		if err != nil {
			item.Status = "error"
			if ctx.Err() != nil {
				item.Status = "timeout"
			}
			items = append(items, item)
			continue
		}
		switch {
		case len(candidates) == 0:
			item.Status = "not_found"
		case IsConfident(candidates):
			item.Status = "suggested"
			item.Candidates = candidates[:1]
		default:
			item.Status = "ambiguous"
			item.Candidates = candidates
		}
		items = append(items, item)
	}
	next := ""
	if len(works) == limit {
		last := works[len(works)-1]
		next = encodeMissingCursor(store.MissingArtworkCursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	return MissingPage{Items: items, NextCursor: next, Total: total}, nil
}

// ParseKind maps the domain edition kinds onto the bounded artwork provider
// families. It reports false for formats with no verified provider instead
// of guessing across media types.
func ParseKind(raw string) (Kind, bool) {
	switch Kind(raw) {
	case KindAnime:
		return KindAnime, true
	case KindManga, "manhwa", "manhua":
		return KindManga, true
	case KindBook, "web_novel", "light_novel":
		return KindBook, true
	case KindMovie:
		return KindMovie, true
	case KindTV:
		return KindTV, true
	}
	return "", false
}

// ClearCache removes regenerable image bytes without touching SQLite rows.
func (s *Service) ClearCache() (int, int64, error) { return s.cache.Clear() }

// CacheStats reports cache usage for diagnostics.
func (s *Service) CacheStats() (int, int64, error) { return s.cache.Stats() }

// ArtworkImage loads the bytes of a stored selection by its cache reference.
func (s *Service) ArtworkImage(ctx context.Context, artworkID string) ([]byte, string, error) {
	record, err := s.db.ArtworkByID(ctx, artworkID)
	if err != nil {
		return nil, "", err
	}
	return s.cache.Open(record.LocalRef)
}
