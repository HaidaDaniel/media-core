package artwork

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const searchLimit = 5

var (
	kitsuAPIHosts = map[string]bool{"kitsu.io": true, "kitsu.app": true, "api.kitsu.app": true}
	kitsuImgHosts = map[string]bool{"media.kitsu.app": true, "media.kitsu.io": true}
	mdexAPIHosts  = map[string]bool{"api.mangadex.org": true}
	mdexImgHosts  = map[string]bool{"uploads.mangadex.org": true}
	olAPIHosts    = map[string]bool{"openlibrary.org": true}
	olImgHosts    = map[string]bool{"covers.openlibrary.org": true}
	shikiHosts    = map[string]bool{"shikimori.one": true, "shikimori.io": true}
)

// Kitsu provides anime and manga through its public JSON:API. No published
// fixed rate limit exists; the shared one-request-per-second client is
// deliberately more conservative than any observed throttling.
type kitsuProvider struct {
	client  *safeClient
	kind    Kind
	baseURL string
}

func (p *kitsuProvider) ID() string             { return "kitsu" }
func (p *kitsuProvider) Label() string          { return "Kitsu" }
func (p *kitsuProvider) AttributionURL() string { return "https://kitsu.io/" }
func (p *kitsuProvider) Kinds() []Kind          { return []Kind{p.kind} }

func (p *kitsuProvider) Search(ctx context.Context, title string, kind Kind, limit int) ([]Candidate, error) {
	if kind != p.kind {
		return nil, nil
	}
	resource := string(kind)
	base := p.baseURL
	if base == "" {
		base = "https://kitsu.io"
	}
	u := base + "/api/edge/" + resource
	q := url.Values{}
	q.Set("filter[text]", title)
	q.Set("page[limit]", strconv.Itoa(boundLimit(limit)))
	var doc struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Slug              string            `json:"slug"`
				CanonicalTitle    string            `json:"canonicalTitle"`
				Titles            map[string]string `json:"titles"`
				AbbreviatedTitles []string          `json:"abbreviatedTitles"`
				PosterImage       map[string]string `json:"posterImage"`
			} `json:"attributes"`
		} `json:"data"`
	}
	res, err := p.client.fetch(ctx, p.ID(), u+"?"+q.Encode(), kitsuAPIHosts, maxJSONBytes, "application/vnd.api+json")
	if err != nil {
		return nil, err
	}
	if err := decodeJSON(res.Body, &doc); err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(doc.Data))
	for _, item := range doc.Data {
		a := item.Attributes
		if item.ID == "" || a.CanonicalTitle == "" {
			continue
		}
		alts := append([]string{}, a.AbbreviatedTitles...)
		for _, v := range a.Titles {
			if v != "" && v != a.CanonicalTitle {
				alts = append(alts, v)
			}
		}
		image := a.PosterImage["medium"]
		if image == "" {
			image = a.PosterImage["small"]
		}
		out = append(out, Candidate{
			Provider: p.ID(), ExternalID: item.ID, Title: a.CanonicalTitle,
			AlternativeTitles: dedupeStrings(alts), Kind: kind,
			SourceURL:      "https://kitsu.io/" + resource + "/" + a.Slug,
			AttributionURL: p.AttributionURL(), ImageURL: image,
		})
	}
	return out, nil
}

// MangaDex supplies manga covers through /manga?includes[]=cover_art. Its
// policy requires attribution, no ads and proxying image requests, all of
// which this implementation does.
type mangadexProvider struct {
	client  *safeClient
	baseURL string
}

func (p *mangadexProvider) ID() string             { return "mangadex" }
func (p *mangadexProvider) Label() string          { return "MangaDex" }
func (p *mangadexProvider) AttributionURL() string { return "https://mangadex.org/" }
func (p *mangadexProvider) Kinds() []Kind          { return []Kind{KindManga} }

func (p *mangadexProvider) Search(ctx context.Context, title string, kind Kind, limit int) ([]Candidate, error) {
	if kind != KindManga {
		return nil, nil
	}
	q := url.Values{}
	q.Set("title", title)
	q.Set("limit", strconv.Itoa(boundLimit(limit)))
	q.Add("includes[]", "cover_art")
	var doc struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Title     map[string]string   `json:"title"`
				AltTitles []map[string]string `json:"altTitles"`
			} `json:"attributes"`
			Relationships []struct {
				ID         string `json:"id"`
				Type       string `json:"type"`
				Attributes struct {
					FileName string `json:"fileName"`
				} `json:"attributes"`
			} `json:"relationships"`
		} `json:"data"`
	}
	base := p.baseURL
	if base == "" {
		base = "https://api.mangadex.org"
	}
	res, err := p.client.fetch(ctx, p.ID(), base+"/manga?"+q.Encode(), mdexAPIHosts, maxJSONBytes, "application/json")
	if err != nil {
		return nil, err
	}
	if err := decodeJSON(res.Body, &doc); err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(doc.Data))
	for _, item := range doc.Data {
		name := preferredMangaTitle(item.Attributes.Title)
		if item.ID == "" || name == "" {
			continue
		}
		alts := []string{}
		for _, m := range item.Attributes.AltTitles {
			for _, v := range m {
				if v != "" {
					alts = append(alts, v)
				}
			}
		}
		image := ""
		for _, rel := range item.Relationships {
			if rel.Type == "cover_art" && rel.Attributes.FileName != "" {
				image = "https://uploads.mangadex.org/covers/" + item.ID + "/" + rel.Attributes.FileName + ".512.jpg"
				break
			}
		}
		out = append(out, Candidate{
			Provider: p.ID(), ExternalID: item.ID, Title: name,
			AlternativeTitles: dedupeStrings(alts), Kind: KindManga,
			SourceURL:      "https://mangadex.org/title/" + item.ID,
			AttributionURL: p.AttributionURL(), ImageURL: image,
		})
	}
	return out, nil
}

func preferredMangaTitle(titles map[string]string) string {
	for _, key := range []string{"en", "ja-ro", "ja", "ko-ro", "zh-ro"} {
		if titles[key] != "" {
			return titles[key]
		}
	}
	for _, v := range titles {
		if v != "" {
			return v
		}
	}
	return ""
}

// Open Library search returns book work keys and cover IDs in one request.
// Its guidance asks for an identified User-Agent, caching and no crawling;
// all three are enforced here.
type openLibraryProvider struct {
	client  *safeClient
	baseURL string
}

func (p *openLibraryProvider) ID() string             { return "openlibrary" }
func (p *openLibraryProvider) Label() string          { return "Open Library" }
func (p *openLibraryProvider) AttributionURL() string { return "https://openlibrary.org/" }
func (p *openLibraryProvider) Kinds() []Kind          { return []Kind{KindBook} }

func (p *openLibraryProvider) Search(ctx context.Context, title string, kind Kind, limit int) ([]Candidate, error) {
	if kind != KindBook {
		return nil, nil
	}
	q := url.Values{}
	q.Set("q", title)
	q.Set("limit", strconv.Itoa(boundLimit(limit)))
	q.Set("fields", "key,title,cover_i,author_name,first_publish_year")
	var doc struct {
		Docs []struct {
			Key     string `json:"key"`
			Title   string `json:"title"`
			CoverID int64  `json:"cover_i"`
		} `json:"docs"`
	}
	base := p.baseURL
	if base == "" {
		base = "https://openlibrary.org"
	}
	res, err := p.client.fetch(ctx, p.ID(), base+"/search.json?"+q.Encode(), olAPIHosts, maxJSONBytes, "application/json")
	if err != nil {
		return nil, err
	}
	if err := decodeJSON(res.Body, &doc); err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(doc.Docs))
	for _, item := range doc.Docs {
		if item.Key == "" || item.Title == "" {
			continue
		}
		image := ""
		if item.CoverID > 0 {
			image = "https://covers.openlibrary.org/b/id/" + strconv.FormatInt(item.CoverID, 10) + "-M.jpg?default=false"
		}
		out = append(out, Candidate{
			Provider: p.ID(), ExternalID: strings.TrimPrefix(item.Key, "/works/"),
			Title: item.Title, Kind: KindBook,
			SourceURL:      "https://openlibrary.org" + item.Key,
			AttributionURL: p.AttributionURL(), ImageURL: image,
		})
	}
	return out, nil
}

// Shikimori's GraphQL API is preferred over its outdated REST versions: new
// posters are only exposed there. Its 5 rps / 90 rpm restriction and
// descriptive User-Agent requirement are handled by the shared client.
type shikimoriProvider struct {
	client  *safeClient
	kind    Kind
	baseURL string
}

func (p *shikimoriProvider) ID() string             { return "shikimori" }
func (p *shikimoriProvider) Label() string          { return "Shikimori" }
func (p *shikimoriProvider) AttributionURL() string { return "https://shikimori.one/" }
func (p *shikimoriProvider) Kinds() []Kind          { return []Kind{p.kind} }

func (p *shikimoriProvider) Search(ctx context.Context, title string, kind Kind, limit int) ([]Candidate, error) {
	if kind != p.kind {
		return nil, nil
	}
	field := "animes"
	if kind == KindManga {
		field = "mangas"
	}
	query := fmt.Sprintf(`query($q: String!, $n: Int!) { %s(search: $q, limit: $n) {
		id name russian kind url poster { previewUrl originalUrl }
	} }`, field)
	payload, err := json.Marshal(map[string]any{
		"query":     query,
		"variables": map[string]any{"q": title, "n": boundLimit(limit)},
	})
	if err != nil {
		return nil, err
	}
	base := p.baseURL
	if base == "" {
		base = "https://shikimori.one"
	}
	res, err := p.client.postJSON(ctx, p.ID(), base+"/api/graphql", payload, shikiHosts)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Data map[string][]struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Russian string `json:"russian"`
			Kind    string `json:"kind"`
			URL     string `json:"url"`
			Poster  struct {
				PreviewURL  string `json:"previewUrl"`
				OriginalURL string `json:"originalUrl"`
			} `json:"poster"`
		} `json:"data"`
	}
	if err := decodeJSON(res.Body, &doc); err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, item := range doc.Data[field] {
		if item.ID == "" || item.Name == "" {
			continue
		}
		alts := []string{}
		if item.Russian != "" {
			alts = append(alts, item.Russian)
		}
		image := item.Poster.PreviewURL
		if image == "" {
			image = item.Poster.OriginalURL
		}
		out = append(out, Candidate{
			Provider: p.ID(), ExternalID: item.ID, Title: item.Name,
			AlternativeTitles: alts, Kind: kind,
			SourceURL: item.URL, AttributionURL: p.AttributionURL(), ImageURL: image,
		})
	}
	return out, nil
}

func boundLimit(limit int) int {
	if limit < 1 {
		return searchLimit
	}
	if limit > searchLimit {
		return searchLimit
	}
	return limit
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// NewDefault wires the four verified providers and the one client they and
// the image fetcher share. TMDB stays an optional extension because it
// requires an API key and attribution banner; it is documented but not
// enabled by default.
func NewDefault() (*Registry, *safeClient) {
	client := newSafeClient("media-core/1.0 (personal library; explicit cover discovery)")
	return NewRegistry(
		&kitsuProvider{client: client, kind: KindAnime},
		&shikimoriProvider{client: client, kind: KindAnime},
		&kitsuProvider{client: client, kind: KindManga},
		&shikimoriProvider{client: client, kind: KindManga},
		&mangadexProvider{client: client},
		&openLibraryProvider{client: client},
	), client
}

// ImageURLs returns the host allowlist needed to fetch a candidate's image
// for the given provider.
func imageHostsFor(provider string) map[string]bool {
	switch provider {
	case "kitsu":
		return kitsuImgHosts
	case "mangadex":
		return mdexImgHosts
	case "openlibrary":
		return olImgHosts
	case "shikimori":
		return shikiHosts
	}
	return nil
}
