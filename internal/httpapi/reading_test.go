package httpapi

import (
	"net/http"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// progressData decodes a successful progress/consumption response and
// returns the edition view data.
func readingData(t *testing.T, resp *http.Response, raw []byte) map[string]any {
	t.Helper()
	return expectSuccess(t, resp, raw)
}

func TestHTTPReadingProgress(t *testing.T) {
	var editionID string
	srv := startCatalogServer(t, func(s *app.Service) {
		_, ed := seedWork(t, s, "seed-progress", []store.TitleInput{titleInput("Progress Story", domain.TitleOriginal)}, appKind(domain.KindManga))
		if ed == nil {
			t.Fatal("seed created no edition")
		}
		editionID = *ed
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	// Chapter 42 of 64 at the initial revision.
	resp, raw := catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/progress", map[string]any{
		"request_id":        "req-progress-1",
		"expected_revision": 0,
		"unit":              "chapter",
		"position":          "42",
		"total":             "64",
	})
	data := readingData(t, resp, raw)
	if got := data["revision"].(float64); got != 1 {
		t.Fatalf("revision after 42 = %v, want 1", data["revision"])
	}
	chapter := data["progress"].(map[string]any)["chapter"].(map[string]any)
	if chapter["position"] != "42" || chapter["total"] != "64" {
		t.Fatalf("chapter after 42 = %v, want 42 of 64", chapter)
	}
	if got := data["consumption"].(map[string]any)["value"]; got != "not_started" {
		t.Fatalf("consumption after progress = %v, want not_started: progress never sets status", data["consumption"])
	}

	// Chapter 48 advances the position.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/progress", map[string]any{
		"request_id":        "req-progress-2",
		"expected_revision": 1,
		"unit":              "chapter",
		"position":          "48",
		"total":             "64",
	})
	data = readingData(t, resp, raw)
	chapter = data["progress"].(map[string]any)["chapter"].(map[string]any)
	if chapter["position"] != "48" {
		t.Fatalf("chapter after 48 = %v, want 48", chapter)
	}
	position48 := chapter["id"].(string)

	// A correction supersedes the 48 row; it stays in history.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/progress", map[string]any{
		"request_id":        "req-progress-3",
		"expected_revision": 2,
		"unit":              "chapter",
		"position":          "47",
		"total":             "64",
		"supersedes_id":     position48,
		"reason":            "miscounted",
	})
	data = readingData(t, resp, raw)
	chapter = data["progress"].(map[string]any)["chapter"].(map[string]any)
	if chapter["position"] != "47" {
		t.Fatalf("chapter after correction = %v, want 47", chapter)
	}

	// The same current value is a no-op without a revision bump.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/progress", map[string]any{
		"request_id":        "req-progress-4",
		"expected_revision": 3,
		"unit":              "chapter",
		"position":          "47",
		"total":             "64",
	})
	data = readingData(t, resp, raw)
	if got := data["revision"].(float64); got != 3 {
		t.Fatalf("revision after the same-value no-op = %v, want 3", data["revision"])
	}

	// A stale expected revision conflicts.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/progress", map[string]any{
		"request_id":        "req-progress-stale",
		"expected_revision": 2,
		"unit":              "chapter",
		"position":          "49",
		"total":             "64",
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))

	// An unknown unit and a non-decimal position are invalid arguments.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/progress", map[string]any{
		"request_id":        "req-progress-bad-unit",
		"expected_revision": 3,
		"unit":              "season",
		"position":          "49",
	})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/progress", map[string]any{
		"request_id":        "req-progress-bad-position",
		"expected_revision": 3,
		"unit":              "chapter",
		"position":          "abc",
	})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// A repeated request replays: same key and payload succeed again for
	// the same edition, without writing a second observation.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/progress", map[string]any{
		"request_id":        "req-progress-1",
		"expected_revision": 0,
		"unit":              "chapter",
		"position":          "42",
		"total":             "64",
	})
	data = readingData(t, resp, raw)
	if data["id"] != editionID {
		t.Fatalf("replayed result for edition %v, want %s", data["id"], editionID)
	}
	if got := data["progress"].(map[string]any)["chapter"].(map[string]any)["position"]; got != "47" {
		t.Fatalf("chapter after the replay = %v, want the live 47: a replay writes nothing", got)
	}
}

func TestHTTPReadingConsumption(t *testing.T) {
	var editionID string
	srv := startCatalogServer(t, func(s *app.Service) {
		_, ed := seedWork(t, s, "seed-consumption", []store.TitleInput{titleInput("Status Story", domain.TitleOriginal)}, appKind(domain.KindBook))
		if ed == nil {
			t.Fatal("seed created no edition")
		}
		editionID = *ed
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	// Start reading.
	resp, raw := catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/consumption", map[string]any{
		"request_id":        "req-consumption-1",
		"expected_revision": 0,
		"value":             "in_progress",
	})
	data := readingData(t, resp, raw)
	if got := data["revision"].(float64); got != 1 {
		t.Fatalf("revision after in_progress = %v, want 1", data["revision"])
	}
	consumption := data["consumption"].(map[string]any)
	if consumption["value"] != "in_progress" {
		t.Fatalf("consumption = %v, want in_progress", consumption)
	}
	if data["progress"] == nil {
		t.Fatal("progress view missing from the edition view")
	}

	// The same value is a no-op without a revision bump.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/consumption", map[string]any{
		"request_id":        "req-consumption-2",
		"expected_revision": 1,
		"value":             "in_progress",
	})
	data = readingData(t, resp, raw)
	if got := data["revision"].(float64); got != 1 {
		t.Fatalf("revision after the same-value no-op = %v, want 1", data["revision"])
	}

	// A correction supersedes the previous row; the record id of a
	// consumption observation comes from the history projection.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+editionID+"/history")
	hist := readingData(t, resp, raw)
	var inProgressID string
	for _, it := range hist["items"].([]any) {
		item, _ := it.(map[string]any)
		if item["type"] == "consumption" && item["value"] == "in_progress" {
			inProgressID = item["id"].(string)
		}
	}
	if inProgressID == "" {
		t.Fatalf("in_progress row not found in history: %v", hist["items"])
	}
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/consumption", map[string]any{
		"request_id":        "req-consumption-3",
		"expected_revision": 1,
		"value":             "paused",
		"supersedes_id":     inProgressID,
		"reason":            "on a break",
	})
	data = readingData(t, resp, raw)
	if got := data["consumption"].(map[string]any)["value"]; got != "paused" {
		t.Fatalf("consumption after correction = %v, want paused", got)
	}

	// An unknown value is an invalid argument.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/consumption", map[string]any{
		"request_id":        "req-consumption-bad",
		"expected_revision": 2,
		"value":             "abandoned",
	})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// A stale revision conflicts.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/consumption", map[string]any{
		"request_id":        "req-consumption-stale",
		"expected_revision": 1,
		"value":             "completed",
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))

	// A malformed observed_at is an invalid argument.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/consumption", map[string]any{
		"request_id":        "req-consumption-bad-time",
		"expected_revision": 2,
		"value":             "completed",
		"observed_at":       "yesterday",
	})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
}

func TestHTTPReadingHistory(t *testing.T) {
	var editionID string
	srv := startCatalogServer(t, func(s *app.Service) {
		_, ed := seedWork(t, s, "seed-history", []store.TitleInput{titleInput("History Story", domain.TitleOriginal)}, appKind(domain.KindAnime))
		if ed == nil {
			t.Fatal("seed created no edition")
		}
		editionID = *ed
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	// Build five history rows: two progress rows (one superseded) and two
	// consumption rows (one superseded), plus a retraction below.
	write := func(method, path string, body map[string]any) map[string]any {
		t.Helper()
		resp, raw := catalogWrite(t, srv, cookie, csrf, method, "/api/v1/editions/"+editionID+path, body)
		return readingData(t, resp, raw)
	}
	write(http.MethodPost, "/progress", map[string]any{
		"request_id": "req-h-1", "expected_revision": 0, "unit": "episode", "position": "3", "total": "12",
	})
	d2 := write(http.MethodPost, "/progress", map[string]any{
		"request_id": "req-h-2", "expected_revision": 1, "unit": "episode", "position": "7", "total": "12",
	})
	ep7 := d2["progress"].(map[string]any)["episode"].(map[string]any)
	write(http.MethodPost, "/progress", map[string]any{
		"request_id": "req-h-3", "expected_revision": 2, "unit": "episode", "position": "6", "total": "12",
		"supersedes_id": ep7["id"].(string), "reason": "miscounted",
	})
	write(http.MethodPost, "/consumption", map[string]any{
		"request_id": "req-h-4", "expected_revision": 3, "value": "in_progress",
	})
	var inProgressID string
	resp, raw := catalogGet(t, srv, cookie, "/api/v1/editions/"+editionID+"/history")
	data := readingData(t, resp, raw)
	for _, it := range data["items"].([]any) {
		item, _ := it.(map[string]any)
		if item["type"] == "consumption" && item["value"] == "in_progress" {
			inProgressID = item["id"].(string)
		}
	}
	if inProgressID == "" {
		t.Fatalf("in_progress row not found in history: %v", data["items"])
	}
	write(http.MethodPost, "/consumption", map[string]any{
		"request_id": "req-h-5", "expected_revision": 4, "value": "paused",
		"supersedes_id": inProgressID, "reason": "on a break",
	})

	// The history preserves every previous row with its timestamps.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+editionID+"/history")
	data = readingData(t, resp, raw)
	if got := data["total"].(float64); got != 6 {
		t.Fatalf("history total = %v, want 6 rows", data["total"])
	}
	items, _ := data["items"].([]any)
	if len(items) != 6 {
		t.Fatalf("history items = %d, want 6", len(items))
	}
	byID := map[string]map[string]any{}
	for _, it := range items {
		item, _ := it.(map[string]any)
		byID[item["id"].(string)] = item
		if item["recorded_at"] == nil || item["recorded_at"] == "" {
			t.Fatalf("history row without a recorded_at: %v", item)
		}
	}
	// The superseded 7 row and its correction both exist.
	if item := byID[ep7["id"].(string)]; item == nil {
		t.Fatal("the superseded episode-7 row is missing from history")
	} else if item["value"] != "episode" || item["detail"] != "7/12" {
		t.Fatalf("superseded row value/detail = %v/%v, want episode/7/12", item["value"], item["detail"])
	}
	if len(byID) != 6 {
		t.Fatalf("distinct history rows = %d, want 6", len(byID))
	}

	// Pagination: one row per page with a cursor.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+editionID+"/history?limit=2")
	data = readingData(t, resp, raw)
	items, _ = data["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("page 1 items = %d, want 2", len(items))
	}
	cursor, _ := data["next_cursor"].(string)
	if cursor == "" {
		t.Fatalf("page 1 without a next cursor (total %v)", data["total"])
	}
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+editionID+"/history?limit=2&cursor="+cursor)
	data = readingData(t, resp, raw)
	items, _ = data["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("page 2 items = %d, want 2", len(items))
	}
	// A malformed cursor is an invalid argument.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+editionID+"/history?cursor=garbage")
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
	// A non-numeric limit is an invalid argument.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+editionID+"/history?limit=many")
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
	// An unknown edition is a JSON 404.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/does-not-exist/history")
	expectFailure(t, resp, raw, http.StatusNotFound, string(app.ErrNotFound))
}

func TestHTTPReadingRetract(t *testing.T) {
	var editionID string
	srv := startCatalogServer(t, func(s *app.Service) {
		_, ed := seedWork(t, s, "seed-retract", []store.TitleInput{titleInput("Retract Story", domain.TitleOriginal)}, appKind(domain.KindBook))
		if ed == nil {
			t.Fatal("seed created no edition")
		}
		editionID = *ed
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	// Record a page position, then retract it through the typed route.
	resp, raw := catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/progress", map[string]any{
		"request_id": "req-retract-record", "expected_revision": 0, "unit": "page", "position": "120", "total": "200",
	})
	data := readingData(t, resp, raw)
	page := data["progress"].(map[string]any)["page"].(map[string]any)
	recordID := page["id"].(string)

	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/history/"+recordID+"/retract", map[string]any{
		"request_id":        "req-retract-1",
		"expected_revision": 1,
		"reason":            "wrong edition",
	})
	data = readingData(t, resp, raw)
	if data["progress"].(map[string]any)["page"] != nil {
		t.Fatalf("page progress after retraction = %v, want absent", data["progress"])
	}

	// The retracted row stays in history.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+editionID+"/history")
	data = readingData(t, resp, raw)
	items, _ := data["items"].([]any)
	found := false
	for _, it := range items {
		item, _ := it.(map[string]any)
		if item["id"] == recordID {
			found = true
		}
	}
	if !found {
		t.Fatalf("retracted row missing from history: %v", items)
	}

	// A missing reason is an invalid argument.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/history/"+recordID+"/retract", map[string]any{
		"request_id": "req-retract-noreason", "expected_revision": 2,
	})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// An unknown record id is a JSON 404.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/history/does-not-exist/retract", map[string]any{
		"request_id": "req-retract-unknown", "expected_revision": 2, "reason": "no such row",
	})
	expectFailure(t, resp, raw, http.StatusNotFound, string(app.ErrNotFound))

	// A stale revision conflicts.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/history/"+recordID+"/retract", map[string]any{
		"request_id": "req-retract-stale", "expected_revision": 1, "reason": "stale",
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))
}

func TestHTTPReadingAuth(t *testing.T) {
	var editionID string
	srv := startCatalogServer(t, func(s *app.Service) {
		_, ed := seedWork(t, s, "seed-reading-auth", []store.TitleInput{titleInput("Auth Story", domain.TitleOriginal)}, appKind(domain.KindBook))
		if ed == nil {
			t.Fatal("seed created no edition")
		}
		editionID = *ed
	})
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	body := map[string]any{
		"request_id": "req-reading-auth", "expected_revision": 0, "unit": "chapter", "position": "1",
	}

	// No session: unauthorized.
	resp, raw := doJSON(t, srv, http.MethodPost, "/api/v1/editions/"+editionID+"/progress", nil, body)
	expectFailure(t, resp, raw, http.StatusUnauthorized, string(app.ErrUnauthorized))

	// A stale session: unauthorized.
	resp, raw = doJSON(t, srv, http.MethodPost, "/api/v1/editions/"+editionID+"/progress", map[string]string{"Cookie": SessionCookieName + "=stale"}, body)
	expectFailure(t, resp, raw, http.StatusUnauthorized, string(app.ErrUnauthorized))

	// A live session without the CSRF token: forbidden.
	resp, raw = doJSON(t, srv, http.MethodPost, "/api/v1/editions/"+editionID+"/consumption", map[string]string{"Cookie": cookie.String()}, map[string]any{
		"request_id": "req-reading-auth-2", "expected_revision": 0, "value": "in_progress",
	})
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// A live session with the wrong CSRF token: forbidden.
	resp, raw = doJSON(t, srv, http.MethodPost, "/api/v1/editions/"+editionID+"/progress", map[string]string{"Cookie": cookie.String(), CSRFHeader: "wrong-token"}, body)
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// A foreign origin: forbidden by the security boundary.
	resp, raw = doJSON(t, srv, http.MethodPost, "/api/v1/editions/"+editionID+"/history/some-record/retract", map[string]string{
		"Cookie": cookie.String(), CSRFHeader: csrf, "Origin": "http://evil.example",
	}, map[string]any{"request_id": "req-reading-auth-3", "expected_revision": 0, "reason": "nope"})
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// None of the rejected mutations touched the edition: no progress was
	// recorded and the consumption stays the seeded default.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+editionID)
	data := readingData(t, resp, raw)
	if p, ok := data["progress"].(map[string]any); ok && p["chapter"] != nil {
		t.Fatalf("progress after the rejected mutations = %v, want absent", data["progress"])
	}
	if got := data["consumption"].(map[string]any)["value"]; got != "not_started" {
		t.Fatalf("consumption after the rejected mutations = %v, want not_started", data["consumption"])
	}

	// History still needs only the session, not the CSRF token.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+editionID+"/history")
	expectSuccess(t, resp, raw)
}
