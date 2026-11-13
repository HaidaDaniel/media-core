package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// startAnnotationServer serves an initialized state directory with one
// seeded work and edition and returns the ids plus the service for
// service-level retrieval checks.
func startAnnotationServer(t *testing.T) (*Server, string, string, *app.Service) {
	t.Helper()
	var workID, editionID string
	var svcRef *app.Service
	srv := startCatalogServer(t, func(s *app.Service) {
		svcRef = s
		var ed *string
		workID, ed = seedWork(t, s, "seed-annotation", []store.TitleInput{titleInput("Annotation Story", domain.TitleOriginal)}, appKind(domain.KindManga))
		if ed == nil {
			t.Fatal("seed created no edition")
		}
		editionID = *ed
	})
	return srv, workID, editionID, svcRef
}

// historyIDs lists the history row ids of one entity through the shared
// service; the HTTP history route is edition-scoped, so work-scoped
// retrieval is checked at the service level.
func historyIDs(t *testing.T, s *app.Service, workID *string, editionID *string) []string {
	t.Helper()
	payload, err := s.GetHistory(context.Background(), app.HistoryQuery{WorkID: workID, EditionID: editionID})
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	ids := make([]string, 0, len(payload.Items))
	for _, item := range payload.Items {
		ids = append(ids, item.(app.ActivityItem).ID)
	}
	return ids
}

// inIDs reports whether want is among ids.
func inIDs(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func TestHTTPAnnotationIntention(t *testing.T) {
	srv, _, editionID, _ := startAnnotationServer(t)
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	// Record an intention.
	resp, raw := catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/intention", map[string]any{
		"request_id": "req-int-1", "expected_revision": 0, "value": "interested",
	})
	data := readingData(t, resp, raw)
	if got := data["revision"].(float64); got != 1 {
		t.Fatalf("revision after intention = %v, want 1", data["revision"])
	}
	if got := data["intention"].(map[string]any)["value"]; got != "interested" {
		t.Fatalf("intention = %v, want interested", data["intention"])
	}
	if data["consumption"].(map[string]any)["value"] != "not_started" {
		t.Fatalf("intention must not change consumption: %v", data["consumption"])
	}

	// The same current value is a no-op without a revision bump.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/intention", map[string]any{
		"request_id": "req-int-2", "expected_revision": 1, "value": "interested",
	})
	data = readingData(t, resp, raw)
	if got := data["revision"].(float64); got != 1 {
		t.Fatalf("revision after the same-value no-op = %v, want 1", data["revision"])
	}

	// A correction supersedes the previous row; the record id comes from
	// the history projection.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+editionID+"/history")
	hist := readingData(t, resp, raw)
	var interestedID string
	for _, it := range hist["items"].([]any) {
		item, _ := it.(map[string]any)
		if item["type"] == "intention" && item["value"] == "interested" {
			interestedID = item["id"].(string)
		}
	}
	if interestedID == "" {
		t.Fatalf("interested row not found in history: %v", hist["items"])
	}
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/intention", map[string]any{
		"request_id": "req-int-3", "expected_revision": 1, "value": "planned",
		"supersedes_id": interestedID,
	})
	data = readingData(t, resp, raw)
	if got := data["intention"].(map[string]any)["value"]; got != "planned" {
		t.Fatalf("intention after correction = %v, want planned", data["intention"])
	}

	// An unknown value is an invalid argument.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/intention", map[string]any{
		"request_id": "req-int-bad", "expected_revision": 2, "value": "curious",
	})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// A stale revision conflicts.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/intention", map[string]any{
		"request_id": "req-int-stale", "expected_revision": 1, "value": "saved",
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))
}

func TestHTTPAnnotationRating(t *testing.T) {
	srv, _, editionID, _ := startAnnotationServer(t)
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	// Rate eight of ten.
	resp, raw := catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/ratings", map[string]any{
		"request_id": "req-rate-1", "expected_revision": 0, "value": 8, "comment": "solid",
	})
	data := readingData(t, resp, raw)
	rating := data["rating"].(map[string]any)
	if rating["value"] != float64(8) || rating["comment"] != "solid" {
		t.Fatalf("rating = %v, want 8 with comment", rating)
	}

	// The same value and comment is a no-op without a revision bump.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/ratings", map[string]any{
		"request_id": "req-rate-2", "expected_revision": 1, "value": 8, "comment": "solid",
	})
	data = readingData(t, resp, raw)
	if got := data["revision"].(float64); got != 1 {
		t.Fatalf("revision after the same-value no-op = %v, want 1", data["revision"])
	}

	// A correction supersedes the previous row.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+editionID+"/history")
	hist := readingData(t, resp, raw)
	var rate8ID string
	for _, it := range hist["items"].([]any) {
		item, _ := it.(map[string]any)
		if item["type"] == "rating" && item["value"] != nil && item["value"].(string) == "8" {
			rate8ID = item["id"].(string)
		}
	}
	if rate8ID == "" {
		t.Fatalf("rating-8 row not found in history: %v", hist["items"])
	}
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/ratings", map[string]any{
		"request_id": "req-rate-3", "expected_revision": 1, "value": 9,
		"supersedes_id": rate8ID,
	})
	data = readingData(t, resp, raw)
	if got := data["rating"].(map[string]any)["value"]; got != float64(9) {
		t.Fatalf("rating after correction = %v, want 9", got)
	}

	// A null value is the explicit clear, never zero.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/ratings", map[string]any{
		"request_id": "req-rate-4", "expected_revision": 2, "value": nil,
	})
	data = readingData(t, resp, raw)
	if data["rating"] != nil {
		t.Fatalf("rating after the clear = %v, want absent", data["rating"])
	}

	// A value outside the 1-10 bound is an invalid argument.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/ratings", map[string]any{
		"request_id": "req-rate-bad", "expected_revision": 3, "value": 11,
	})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// A stale revision conflicts.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/ratings", map[string]any{
		"request_id": "req-rate-stale", "expected_revision": 2, "value": 5,
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))
}

func TestHTTPAnnotationReference(t *testing.T) {
	srv, _, editionID, svc := startAnnotationServer(t)
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	// Attach one sourced reference.
	resp, raw := catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/references", map[string]any{
		"request_id": "req-ref-1", "expected_revision": 0,
		"provider": "mal", "external_id": "9001", "role": "metadata",
		"authoritative": false, "provenance": "manual",
	})
	data := readingData(t, resp, raw)
	refs, _ := data["references"].([]any)
	if len(refs) != 1 {
		t.Fatalf("references = %v, want one", data["references"])
	}
	refID := refs[0].(map[string]any)["id"].(string)

	// Retire it; the row stays in history.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/references/"+refID+"/retire", map[string]any{
		"request_id": "req-ref-2", "expected_revision": 1, "reason": "wrong source",
	})
	data = readingData(t, resp, raw)
	for _, it := range data["references"].([]any) {
		if it.(map[string]any)["id"] == refID {
			t.Fatalf("retired reference still active: %v", data["references"])
		}
	}
	if !inIDs(historyIDs(t, svc, nil, &editionID), refID) {
		t.Fatal("retired reference missing from history")
	}

	// Both an external id and a url is an invalid argument.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/references", map[string]any{
		"request_id": "req-ref-both", "expected_revision": 2,
		"provider": "mal", "external_id": "9002", "url": "https://example.org/x",
		"role": "metadata", "provenance": "manual",
	})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// A stale revision conflicts.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/editions/"+editionID+"/references", map[string]any{
		"request_id": "req-ref-stale", "expected_revision": 1,
		"provider": "mal", "url": "https://example.org/y",
		"role": "content", "provenance": "manual",
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))
}

func TestHTTPAnnotationNote(t *testing.T) {
	srv, workID, editionID, svc := startAnnotationServer(t)
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	// Add an edition-scoped note.
	resp, raw := catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/notes", map[string]any{
		"request_id": "req-note-1", "target": "edition", "target_id": editionID,
		"text": "first thought", "spoiler": true,
	})
	data := readingData(t, resp, raw)
	notes, _ := data["notes"].([]any)
	if len(notes) != 1 {
		t.Fatalf("notes = %v, want one", data["notes"])
	}
	note := notes[0].(map[string]any)
	noteID := note["id"].(string)
	if note["text"] != "first thought" || note["spoiler"] != true {
		t.Fatalf("note = %v, want the added text with the spoiler flag", note)
	}

	// A note's first revision is 1. Revise it: a new immutable revision
	// replaces the current text.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPatch, "/api/v1/notes/"+noteID, map[string]any{
		"request_id": "req-note-2", "expected_revision": 1, "text": "second thought",
	})
	data = readingData(t, resp, raw)
	notes, _ = data["notes"].([]any)
	if notes[0].(map[string]any)["text"] != "second thought" {
		t.Fatalf("note after the revision = %v, want second thought", notes)
	}
	if got := notes[0].(map[string]any)["revision"].(float64); got != 2 {
		t.Fatalf("note revision after the revision = %v, want 2", notes)
	}

	// A stale note revision conflicts.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPatch, "/api/v1/notes/"+noteID, map[string]any{
		"request_id": "req-note-stale", "expected_revision": 1, "text": "third thought",
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))

	// Retract it: the note becomes absent from the view but stays
	// retrievable in history.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/notes/"+noteID+"/retract", map[string]any{
		"request_id": "req-note-3", "expected_revision": 2, "reason": "no longer true",
	})
	data = readingData(t, resp, raw)
	if notes, _ := data["notes"].([]any); len(notes) != 0 {
		t.Fatalf("notes after the retraction = %v, want none", notes)
	}
	// The retraction revision stays retrievable in history with its
	// reason; note rows are keyed by revision id, not note id.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+editionID+"/history")
	hist := readingData(t, resp, raw)
	retracted := false
	for _, it := range hist["items"].([]any) {
		item, _ := it.(map[string]any)
		if item["type"] == "note" && item["label"] == "retracted" && item["reason"] == "no longer true" {
			retracted = true
		}
	}
	if !retracted {
		t.Fatalf("retracted note revision missing from history: %v", hist["items"])
	}

	// A work-scoped note is stored and retrievable through the work
	// history; the work view itself carries no notes field.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/notes", map[string]any{
		"request_id": "req-note-4", "target": "work", "target_id": workID, "text": "work thought",
	})
	data = readingData(t, resp, raw)
	if data["id"] != workID {
		t.Fatalf("work-scoped note target = %v, want %s", data["id"], workID)
	}
	var workNoteID string
	payload, err := svc.GetHistory(context.Background(), app.HistoryQuery{WorkID: &workID})
	if err != nil {
		t.Fatalf("work GetHistory: %v", err)
	}
	for _, item := range payload.Items {
		entry := item.(app.ActivityItem)
		if entry.Type == "note" && entry.Value != nil && *entry.Value == "work thought" {
			workNoteID = entry.ID
		}
	}
	if workNoteID == "" {
		t.Fatal("work-scoped note not retrievable in the work history")
	}

	// An unknown target scope is an invalid argument.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/notes", map[string]any{
		"request_id": "req-note-bad-target", "target": "series", "target_id": workID, "text": "x",
	})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// Empty text is an invalid argument.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/notes", map[string]any{
		"request_id": "req-note-empty", "target": "work", "target_id": workID, "text": "",
	})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))
}

func TestHTTPAnnotationTag(t *testing.T) {
	srv, workID, editionID, svc := startAnnotationServer(t)
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, csrf := login(t, srv, secret)

	// Attach an edition-scoped tag.
	resp, raw := catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/tags/assignments", map[string]any{
		"request_id": "req-tag-1", "expected_revision": 0,
		"target": "edition", "target_id": editionID, "tag": "comfort watch",
	})
	data := readingData(t, resp, raw)
	tags, _ := data["tags"].([]any)
	if len(tags) != 1 {
		t.Fatalf("tags = %v, want one", data["tags"])
	}

	// Detach it through DELETE.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodDelete, "/api/v1/tags/assignments", map[string]any{
		"request_id": "req-tag-2", "expected_revision": 1,
		"target": "edition", "target_id": editionID, "tag": "comfort watch",
	})
	data = readingData(t, resp, raw)
	if tags, _ := data["tags"].([]any); len(tags) != 0 {
		t.Fatalf("tags after the detach = %v, want none", tags)
	}

	// A work-scoped tag is stored and retrievable through the work
	// history; the work view itself carries no tags field.
	// The seeded work is at revision 1: its creation revision 0 was
	// bumped by the first edition.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/tags/assignments", map[string]any{
		"request_id": "req-tag-3", "expected_revision": 1,
		"target": "work", "target_id": workID, "tag": "shelf",
	})
	data = readingData(t, resp, raw)
	if data["id"] != workID {
		t.Fatalf("work-scoped tag target = %v, want %s", data["id"], workID)
	}
	payload, err := svc.GetHistory(context.Background(), app.HistoryQuery{WorkID: &workID})
	if err != nil {
		t.Fatalf("work GetHistory: %v", err)
	}
	found := false
	for _, item := range payload.Items {
		if item.(app.ActivityItem).Type == "tag" {
			found = true
		}
	}
	if !found {
		t.Fatal("work-scoped tag not retrievable in the work history")
	}

	// An unknown target scope is an invalid argument.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/tags/assignments", map[string]any{
		"request_id": "req-tag-bad-target", "expected_revision": 1,
		"target": "series", "target_id": workID, "tag": "x",
	})
	expectFailure(t, resp, raw, http.StatusBadRequest, string(app.ErrInvalidArgument))

	// A stale revision conflicts.
	resp, raw = catalogWrite(t, srv, cookie, csrf, http.MethodPost, "/api/v1/tags/assignments", map[string]any{
		"request_id": "req-tag-stale", "expected_revision": 1,
		"target": "work", "target_id": workID, "tag": "y",
	})
	expectFailure(t, resp, raw, http.StatusConflict, string(app.ErrConflict))
}

func TestHTTPAnnotationAuth(t *testing.T) {
	srv, _, editionID, _ := startAnnotationServer(t)
	secret, err := ReadOwnerSecretHex(srv.cfg.StateDir)
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	cookie, _ := login(t, srv, secret)

	// No session: unauthorized.
	resp, raw := doJSON(t, srv, http.MethodPost, "/api/v1/editions/"+editionID+"/intention", nil, map[string]any{
		"request_id": "req-ann-auth", "expected_revision": 0, "value": "saved",
	})
	expectFailure(t, resp, raw, http.StatusUnauthorized, string(app.ErrUnauthorized))

	// A live session without the CSRF token: forbidden.
	resp, raw = doJSON(t, srv, http.MethodPost, "/api/v1/editions/"+editionID+"/ratings", map[string]string{"Cookie": cookie.String()}, map[string]any{
		"request_id": "req-ann-auth-2", "expected_revision": 0, "value": 7,
	})
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// A live session with the wrong CSRF token: forbidden.
	resp, raw = doJSON(t, srv, http.MethodDelete, "/api/v1/tags/assignments", map[string]string{"Cookie": cookie.String(), CSRFHeader: "wrong-token"}, map[string]any{
		"request_id": "req-ann-auth-3", "expected_revision": 0, "target": "edition", "target_id": editionID, "tag": "x",
	})
	expectFailure(t, resp, raw, http.StatusForbidden, string(app.ErrForbidden))

	// None of the rejected mutations touched the edition.
	resp, raw = catalogGet(t, srv, cookie, "/api/v1/editions/"+editionID)
	data := readingData(t, resp, raw)
	if data["intention"] != nil {
		t.Fatalf("intention after the rejected mutations = %v, want absent", data["intention"])
	}
	if data["rating"] != nil {
		t.Fatalf("rating after the rejected mutations = %v, want absent", data["rating"])
	}
	if tags, _ := data["tags"].([]any); len(tags) != 0 {
		t.Fatalf("tags after the rejected mutations = %v, want none", tags)
	}
}
