// Package artwork implements explicit, provider-based cover discovery. All
// network access is server-side, allowlisted and bounded: the browser never
// receives a provider URL for a saved cover and users never submit URLs.
package artwork

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

// Kind is the media format used to keep anime, manga and books separate.
type Kind string

const (
	KindAnime Kind = "anime"
	KindManga Kind = "manga"
	KindBook  Kind = "book"
	KindMovie Kind = "movie"
	KindTV    Kind = "tv"
	KindOther Kind = "other"
)

func (k Kind) Valid() bool {
	switch k {
	case KindAnime, KindManga, KindBook, KindMovie, KindTV, KindOther:
		return true
	}
	return false
}

// Candidate is one provider result. ImageURL is a provider-supplied URL that
// only the server may fetch; it is replaced by a local preview URL before the
// candidate is sent to the browser.
type Candidate struct {
	Provider          string
	ExternalID        string
	Title             string
	AlternativeTitles []string
	Kind              Kind
	SourceURL         string
	AttributionURL    string
	ImageURL          string
}

// Provider is one verified upstream adapter.
type Provider interface {
	ID() string
	Label() string
	Kinds() []Kind
	AttributionURL() string
	Search(ctx context.Context, title string, kind Kind, limit int) ([]Candidate, error)
}

// ProviderInfo is the public provider catalog shown in the UI.
type ProviderInfo struct {
	ID             string   `json:"id"`
	Label          string   `json:"label"`
	Kinds          []string `json:"kinds"`
	AttributionURL string   `json:"attribution_url"`
	Required       bool     `json:"required"`
}

// ScoredCandidate is a matched provider result ready for the wire.
type ScoredCandidate struct {
	Provider          string   `json:"provider"`
	ExternalID        string   `json:"external_id"`
	Title             string   `json:"title"`
	AlternativeTitles []string `json:"alternative_titles"`
	Kind              string   `json:"kind"`
	ImageURL          string   `json:"image_url"`
	SourceURL         string   `json:"source_url"`
	PreviewToken      string   `json:"preview_token"`
	AttributionURL    string   `json:"attribution_url"`
	Score             float64  `json:"score"`
	Reason            string   `json:"reason"`
}

// SearchInput is the bounded payload accepted by the discovery endpoints.
type SearchInput struct {
	Title        string
	Kind         Kind
	Alternatives []string
	References   []string
}

// Registry holds the verified providers in a stable order.
type Registry struct {
	providers []Provider
}

func NewRegistry(providers ...Provider) *Registry {
	return &Registry{providers: providers}
}

func (r *Registry) Providers() []ProviderInfo {
	out := make([]ProviderInfo, 0, len(r.providers))
	for _, p := range r.providers {
		kinds := make([]string, 0, len(p.Kinds()))
		for _, k := range p.Kinds() {
			kinds = append(kinds, string(k))
		}
		out = append(out, ProviderInfo{
			ID: p.ID(), Label: p.Label(), Kinds: kinds,
			AttributionURL: p.AttributionURL(), Required: true,
		})
	}
	return out
}

func (r *Registry) forKind(kind Kind) []Provider {
	out := []Provider{}
	for _, p := range r.providers {
		for _, supported := range p.Kinds() {
			if supported == kind {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// punctuationFold removes punctuation so "Cowboy Bebop!" and "Cowboy-Bebop"
// compare equal, while keeping kana, kanji and diacritics untouched.
func punctuationFold(s string) string {
	var b strings.Builder
	for _, r := range domain.SearchKey(s) {
		if r == ' ' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r >= 0x80 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

// scoreTitle returns a bounded 0..1 similarity against any known title. An
// exact normalized match wins; prefix/containment and edit distance are
// secondary. No fuzzy match alone ever creates a binding.
func scoreTitle(titles []string, candidate string) (float64, string) {
	want := punctuationFold(candidate)
	if want == "" {
		return 0, ""
	}
	for _, title := range titles {
		got := punctuationFold(title)
		if got == want {
			return 1, "exact normalized title"
		}
	}
	best, reason := 0.0, "similar title"
	for _, title := range titles {
		got := punctuationFold(title)
		if got == "" {
			continue
		}
		var score float64
		switch {
		case strings.HasPrefix(got, want) || strings.HasPrefix(want, got):
			shorter, longer := len([]rune(got)), len([]rune(want))
			if shorter > longer {
				shorter, longer = longer, shorter
			}
			score = 0.7 + 0.25*float64(shorter)/float64(longer)
		case strings.Contains(got, want) || strings.Contains(want, got):
			score = 0.78
		default:
			max := len([]rune(got))
			if len([]rune(want)) > max {
				max = len([]rune(want))
			}
			if max > 96 {
				continue
			}
			dist := levenshtein(got, want)
			score = 1 - float64(dist)/float64(max)
		}
		if score > best {
			best, reason = score, "similar title"
		}
	}
	if best < 0 {
		best = 0
	}
	return best, reason
}

// Match filters candidates by kind, scores them against every known title and
// returns at most eight distinct, ordered results. A single high-confidence
// result is a suggestion; the caller never binds it automatically.
func Match(in SearchInput, candidates []Candidate) []ScoredCandidate {
	titles := make([]string, 0, 1+len(in.Alternatives))
	titles = append(titles, in.Title)
	titles = append(titles, in.Alternatives...)
	seen := map[string]bool{}
	scored := make([]ScoredCandidate, 0, len(candidates))
	for _, c := range candidates {
		key := c.Provider + "/" + c.ExternalID
		if seen[key] {
			continue
		}
		seen[key] = true
		if c.Kind != in.Kind {
			continue
		}
		all := append(append([]string{}, titles...), c.AlternativeTitles...)
		score, reason := scoreTitle(all, c.Title)
		if score < 0.35 {
			continue
		}
		alts := c.AlternativeTitles
		if alts == nil {
			alts = []string{}
		}
		scored = append(scored, ScoredCandidate{
			Provider: c.Provider, ExternalID: c.ExternalID, Title: c.Title,
			AlternativeTitles: alts, Kind: string(c.Kind), SourceURL: c.SourceURL,
			AttributionURL: c.AttributionURL, ImageURL: c.ImageURL,
			Score: score, Reason: reason,
		})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].Provider < scored[j].Provider
		}
		return scored[i].Score > scored[j].Score
	})
	if len(scored) > 8 {
		scored = scored[:8]
	}
	return scored
}

// IsConfident reports whether exactly one candidate is strong enough to be
// offered as a dismissible suggestion.
func IsConfident(candidates []ScoredCandidate) bool {
	return len(candidates) == 1 && candidates[0].Score >= 0.9
}

// ErrUpstream means every provider failed or was unavailable; the caller can
// still complete the user's primary action without a cover.
var ErrUpstream = fmt.Errorf("artwork: upstream providers unavailable")
