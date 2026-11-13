package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// activityTypes returns the set of activity types in one page, in page
// order.
func activityTypes(t *testing.T, items []any) []string {
	t.Helper()
	types := make([]string, 0, len(items))
	for _, it := range items {
		types = append(types, it.(map[string]any)["type"].(string))
	}
	return types
}

// activityData decodes a successful /activity response and returns the
// payload map.
func activityData(t *testing.T, srv *Server, cookie *http.Cookie, query string) map[string]any {
	t.Helper()
	resp, raw := catalogGet(t, srv, cookie, "/api/v1/activity"+query)
	return expectSuccess(t, resp, raw)
}

func TestHTTPActivity(t *testing.T) {
	var editionID string
	srv := startCatalogServer(t, func(s *app.Service) {
		var ed *string
		_, ed = seedWork(t, s, "seed-activity", []store.TitleInput{titleInput("Activity Story", domain.TitleOriginal)}, appKind(domain.KindManga))
		if ed == nil {
			t.Fatal("seed created no edition")
		}
		editionID = *ed
		// Each observation bumps the edition revision: 0 -> 1 -> 2 -> 3.
		rev0 := int64(0)
		if _, err := s.RecordProgress(context.Background(), app.RecordProgressRequest{
			MutationBase: app.MutationBase{RequestID: "act-progress", Actor: testActor, ExpectedRevision: &rev0},
			EditionID:    editionID, Unit: domain.UnitChapter, Position: "5", Total: strPtrVal("10"),
		}); err != nil {
			t.Fatalf("seed progress: %v", err)
		}
		rev1 := int64(1)
		if _, err := s.SetConsumption(context.Background(), app.SetConsumptionRequest{
			MutationBase: app.MutationBase{RequestID: "act-consumption", Actor: testActor, ExpectedRevision: &rev1},
			EditionID:    editionID, Value: "in_progress",
		}); err != nil {
			t.Fatalf("seed consumption: %v", err)
		}
		rev2 := int64(2)
		if _, err := s.SetIntention(context.Background(), app.SetIntentionRequest{
			MutationBase: app.MutationBase{RequestID: "act-intention", Actor: testActor, ExpectedRevision: &rev2},
			EditionID:    editionID, Value: "saved",
		}); err != nil {
			t.Fatalf("seed intention: %v", err)
		}
		rev3 := int64(3)
		seven := 7
		if _, err := s.SetRating(context.Background(), app.SetRatingRequest{
			MutationBase: app.MutationBase{RequestID: "act-rating", Actor: testActor, ExpectedRevision: &rev3},
			EditionID:    editionID, Value: &seven,
		}); err != nil {
			t.Fatalf("seed rating: %v", err)
		}
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, _ := login(t, srv, secret)

	// The feed carries every seeded observation: the default
	// not_started consumption row plus the four seeded observations.
	data := activityData(t, srv, cookie, "")
	if got := data["total"].(float64); got != 5 {
		t.Fatalf("total = %v, want 5", data["total"])
	}
	types := activityTypes(t, data["items"].([]any))
	for _, want := range []string{"consumption", "progress", "intention", "rating"} {
		found := false
		for _, got := range types {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("activity types = %v, want to include %s", types, want)
		}
	}

	// The types filter narrows the feed to one type.
	data = activityData(t, srv, cookie, "?types=progress")
	if got := data["total"].(float64); got != 1 {
		t.Fatalf("filtered total = %v, want 1", data["total"])
	}
	if types := activityTypes(t, data["items"].([]any)); len(types) != 1 || types[0] != "progress" {
		t.Fatalf("filtered types = %v, want only progress", types)
	}

	// An unknown type is an invalid argument.
	resp, raw := catalogGet(t, srv, cookie, "/api/v1/activity?types=bogus")
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// A non-numeric limit is an invalid argument.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/activity?limit=abc")
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// Pagination: two rows per page with a seek cursor, no row
	// skipped at the boundary.
	page1 := activityData(t, srv, cookie, "?limit=2")
	items1 := page1["items"].([]any)
	if len(items1) != 2 {
		t.Fatalf("page 1 items = %d, want 2", len(items1))
	}
	next, _ := page1["next_cursor"].(string)
	if next == "" {
		t.Fatalf("page 1 next_cursor = %v, want a cursor", page1["next_cursor"])
	}
	seen := map[string]bool{}
	for _, it := range items1 {
		seen[it.(map[string]any)["id"].(string)] = true
	}
	page2 := activityData(t, srv, cookie, "?limit=2&cursor="+next)
	items2 := page2["items"].([]any)
	if len(items2) != 2 {
		t.Fatalf("page 2 items = %d, want 2", len(items2))
	}
	for _, it := range items2 {
		if seen[it.(map[string]any)["id"].(string)] {
			t.Fatalf("page 2 repeats a page 1 row: %v", it)
		}
	}

	// A malformed cursor is an invalid argument.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/activity?cursor=not-a-cursor")
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// The read boundary requires a live session.
	resp, raw = doJSON(t, srv, http.MethodGet, "/api/v1/activity", nil, nil)
	expectFailure(t, resp, raw, http.StatusUnauthorized, string(app.ErrUnauthorized))
}
