package artwork

import (
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// tinyJPEG produces a real encoded JPEG so cache validation exercises the
// image header path without a fixture file.
func tinyJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: 32, B: 36, A: 255})
		}
	}
	var b strings.Builder
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return []byte(b.String())
}

func testClientFor(t *testing.T, handler http.Handler) (*safeClient, map[string]bool, string) {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	host := strings.Split(strings.TrimPrefix(srv.URL, "https://"), ":")[0]
	kitsuAPIHosts[host] = true
	kitsuImgHosts[host] = true
	mdexAPIHosts[host] = true
	mdexImgHosts[host] = true
	olAPIHosts[host] = true
	olImgHosts[host] = true
	shikiHosts[host] = true
	c := &safeClient{
		limits: map[string]*limiter{}, userAgent: "media-core-test", allowPrivate: true,
	}
	cl := srv.Client()
	cl.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("%w: too many redirects", ErrNetworkURL)
		}
		return c.validateURL(req.URL, via[0].URL)
	}
	c.client = cl
	return c, map[string]bool{host: true}, srv.URL
}

func TestMatchKindAndConfidence(t *testing.T) {
	in := SearchInput{Title: "Vinland Saga", Kind: KindManga, Alternatives: []string{"ヴィンランド・サガ"}}
	candidates := []Candidate{
		{Provider: "kitsu", ExternalID: "1", Title: "Vinland Saga", Kind: KindAnime},
		{Provider: "mangadex", ExternalID: "2", Title: "Vinland Saga", Kind: KindManga, SourceURL: "https://mangadex.org/title/2"},
		{Provider: "kitsu", ExternalID: "3", Title: "Vinland Saga Extended", Kind: KindManga, SourceURL: "https://kitsu.io/manga/3"},
	}
	got := Match(in, candidates)
	if len(got) != 2 {
		t.Fatalf("Match returned %d candidates, want 2 (anime must be filtered)", len(got))
	}
	if got[0].Provider != "mangadex" || got[0].Score < 0.99 {
		t.Fatalf("best candidate = %+v, want exact manga match", got[0])
	}
	if IsConfident(got) {
		t.Fatal("two close candidates must not be a confident suggestion")
	}
	single := Match(in, candidates[1:2])
	if !IsConfident(single) {
		t.Fatal("a single exact candidate must be a dismissible suggestion")
	}
}

func TestMatchDoesNotMixFormats(t *testing.T) {
	got := Match(SearchInput{Title: "Dune", Kind: KindBook}, []Candidate{
		{Provider: "kitsu", ExternalID: "1", Title: "Dune", Kind: KindAnime},
		{Provider: "openlibrary", ExternalID: "2", Title: "Dune", Kind: KindBook},
	})
	if len(got) != 1 || got[0].Provider != "openlibrary" {
		t.Fatalf("format filter failed: %+v", got)
	}
}

func TestClientRejectsUnsafeURLs(t *testing.T) {
	c := newSafeClient("media-core-test")
	hosts := map[string]bool{"example.com": true}
	cases := []struct {
		url  string
		want error
	}{
		{"http://example.com/x", ErrNetworkURL},
		{"https://user:pass@example.com/x", ErrNetworkURL},
		{"https://127.0.0.1/x", ErrNetworkURL},
		{"https://10.0.0.5/x", ErrNetworkURL},
		{"https://[::1]/x", ErrNetworkURL},
	}
	for _, tc := range cases {
		_, err := c.fetch(context.Background(), "test", tc.url, hosts, maxJSONBytes, "application/json")
		if !errorIs(err, tc.want) {
			t.Errorf("fetch(%q) err = %v, want %v", tc.url, err, tc.want)
		}
	}
	_, err := c.fetch(context.Background(), "test", "https://not-allowed.example/x", hosts, maxJSONBytes, "application/json")
	if !errorIs(err, ErrNetworkURL) {
		t.Errorf("non-allowlisted host err = %v", err)
	}
}

func errorIs(err, target error) bool {
	if err == nil {
		return false
	}
	for err != nil {
		if err == target {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func TestClientBoundsOversizeAndRedirects(t *testing.T) {
	big := strings.Repeat("x", 2048)
	c, hosts, base := testClientFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/big":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(big))
		case "/redir":
			http.Redirect(w, r, "/redir", http.StatusFound)
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	if _, err := c.fetch(context.Background(), "test", base+"/big", hosts, 100, "application/json"); !errorIs(err, ErrUpstreamSize) {
		t.Fatalf("oversize err = %v, want ErrUpstreamSize", err)
	}
	if _, err := c.fetch(context.Background(), "test", base+"/redir", hosts, 100, "application/json"); !errorIs(err, ErrNetworkURL) {
		t.Fatalf("redirect loop err = %v, want ErrNetworkURL", err)
	}
}

func TestAdaptersWithMockedFixtures(t *testing.T) {
	ctx := context.Background()
	t.Run("kitsu", func(t *testing.T) {
		c, hosts, base := testClientFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.api+json")
			_, _ = w.Write([]byte(`{"data":[{"id":"7","attributes":{"canonicalTitle":"Vinland Saga","slug":"vinland-saga","titles":{"ja_jp":"ヴィンランド・サガ"},"abbreviatedTitles":["Vinland"],"posterImage":{"medium":"https://media.kitsu.app/manga/poster_images/7/medium.jpg"}}}]}`))
		}))
		_ = hosts
		p := &kitsuProvider{client: c, kind: KindManga, baseURL: base}
		got, err := p.Search(ctx, "Vinland Saga", KindManga, 5)
		if err != nil || len(got) != 1 || got[0].ExternalID != "7" {
			t.Fatalf("kitsu = %+v, %v", got, err)
		}
	})
	t.Run("mangadex", func(t *testing.T) {
		c, _, base := testClientFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"m1","attributes":{"title":{"en":"Vinland Saga"},"altTitles":[{"ja":"ヴィンランド・サガ"}]},"relationships":[{"id":"c1","type":"cover_art","attributes":{"fileName":"abc.jpg"}}]}]}`))
		}))
		p := &mangadexProvider{client: c, baseURL: base}
		got, err := p.Search(ctx, "Vinland Saga", KindManga, 5)
		if err != nil || len(got) != 1 || !strings.Contains(got[0].ImageURL, "/covers/m1/abc.jpg.512.jpg") {
			t.Fatalf("mangadex = %+v, %v", got, err)
		}
	})
	t.Run("openlibrary", func(t *testing.T) {
		c, _, base := testClientFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"docs":[{"key":"/works/OL1W","title":"Dune","cover_i":42}]}`))
		}))
		p := &openLibraryProvider{client: c, baseURL: base}
		got, err := p.Search(ctx, "Dune", KindBook, 5)
		if err != nil || len(got) != 1 || !strings.Contains(got[0].ImageURL, "/b/id/42-M.jpg") {
			t.Fatalf("openlibrary = %+v, %v", got, err)
		}
	})
	t.Run("shikimori", func(t *testing.T) {
		c, _, base := testClientFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"animes":[{"id":"1","name":"Cowboy Bebop","russian":"Ковбой Бибоп","kind":"tv","url":"https://shikimori.one/animes/1","poster":{"previewUrl":"https://shikimori.one/system/animes/preview/1.jpg"}}]}}`))
		}))
		p := &shikimoriProvider{client: c, kind: KindAnime, baseURL: base}
		got, err := p.Search(ctx, "Cowboy Bebop", KindAnime, 5)
		if err != nil || len(got) != 1 || got[0].ExternalID != "1" {
			t.Fatalf("shikimori = %+v, %v", got, err)
		}
	})
}

func TestAdapterRejectsMalformedResponse(t *testing.T) {
	c, _, base := testClientFor(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":`))
	}))
	p := &openLibraryProvider{client: c, baseURL: base}
	if _, err := p.Search(context.Background(), "Dune", KindBook, 5); err == nil {
		t.Fatal("malformed JSON must fail")
	}
}

func TestCachePutOpenClearAndRefValidation(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewCache(dir, 1<<20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	data := tinyJPEG(t)
	ref := Ref("test", "https://example.com/a.jpg")
	if _, err := cache.Put(ref, data, "image/jpeg", nil); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, mime, err := cache.Open(ref)
	if err != nil || mime != "image/jpeg" || len(got) != len(data) {
		t.Fatalf("Open = %d bytes, %q, %v", len(got), mime, err)
	}
	if _, _, err := cache.Open("../outside.img"); !errorIs(err, ErrCacheRef) {
		t.Fatalf("path traversal err = %v", err)
	}
	removed, bytes, err := cache.Clear()
	if err != nil || removed != 1 || bytes != int64(len(data)) {
		t.Fatalf("Clear = %d, %d, %v", removed, bytes, err)
	}
	if _, _, err := cache.Open(ref); !os.IsNotExist(err) {
		t.Fatalf("Open after clear err = %v", err)
	}
}

func TestCacheRejectsOversizeAndDecompressionBombs(t *testing.T) {
	cache, err := NewCache(t.TempDir(), 1<<20, 16)
	if err != nil {
		t.Fatal(err)
	}
	ref := Ref("test", "https://example.com/big.jpg")
	if _, err := cache.Put(ref, []byte(strings.Repeat("x", 32)), "image/jpeg", nil); !errorIs(err, ErrCacheSize) {
		t.Fatalf("oversize err = %v", err)
	}
	smallLimit, err := NewCache(t.TempDir(), 1<<20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	// A declared 3000x3000 JPEG header is rejected by the dimension bound
	// even though the encoded body is small.
	var b strings.Builder
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3001, 3001)), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := smallLimit.Put(ref, []byte(b.String()), "image/jpeg", nil); !errorIs(err, ErrCacheSize) {
		t.Fatalf("bomb err = %v", err)
	}
}

func TestServiceSelectBindsPreviewToWork(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.OpenForMaintenance(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RunMigrations(db.DB()); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	work, err := db.CreateWork(ctx, time.Now().UTC(), []store.TitleInput{{
		Text: "Vinland Saga", Kind: domain.TitleOriginal, Preferred: true,
		Provenance: "test",
	}})
	if err != nil {
		t.Fatal(err)
	}
	cache, err := NewCache(dir, 1<<20, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	client := newSafeClient("test")
	svc := NewService(db, NewRegistry(), client, cache)
	candidate := ScoredCandidate{
		Provider: "kitsu", ExternalID: "7", Title: "Vinland Saga", Kind: string(KindManga),
		SourceURL: "https://kitsu.io/manga/7", AttributionURL: "https://kitsu.io/",
	}
	token := svc.previews.Put(candidate, tinyJPEG(t), "image/jpeg", time.Now())
	record, err := svc.Select(ctx, work.ID, "", token, false)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if record.Provider != "kitsu" || !record.SelectedByUser {
		t.Fatalf("record = %+v", record)
	}
	if _, err := svc.Select(ctx, work.ID, "", token, false); !errorIs(err, store.ErrArtworkExists) {
		t.Fatalf("second select err = %v, want ErrArtworkExists", err)
	}
	if _, _, err := svc.ArtworkImage(ctx, record.ID); err != nil {
		t.Fatalf("ArtworkImage: %v", err)
	}
}

func TestPreviewTokenIsOpaqueAndBounded(t *testing.T) {
	store := NewPreviewStore()
	now := time.Now()
	token := store.Put(ScoredCandidate{Provider: "kitsu"}, []byte("x"), "image/jpeg", now)
	if len(token) != 32 {
		t.Fatalf("token = %q", token)
	}
	if _, ok := store.Get("../../etc/passwd", now); ok {
		t.Fatal("path-like token must not resolve")
	}
	if _, ok := store.Get(token, now.Add(11*time.Minute)); ok {
		t.Fatal("expired token must not resolve")
	}
	for i := 0; i < 30; i++ {
		store.Put(ScoredCandidate{Provider: "kitsu", ExternalID: fmt.Sprint(i)}, []byte("x"), "image/jpeg", now)
	}
	if len(store.order) > store.max {
		t.Fatalf("preview store grew to %d entries", len(store.order))
	}
}

var _ = base64.StdEncoding
var _ = filepath.Join

func TestParseKindKeepsFormatsSeparate(t *testing.T) {
	cases := map[string]Kind{
		"anime": KindAnime, "manga": KindManga, "manhwa": KindManga,
		"manhua": KindManga, "book": KindBook, "web_novel": KindBook,
		"light_novel": KindBook, "movie": KindMovie, "tv": KindTV,
	}
	for raw, want := range cases {
		got, ok := ParseKind(raw)
		if !ok || got != want {
			t.Errorf("ParseKind(%q) = %q, %v; want %q", raw, got, ok, want)
		}
	}
	for _, raw := range []string{"game", "visual_novel", "other", "unknown", ""} {
		if _, ok := ParseKind(raw); ok {
			t.Errorf("ParseKind(%q) unexpectedly succeeded", raw)
		}
	}
}
