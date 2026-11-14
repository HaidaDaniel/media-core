// MCP request tests for the catalog write tools: the candidate decision
// stays explicit (an unresolved candidate creates nothing until the
// user's choice with its token arrives), a matching identity never
// clones, the idempotency key replays exact retries and conflicts on
// changed payloads, and the catalog corrections enforce the revision
// guard with distinguishable failures. The client talks to the server
// through the SDK in-memory transport against a real temporary database.
package mcp

import (
	"encoding/json"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func TestMCPCatalogWrite(t *testing.T) {
	testActor := map[string]any{"principal": "hermes-agent", "origin": "agent"}
	titles := func(text string) []map[string]any {
		return []map[string]any{{"text": text, "kind": "original", "provenance": "manual"}}
	}

	t.Run("an unresolved candidate creates nothing until the explicit choice", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Candidate Target", Kind: domain.TitleOriginal, Provenance: "manual"})

		// The resolve finds the exact title match (case-folded) and
		// hands back the candidates with their short-lived token.
		res := callTool(t, f, ToolResolveAddition, map[string]any{
			"titles": titles("candidate target"),
			"kind":   "manga",
		})
		if res.IsError {
			t.Fatalf("resolve failed: %s", resultText(t, res))
		}
		var resolve app.ResolveResult
		if err := json.Unmarshal([]byte(resultText(t, res)), &resolve); err != nil {
			t.Fatalf("unmarshal resolve result: %v", err)
		}
		if string(resolve.Outcome) != "candidates" || resolve.CandidateToken == nil {
			t.Fatalf("expected the candidates outcome with a token, got %+v", resolve)
		}
		if len(resolve.Candidates) != 1 || resolve.Candidates[0].WorkID != work {
			t.Fatalf("candidates = %+v, want the seeded work %v", resolve.Candidates, work)
		}

		// A create without the explicit choice is a duplicate_candidate
		// error and creates nothing.
		worksBefore, editionsBefore := counts(t, f.db)
		res = callTool(t, f, ToolCreateEntry, map[string]any{
			"request_id": "req-no-choice",
			"actor":      testActor,
			"titles":     titles("candidate target"),
			"kind":       "manga",
		})
		if code := failureCode(t, res); code != "duplicate_candidate" {
			t.Fatalf("no-choice create failure code = %q, want duplicate_candidate", code)
		}
		if works, editions := counts(t, f.db); works != worksBefore || editions != editionsBefore {
			t.Fatalf("an unresolved candidate must create nothing: works %d -> %d, editions %d -> %d", worksBefore, works, editionsBefore, editions)
		}

		// use_existing with the token resolves to the existing work.
		res = callTool(t, f, ToolCreateEntry, map[string]any{
			"request_id":      "req-use-existing",
			"actor":           testActor,
			"titles":          titles("candidate target"),
			"kind":            "manga",
			"choice":          "use_existing",
			"candidate_token": *resolve.CandidateToken,
		})
		if res.IsError {
			t.Fatalf("use_existing failed: %s", resultText(t, res))
		}
		var existing app.AdditionResult
		if err := json.Unmarshal([]byte(resultText(t, res)), &existing); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if existing.Created || existing.WorkID != work {
			t.Fatalf("use_existing must not create: %+v", existing)
		}

		// add_edition with the token and the explicit work adds an
		// edition to that work.
		res = callTool(t, f, ToolCreateEntry, map[string]any{
			"request_id":      "req-add-edition",
			"actor":           testActor,
			"titles":          titles("candidate target"),
			"kind":            "manga",
			"choice":          "add_edition",
			"work_id":         work,
			"candidate_token": *resolve.CandidateToken,
		})
		if res.IsError {
			t.Fatalf("add_edition failed: %s", resultText(t, res))
		}
		var added app.AdditionResult
		if err := json.Unmarshal([]byte(resultText(t, res)), &added); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if !added.Created || added.WorkID != work || added.EditionID == nil {
			t.Fatalf("add_edition must create an edition on the same work: %+v", added)
		}

		// create_distinct re-resolves (the add_edition above changed the
		// candidate set, so the earlier token is stale) and creates a
		// separate work with the fresh token.
		res = callTool(t, f, ToolResolveAddition, map[string]any{
			"titles": titles("candidate target"),
			"kind":   "manga",
		})
		if res.IsError {
			t.Fatalf("re-resolve failed: %s", resultText(t, res))
		}
		var fresh app.ResolveResult
		if err := json.Unmarshal([]byte(resultText(t, res)), &fresh); err != nil {
			t.Fatalf("unmarshal re-resolve: %v", err)
		}
		if fresh.CandidateToken == nil {
			t.Fatalf("expected a fresh candidate token: %+v", fresh)
		}
		res = callTool(t, f, ToolCreateEntry, map[string]any{
			"request_id":      "req-create-distinct",
			"actor":           testActor,
			"titles":          titles("candidate target"),
			"kind":            "manga",
			"choice":          "create_distinct",
			"candidate_token": *fresh.CandidateToken,
		})
		if res.IsError {
			t.Fatalf("create_distinct failed: %s", resultText(t, res))
		}
		var distinct app.AdditionResult
		if err := json.Unmarshal([]byte(resultText(t, res)), &distinct); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if !distinct.Created || distinct.WorkID == work {
			t.Fatalf("create_distinct must create a separate work: %+v", distinct)
		}
		gotWorks, _ := counts(t, f.db)
		if gotWorks != worksBefore+1 {
			t.Fatalf("create_distinct must add exactly one work: %d -> %d", worksBefore, gotWorks)
		}
	})

	t.Run("a matching external identity never clones", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Identity Target", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, work, domain.KindManga)
		if _, err := f.service.AddReference(f.ctx, app.AddReferenceRequest{
			MutationBase: app.MutationBase{
				RequestID:        "seed-reference",
				Actor:            app.Actor{Principal: "seed", Origin: domain.OriginUser},
				ExpectedRevision: ptr(int64(0)),
			},
			EditionID:  ed,
			Provider:   "mangadex",
			ExternalID: ptr("abc123"),
			Role:       domain.RoleMetadata,
			Provenance: "manual",
		}); err != nil {
			t.Fatalf("seed reference: %v", err)
		}

		// The resolve reports the existing entry, not candidates.
		res := callTool(t, f, ToolResolveAddition, map[string]any{
			"references": []map[string]any{{"provider": "mangadex", "external_id": "abc123"}},
		})
		if res.IsError {
			t.Fatalf("resolve failed: %s", resultText(t, res))
		}
		var resolve app.ResolveResult
		if err := json.Unmarshal([]byte(resultText(t, res)), &resolve); err != nil {
			t.Fatalf("unmarshal resolve result: %v", err)
		}
		if string(resolve.Outcome) != "existing" || resolve.WorkID == nil || *resolve.WorkID != work {
			t.Fatalf("expected the existing outcome with the seeded work, got %+v", resolve)
		}

		// A create with the matching identity resolves to the existing
		// entry: no clone, no reassignment.
		res = callTool(t, f, ToolCreateEntry, map[string]any{
			"request_id": "req-identity",
			"actor":      testActor,
			"references": []map[string]any{{"provider": "mangadex", "external_id": "abc123"}},
		})
		if res.IsError {
			t.Fatalf("create failed: %s", resultText(t, res))
		}
		var r app.AdditionResult
		if err := json.Unmarshal([]byte(resultText(t, res)), &r); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if r.Created || r.WorkID != work {
			t.Fatalf("a matching identity must not clone: %+v", r)
		}
		gotWorks, _ := counts(t, f.db)
		if gotWorks != 1 {
			t.Fatalf("no clone may add a work: %d works", gotWorks)
		}
	})

	t.Run("the idempotency key replays exact retries and conflicts on changed payloads", func(t *testing.T) {
		f := startLibraryFixture(t)
		payload := map[string]any{
			"request_id": "req-replay",
			"actor":      testActor,
			"titles":     titles("Replay Target"),
		}
		res := callTool(t, f, ToolCreateEntry, payload)
		if res.IsError {
			t.Fatalf("create failed: %s", resultText(t, res))
		}
		var first app.AdditionResult
		if err := json.Unmarshal([]byte(resultText(t, res)), &first); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if !first.Created {
			t.Fatalf("expected a created entry: %+v", first)
		}

		// The lost-response retry: the same key and payload replays the
		// stored result without creating again.
		res = callTool(t, f, ToolCreateEntry, payload)
		if res.IsError {
			t.Fatalf("retry failed: %s", resultText(t, res))
		}
		var replay app.AdditionResult
		if err := json.Unmarshal([]byte(resultText(t, res)), &replay); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if !replay.Created || replay.WorkID != first.WorkID {
			t.Fatalf("the retry must replay the stored result: %+v vs %+v", replay, first)
		}
		gotWorks, _ := counts(t, f.db)
		if gotWorks != 1 {
			t.Fatalf("the retry must not create again: %d works", gotWorks)
		}

		// The same key with a different payload is a conflict.
		res = callTool(t, f, ToolCreateEntry, map[string]any{
			"request_id": "req-replay",
			"actor":      testActor,
			"titles":     titles("A Different Title"),
		})
		if code := failureCode(t, res); code != "conflict" {
			t.Fatalf("changed-payload retry failure code = %q, want conflict", code)
		}
	})

	t.Run("catalog corrections enforce the revision guard and reject invalid shapes", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Update Target", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, work, domain.KindManga)
		w, err := f.service.GetWork(f.ctx, work)
		if err != nil {
			t.Fatalf("GetWork: %v", err)
		}

		// A stale revision is a conflict and the prior state stays intact.
		res := callTool(t, f, ToolUpdateCatalog, map[string]any{
			"request_id":        "req-stale",
			"actor":             testActor,
			"action":            "add_title",
			"expected_revision": 999,
			"target_scope":      "work",
			"work_id":           work,
			"add_titles":        []map[string]any{{"text": "Stale Title", "kind": "alias", "provenance": "manual"}},
		})
		if code := failureCode(t, res); code != "conflict" {
			t.Fatalf("stale revision failure code = %q, want conflict", code)
		}
		afterStale, err := f.service.GetWork(f.ctx, work)
		if err != nil {
			t.Fatalf("GetWork: %v", err)
		}
		if len(afterStale.Titles) != len(w.Titles) {
			t.Fatalf("a stale revision must not change titles: %d -> %d", len(w.Titles), len(afterStale.Titles))
		}

		// The current revision succeeds and the title lands.
		res = callTool(t, f, ToolUpdateCatalog, map[string]any{
			"request_id":        "req-add-title",
			"actor":             testActor,
			"action":            "add_title",
			"expected_revision": w.Revision,
			"target_scope":      "work",
			"work_id":           work,
			"add_titles":        []map[string]any{{"text": "New Alias", "kind": "alias", "provenance": "manual"}},
		})
		if res.IsError {
			t.Fatalf("add_title failed: %s", resultText(t, res))
		}
		var updatedWork app.WorkView
		if err := json.Unmarshal([]byte(resultText(t, res)), &updatedWork); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if len(updatedWork.Titles) != len(w.Titles)+1 {
			t.Fatalf("add_title must add one title: %d -> %d", len(w.Titles), len(updatedWork.Titles))
		}

		// update_metadata changes the edition kind.
		res = callTool(t, f, ToolUpdateCatalog, map[string]any{
			"request_id":        "req-meta",
			"actor":             testActor,
			"action":            "update_metadata",
			"expected_revision": int64(0),
			"edition_id":        ed,
			"kind":              "book",
		})
		if res.IsError {
			t.Fatalf("update_metadata failed: %s", resultText(t, res))
		}
		var updatedEdition app.EditionView
		if err := json.Unmarshal([]byte(resultText(t, res)), &updatedEdition); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if updatedEdition.Kind != domain.KindBook {
			t.Fatalf("update_metadata must change the kind, got %v", updatedEdition.Kind)
		}

		// archive with the revision the metadata correction advanced.
		res = callTool(t, f, ToolUpdateCatalog, map[string]any{
			"request_id":        "req-archive",
			"actor":             testActor,
			"action":            "archive",
			"expected_revision": updatedEdition.Revision,
			"target_scope":      "edition",
			"edition_id":        ed,
		})
		if res.IsError {
			t.Fatalf("archive failed: %s", resultText(t, res))
		}
		var archivedAt string
		if err := f.db.DB().QueryRow("SELECT archived_at FROM editions WHERE id = ?", ed).Scan(&archivedAt); err != nil {
			t.Fatalf("read archived_at: %v", err)
		}
		if archivedAt == "" {
			t.Fatalf("archive must set the archived instant")
		}

		// Invalid shapes fail with a clear invalid_argument.
		if code := failureCode(t, callTool(t, f, ToolUpdateCatalog, map[string]any{
			"request_id": "req-no-scope",
			"actor":      testActor,
			"action":     "add_title",
			"work_id":    work,
			"add_titles": []map[string]any{{"text": "X", "kind": "alias", "provenance": "manual"}},
		})); code != "invalid_argument" {
			t.Fatalf("missing target_scope failure code = %q, want invalid_argument", code)
		}
		if code := failureCode(t, callTool(t, f, ToolUpdateCatalog, map[string]any{
			"request_id":   "req-scope-mismatch",
			"actor":        testActor,
			"action":       "archive",
			"target_scope": "work",
			"edition_id":   ed,
		})); code != "invalid_argument" {
			t.Fatalf("scope/id mismatch failure code = %q, want invalid_argument", code)
		}
		// An action outside the bounded enum is rejected by the schema.
		res = callTool(t, f, ToolUpdateCatalog, map[string]any{
			"request_id": "req-bogus-action",
			"actor":      testActor,
			"action":     "drop_table",
		})
		if !res.IsError {
			t.Fatalf("an unknown action must fail: %s", resultText(t, res))
		}
	})
}
