// MCP request tests for the release import and binding tools: a valid
// envelope stages a pending event, an explicit bind projects it and clears
// the pending queue, a conflicting bind is a conflict, an explicit rebind
// moves the projection without a duplicate, a revoke un-projects it, and an
// unsupported schema major writes nothing. The client talks to the server
// through the SDK in-memory transport against a real temporary database.
package mcp

import (
	"encoding/json"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func releaseEnvelopeMap(eventID, publicationID string) map[string]any {
	return map[string]any{
		"schema_version": "1.0",
		"ok":             true,
		"data": map[string]any{
			"events": []map[string]any{{
				"schema_version": "1.0",
				"event_id":       eventID,
				"event_type":     "release.added",
				"publication_id": publicationID,
				"release_number": "1",
				"release_title":  "Chapter 1",
			}},
		},
	}
}

// TestReleaseBinding proves the release vertical slice over MCP: the bounded
// JSON batch stages a pending event, the explicit bind/rebind/revoke controls
// project and un-project it, the read tools expose the projection and the
// active binding, and the domain rules (conflict, no duplicate projection,
// the pending queue) are the shared service's.
func TestReleaseBinding(t *testing.T) {
	testActor := map[string]any{"principal": "hermes-agent", "origin": "agent"}

	t.Run("import, bind, rebind and revoke the release history", func(t *testing.T) {
		f := startLibraryFixture(t)
		workA := seedWork(t, f.db, store.TitleInput{Text: "Release One", Kind: domain.TitleOriginal, Provenance: "manual"})
		edA := seedEdition(t, f.db, workA, domain.KindManga)
		workB := seedWork(t, f.db, store.TitleInput{Text: "Release Two", Kind: domain.TitleOriginal, Provenance: "manual"})
		edB := seedEdition(t, f.db, workB, domain.KindWebNovel)

		// Import one event: it stages pending.
		res := callTool(t, f, ToolImportReleases, map[string]any{
			"envelope": releaseEnvelopeMap("evt_mcp", "pub_mcp"),
		})
		if res.IsError {
			t.Fatalf("import_releases failed: %s", resultText(t, res))
		}
		var imp app.ImportResult
		if err := json.Unmarshal([]byte(resultText(t, res)), &imp); err != nil {
			t.Fatalf("unmarshal import result: %v", err)
		}
		if imp.Imported != 1 || imp.Conflicts != 0 {
			t.Fatalf("expected one imported event, got %+v", imp)
		}

		// Before a binding the event is pending.
		res = callTool(t, f, ToolListPendingImport, nil)
		if res.IsError {
			t.Fatalf("list_pending_imports failed: %s", resultText(t, res))
		}
		var pending app.ListPayload
		if err := json.Unmarshal([]byte(resultText(t, res)), &pending); err != nil {
			t.Fatalf("unmarshal pending: %v", err)
		}
		if pending.Total != 1 {
			t.Fatalf("expected one pending event, got %d", pending.Total)
		}

		// Bind the publication to the first edition.
		res = callTool(t, f, ToolBindRelease, map[string]any{
			"request_id": "bind-1", "actor": testActor,
			"publication_id": "pub_mcp", "edition_id": edA,
		})
		if res.IsError {
			t.Fatalf("bind_release failed: %s", resultText(t, res))
		}
		var bound app.ReleaseBindingView
		if err := json.Unmarshal([]byte(resultText(t, res)), &bound); err != nil {
			t.Fatalf("unmarshal binding: %v", err)
		}
		if bound.EditionID != edA || bound.State != "active" {
			t.Fatalf("expected an active binding on edition A, got %+v", bound)
		}

		// The event is no longer pending and projects for edition A only.
		res = callTool(t, f, ToolListPendingImport, nil)
		if res.IsError {
			t.Fatalf("list_pending_imports failed: %s", resultText(t, res))
		}
		if err := json.Unmarshal([]byte(resultText(t, res)), &pending); err != nil {
			t.Fatalf("unmarshal pending: %v", err)
		}
		if pending.Total != 0 {
			t.Fatalf("expected no pending events after the bind, got %d", pending.Total)
		}
		res = callTool(t, f, ToolListReleases, map[string]any{"edition_id": edA})
		if res.IsError {
			t.Fatalf("list_releases failed: %s", resultText(t, res))
		}
		var relA app.ListPayload
		if err := json.Unmarshal([]byte(resultText(t, res)), &relA); err != nil {
			t.Fatalf("unmarshal releases: %v", err)
		}
		if relA.Total != 1 {
			t.Fatalf("expected one projection for edition A, got %d", relA.Total)
		}
		res = callTool(t, f, ToolListReleases, map[string]any{"edition_id": edB})
		if res.IsError {
			t.Fatalf("list_releases failed: %s", resultText(t, res))
		}
		var relB app.ListPayload
		if err := json.Unmarshal([]byte(resultText(t, res)), &relB); err != nil {
			t.Fatalf("unmarshal releases: %v", err)
		}
		if relB.Total != 0 {
			t.Fatalf("expected no projection for edition B before the rebind, got %d", relB.Total)
		}

		// A conflicting bind to a different edition is a conflict.
		res = callTool(t, f, ToolBindRelease, map[string]any{
			"request_id": "bind-conflict", "actor": testActor,
			"publication_id": "pub_mcp", "edition_id": edB,
		})
		if code := failureCode(t, res); code != "conflict" {
			t.Fatalf("conflicting bind code = %q, want conflict", code)
		}

		// The explicit rebind moves the projection to edition B.
		res = callTool(t, f, ToolRebindRelease, map[string]any{
			"request_id": "rebind-1", "actor": testActor,
			"publication_id": "pub_mcp", "edition_id": edB,
		})
		if res.IsError {
			t.Fatalf("rebind_release failed: %s", resultText(t, res))
		}
		var rebound app.ReleaseBindingView
		if err := json.Unmarshal([]byte(resultText(t, res)), &rebound); err != nil {
			t.Fatalf("unmarshal binding: %v", err)
		}
		if rebound.EditionID != edB || rebound.ReplacesID == nil {
			t.Fatalf("expected a rebind to edition B with a replaces link, got %+v", rebound)
		}
		res = callTool(t, f, ToolListReleases, map[string]any{"edition_id": edB})
		if res.IsError {
			t.Fatalf("list_releases failed: %s", resultText(t, res))
		}
		if err := json.Unmarshal([]byte(resultText(t, res)), &relB); err != nil {
			t.Fatalf("unmarshal releases: %v", err)
		}
		if relB.Total != 1 {
			t.Fatalf("expected one projection for edition B after the rebind, got %d", relB.Total)
		}
		res = callTool(t, f, ToolListReleases, map[string]any{"edition_id": edA})
		if res.IsError {
			t.Fatalf("list_releases failed: %s", resultText(t, res))
		}
		if err := json.Unmarshal([]byte(resultText(t, res)), &relA); err != nil {
			t.Fatalf("unmarshal releases: %v", err)
		}
		if relA.Total != 0 {
			t.Fatalf("expected no projection for edition A after the rebind, got %d (no duplicate)", relA.Total)
		}

		// The active binding read reports the replacement.
		res = callTool(t, f, ToolGetReleaseBinding, map[string]any{"publication_id": "pub_mcp"})
		if res.IsError {
			t.Fatalf("get_release_binding failed: %s", resultText(t, res))
		}
		var active struct {
			Active *app.ReleaseBindingView `json:"active"`
		}
		if err := json.Unmarshal([]byte(resultText(t, res)), &active); err != nil {
			t.Fatalf("unmarshal active binding: %v", err)
		}
		if active.Active == nil || active.Active.EditionID != edB {
			t.Fatalf("expected the active binding on edition B, got %+v", active.Active)
		}

		// Revoking un-projects the event from every edition.
		res = callTool(t, f, ToolRevokeRelease, map[string]any{
			"request_id": "revoke-1", "actor": testActor, "publication_id": "pub_mcp",
		})
		if res.IsError {
			t.Fatalf("revoke_release failed: %s", resultText(t, res))
		}
		res = callTool(t, f, ToolListReleases, map[string]any{"edition_id": edB})
		if res.IsError {
			t.Fatalf("list_releases failed: %s", resultText(t, res))
		}
		if err := json.Unmarshal([]byte(resultText(t, res)), &relB); err != nil {
			t.Fatalf("unmarshal releases: %v", err)
		}
		if relB.Total != 0 {
			t.Fatalf("expected no projection after the revoke, got %d", relB.Total)
		}
	})

	t.Run("an unsupported schema major writes nothing", func(t *testing.T) {
		f := startLibraryFixture(t)
		envelope := releaseEnvelopeMap("evt_major", "pub_major")
		envelope["schema_version"] = "2.0"
		res := callTool(t, f, ToolImportReleases, map[string]any{"envelope": envelope})
		if code := failureCode(t, res); code != "invalid_argument" {
			t.Fatalf("unsupported major code = %q, want invalid_argument", code)
		}
		res = callTool(t, f, ToolListPendingImport, nil)
		if res.IsError {
			t.Fatalf("list_pending_imports failed: %s", resultText(t, res))
		}
		var pending app.ListPayload
		if err := json.Unmarshal([]byte(resultText(t, res)), &pending); err != nil {
			t.Fatalf("unmarshal pending: %v", err)
		}
		if pending.Total != 0 {
			t.Fatalf("the unsupported major must write nothing, got %d pending events", pending.Total)
		}
	})
}
