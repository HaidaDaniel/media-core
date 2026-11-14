// MCP request tests for the relation, origin and activity tools: the
// "link the manga to the completed anime" example (a proposed edge that a
// confirmed one replaces only through the explicit intent guard), the
// confidence-is-never-confirmation invariant, the source-cycle guard with
// no partial mutation, retraction that keeps history, and the bounded
// activity feed with type filters. The client talks to the server through
// the SDK in-memory transport against a real temporary database.
package mcp

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// editionRev reads the current edition revision through the read tool.
func editionRev(t *testing.T, f *libraryFixture, ed string) int64 {
	t.Helper()
	res := callTool(t, f, ToolGetEdition, map[string]any{"edition_id": ed})
	if res.IsError {
		t.Fatalf("get_edition failed: %s", resultText(t, res))
	}
	var v app.EditionView
	if err := json.Unmarshal([]byte(resultText(t, res)), &v); err != nil {
		t.Fatalf("unmarshal view: %v", err)
	}
	return v.Revision
}

// workRev reads the current work revision through the read tool.
func workRev(t *testing.T, f *libraryFixture, work string) int64 {
	t.Helper()
	res := callTool(t, f, ToolGetWork, map[string]any{"work_id": work})
	if res.IsError {
		t.Fatalf("get_work failed: %s", resultText(t, res))
	}
	var v app.WorkView
	if err := json.Unmarshal([]byte(resultText(t, res)), &v); err != nil {
		t.Fatalf("unmarshal view: %v", err)
	}
	return v.Revision
}

func TestMCPRelation(t *testing.T) {
	testActor := map[string]any{"principal": "hermes-agent", "origin": "agent"}

	t.Run("propose then confirm links the manga to the completed anime", func(t *testing.T) {
		f := startLibraryFixture(t)
		mangaWork := seedWork(t, f.db, store.TitleInput{Text: "Manga", Kind: domain.TitleOriginal, Provenance: "manual"})
		mangaEd := seedEdition(t, f.db, mangaWork, domain.KindManga)
		animeWork := seedWork(t, f.db, store.TitleInput{Text: "Anime", Kind: domain.TitleOriginal, Provenance: "manual"})
		animeEd := seedEdition(t, f.db, animeWork, domain.KindAnime)

		// The anime is completed.
		res := callTool(t, f, ToolSetConsumption, map[string]any{
			"request_id": "req-anime-done", "actor": testActor, "edition_id": animeEd,
			"value": "completed", "expected_revision": 0,
		})
		if res.IsError {
			t.Fatalf("set_consumption failed: %s", resultText(t, res))
		}

		// Propose the adaptation: an agent guess arrives proposed.
		res = callTool(t, f, ToolWriteRelation, map[string]any{
			"request_id": "req-propose", "actor": testActor, "action": "propose",
			"scope": "edition", "type": "adaptation_of",
			"from_id": mangaEd, "to_id": animeEd,
			"provenance": "hermes-agent", "confidence": 0.9,
			"expected_revision": editionRev(t, f, mangaEd),
		})
		if res.IsError {
			t.Fatalf("propose failed: %s", resultText(t, res))
		}
		var proposed app.RelationView
		if err := json.Unmarshal([]byte(resultText(t, res)), &proposed); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if len(proposed.Proposed) != 1 || len(proposed.Confirmed) != 0 {
			t.Fatalf("the edge must be proposed, not confirmed: %+v", proposed)
		}
		relationID := proposed.Proposed[0].ID

		// A proposal is separately labelled, not traversed as confirmed.
		res = callTool(t, f, ToolListRelated, map[string]any{"scope": "edition", "entity_id": mangaEd})
		if res.IsError {
			t.Fatalf("list_related failed: %s", resultText(t, res))
		}
		var related app.RelatedView
		if err := json.Unmarshal([]byte(resultText(t, res)), &related); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if len(related.Edges) != 1 || related.Edges[0].State != domain.ClaimProposed {
			t.Fatalf("the proposal must be labelled proposed: %+v", related.Edges)
		}

		// Confirm with the explicit intent guard.
		res = callTool(t, f, ToolWriteRelation, map[string]any{
			"request_id": "req-confirm", "actor": testActor, "action": "confirm",
			"relation_id":  relationID,
			"confirmation": true, "user_instruction": "the user said to confirm the link",
			"expected_revision": editionRev(t, f, mangaEd),
		})
		if res.IsError {
			t.Fatalf("confirm failed: %s", resultText(t, res))
		}
		var confirmed app.RelationView
		if err := json.Unmarshal([]byte(resultText(t, res)), &confirmed); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if len(confirmed.Confirmed) != 1 || len(confirmed.Proposed) != 0 {
			t.Fatalf("the edge must now be confirmed: %+v", confirmed)
		}
		if confirmed.Confirmed[0].ConfirmedBy == nil || *confirmed.Confirmed[0].ConfirmedBy != "hermes-agent" {
			t.Fatalf("the confirmation must carry the confirming principal: %+v", confirmed.Confirmed[0])
		}

		// The confirmed edge is now traversed.
		res = callTool(t, f, ToolListRelated, map[string]any{"scope": "edition", "entity_id": mangaEd})
		if res.IsError {
			t.Fatalf("list_related failed: %s", resultText(t, res))
		}
		related = app.RelatedView{}
		if err := json.Unmarshal([]byte(resultText(t, res)), &related); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if len(related.Edges) != 1 || related.Edges[0].State != domain.ClaimConfirmed {
			t.Fatalf("the edge must now be traversed as confirmed: %+v", related.Edges)
		}
	})

	t.Run("a confidence number is never confirmation", func(t *testing.T) {
		f := startLibraryFixture(t)
		mangaWork := seedWork(t, f.db, store.TitleInput{Text: "Manga C", Kind: domain.TitleOriginal, Provenance: "manual"})
		mangaEd := seedEdition(t, f.db, mangaWork, domain.KindManga)
		animeWork := seedWork(t, f.db, store.TitleInput{Text: "Anime C", Kind: domain.TitleOriginal, Provenance: "manual"})
		animeEd := seedEdition(t, f.db, animeWork, domain.KindAnime)

		// A confidence of 1.0 still creates a proposed edge.
		res := callTool(t, f, ToolWriteRelation, map[string]any{
			"request_id": "req-conf", "actor": testActor, "action": "propose",
			"scope": "edition", "type": "related",
			"from_id": mangaEd, "to_id": animeEd,
			"provenance": "hermes-agent", "confidence": 1.0,
			"expected_revision": editionRev(t, f, mangaEd),
		})
		if res.IsError {
			t.Fatalf("propose failed: %s", resultText(t, res))
		}
		var v app.RelationView
		if err := json.Unmarshal([]byte(resultText(t, res)), &v); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if len(v.Proposed) != 1 || len(v.Confirmed) != 0 {
			t.Fatalf("confidence 1.0 must not confirm: %+v", v)
		}
	})

	t.Run("confirm without the intent guard fails", func(t *testing.T) {
		f := startLibraryFixture(t)
		mangaWork := seedWork(t, f.db, store.TitleInput{Text: "Manga G", Kind: domain.TitleOriginal, Provenance: "manual"})
		mangaEd := seedEdition(t, f.db, mangaWork, domain.KindManga)
		animeWork := seedWork(t, f.db, store.TitleInput{Text: "Anime G", Kind: domain.TitleOriginal, Provenance: "manual"})
		animeEd := seedEdition(t, f.db, animeWork, domain.KindAnime)
		res := callTool(t, f, ToolWriteRelation, map[string]any{
			"request_id": "req-g-propose", "actor": testActor, "action": "propose",
			"scope": "edition", "type": "related",
			"from_id": mangaEd, "to_id": animeEd,
			"provenance": "hermes-agent", "expected_revision": editionRev(t, f, mangaEd),
		})
		if res.IsError {
			t.Fatalf("propose failed: %s", resultText(t, res))
		}
		var v app.RelationView
		if err := json.Unmarshal([]byte(resultText(t, res)), &v); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		relationID := v.Proposed[0].ID

		// confirm without confirmation true is an invalid_argument.
		res = callTool(t, f, ToolWriteRelation, map[string]any{
			"request_id": "req-g-noflag", "actor": testActor, "action": "confirm",
			"relation_id": relationID, "user_instruction": "user said confirm",
			"expected_revision": editionRev(t, f, mangaEd),
		})
		if code := failureCode(t, res); code != "invalid_argument" {
			t.Fatalf("confirm without the flag failure code = %q, want invalid_argument", code)
		}
		// confirmation true without the user-instruction context fails.
		res = callTool(t, f, ToolWriteRelation, map[string]any{
			"request_id": "req-g-noinstr", "actor": testActor, "action": "confirm",
			"relation_id": relationID, "confirmation": true,
			"expected_revision": editionRev(t, f, mangaEd),
		})
		if code := failureCode(t, res); code != "invalid_argument" {
			t.Fatalf("confirm without the instruction failure code = %q, want invalid_argument", code)
		}
		// The edge is still proposed: the prior state is intact.
		res = callTool(t, f, ToolListRelated, map[string]any{"scope": "edition", "entity_id": mangaEd})
		if res.IsError {
			t.Fatalf("list_related failed: %s", resultText(t, res))
		}
		var related app.RelatedView
		if err := json.Unmarshal([]byte(resultText(t, res)), &related); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if len(related.Edges) != 1 || related.Edges[0].State != domain.ClaimProposed {
			t.Fatalf("a failed confirm must leave the edge proposed: %+v", related.Edges)
		}
	})

	t.Run("a source cycle is rejected with no partial mutation", func(t *testing.T) {
		f := startLibraryFixture(t)
		a := seedWork(t, f.db, store.TitleInput{Text: "Work A", Kind: domain.TitleOriginal, Provenance: "manual"})
		b := seedWork(t, f.db, store.TitleInput{Text: "Work B", Kind: domain.TitleOriginal, Provenance: "manual"})
		c := seedWork(t, f.db, store.TitleInput{Text: "Work C", Kind: domain.TitleOriginal, Provenance: "manual"})

		proposeConfirm := func(t *testing.T, from, to, reqID string) string {
			t.Helper()
			res := callTool(t, f, ToolWriteRelation, map[string]any{
				"request_id": reqID + "-propose", "actor": testActor, "action": "propose",
				"scope": "work", "type": "sequel_of",
				"from_id": from, "to_id": to, "provenance": "hermes-agent",
				"expected_revision": workRev(t, f, from),
			})
			if res.IsError {
				t.Fatalf("propose %s->%s failed: %s", from, to, resultText(t, res))
			}
			var v app.RelationView
			if err := json.Unmarshal([]byte(resultText(t, res)), &v); err != nil {
				t.Fatalf("unmarshal view: %v", err)
			}
			res = callTool(t, f, ToolWriteRelation, map[string]any{
				"request_id": reqID + "-confirm", "actor": testActor, "action": "confirm",
				"relation_id":  v.Proposed[0].ID,
				"confirmation": true, "user_instruction": "user confirmed",
				"expected_revision": workRev(t, f, from),
			})
			if res.IsError {
				t.Fatalf("confirm %s->%s failed: %s", from, to, resultText(t, res))
			}
			return v.Proposed[0].ID
		}

		proposeConfirm(t, a, b, "req-ab")
		proposeConfirm(t, b, c, "req-bc")

		// C is a sequel of A would loop the confirmed sequel graph.
		res := callTool(t, f, ToolWriteRelation, map[string]any{
			"request_id": "req-ca-propose", "actor": testActor, "action": "propose",
			"scope": "work", "type": "sequel_of",
			"from_id": c, "to_id": a, "provenance": "hermes-agent",
			"expected_revision": workRev(t, f, c),
		})
		if res.IsError {
			t.Fatalf("propose c->a failed: %s", resultText(t, res))
		}
		var v app.RelationView
		if err := json.Unmarshal([]byte(resultText(t, res)), &v); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		res = callTool(t, f, ToolWriteRelation, map[string]any{
			"request_id": "req-ca-confirm", "actor": testActor, "action": "confirm",
			"relation_id":  v.Proposed[0].ID,
			"confirmation": true, "user_instruction": "user confirmed",
			"expected_revision": workRev(t, f, c),
		})
		if code := failureCode(t, res); code != "cycle_detected" {
			t.Fatalf("cyclic confirm failure code = %q, want cycle_detected", code)
		}

		// No partial mutation: a->b and b->c are confirmed, c->a is still
		// proposed (the cycle guard ran in the confirm transaction).
		res = callTool(t, f, ToolListRelated, map[string]any{"scope": "work", "entity_id": a, "depth": 3})
		if res.IsError {
			t.Fatalf("list_related failed: %s", resultText(t, res))
		}
		var related app.RelatedView
		if err := json.Unmarshal([]byte(resultText(t, res)), &related); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		states := []string{}
		for _, e := range related.Edges {
			states = append(states, string(e.State))
		}
		sort.Strings(states)
		// Two confirmed edges (a->b, b->c) and the still-proposed c->a.
		if len(states) != 3 || states[0] != "confirmed" || states[1] != "confirmed" || states[2] != "proposed" {
			t.Fatalf("the cycle must leave no partial mutation: %+v", states)
		}
	})

	t.Run("revoke keeps history", func(t *testing.T) {
		f := startLibraryFixture(t)
		mangaWork := seedWork(t, f.db, store.TitleInput{Text: "Manga R", Kind: domain.TitleOriginal, Provenance: "manual"})
		mangaEd := seedEdition(t, f.db, mangaWork, domain.KindManga)
		animeWork := seedWork(t, f.db, store.TitleInput{Text: "Anime R", Kind: domain.TitleOriginal, Provenance: "manual"})
		animeEd := seedEdition(t, f.db, animeWork, domain.KindAnime)
		// A directed type keeps the from entity as given, so the
		// revision guard is on mangaEd (a symmetric type would canonical
		// the endpoints).
		res := callTool(t, f, ToolWriteRelation, map[string]any{
			"request_id": "req-r-propose", "actor": testActor, "action": "propose",
			"scope": "edition", "type": "adaptation_of",
			"from_id": mangaEd, "to_id": animeEd, "provenance": "hermes-agent",
			"expected_revision": editionRev(t, f, mangaEd),
		})
		if res.IsError {
			t.Fatalf("propose failed: %s", resultText(t, res))
		}
		var v app.RelationView
		if err := json.Unmarshal([]byte(resultText(t, res)), &v); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		relationID := v.Proposed[0].ID
		res = callTool(t, f, ToolWriteRelation, map[string]any{
			"request_id": "req-r-revoke", "actor": testActor, "action": "revoke",
			"relation_id":       relationID,
			"expected_revision": editionRev(t, f, mangaEd),
		})
		if res.IsError {
			t.Fatalf("revoke failed: %s", resultText(t, res))
		}
		var revoked app.RelationView
		if err := json.Unmarshal([]byte(resultText(t, res)), &revoked); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if len(revoked.Revoked) != 1 || len(revoked.Proposed) != 0 {
			t.Fatalf("the edge must be revoked and stay in history: %+v", revoked)
		}
	})
}

func TestMCPOrigin(t *testing.T) {
	testActor := map[string]any{"principal": "hermes-agent", "origin": "agent"}

	t.Run("propose, confirm and revoke an origin claim", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Origin Work", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, work, domain.KindManga)

		res := callTool(t, f, ToolWriteOrigin, map[string]any{
			"request_id": "req-o-propose", "actor": testActor, "action": "propose",
			"edition_id": ed, "kind": "anime_original", "provenance": "hermes-agent",
			"expected_revision": editionRev(t, f, ed),
		})
		if res.IsError {
			t.Fatalf("propose failed: %s", resultText(t, res))
		}
		var v app.OriginView
		if err := json.Unmarshal([]byte(resultText(t, res)), &v); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if len(v.Claims) != 1 || v.Claims[0].State != domain.ClaimProposed {
			t.Fatalf("the claim must be proposed: %+v", v.Claims)
		}
		claimID := v.Claims[0].ID

		res = callTool(t, f, ToolWriteOrigin, map[string]any{
			"request_id": "req-o-confirm", "actor": testActor, "action": "confirm",
			"edition_id": ed, "claim_id": claimID,
			"confirmation": true, "user_instruction": "user confirmed the origin",
			"expected_revision": editionRev(t, f, ed),
		})
		if res.IsError {
			t.Fatalf("confirm failed: %s", resultText(t, res))
		}
		var confirmed app.OriginView
		if err := json.Unmarshal([]byte(resultText(t, res)), &confirmed); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if len(confirmed.Claims) != 1 || confirmed.Claims[0].State != domain.ClaimConfirmed {
			t.Fatalf("the claim must be confirmed: %+v", confirmed.Claims)
		}

		res = callTool(t, f, ToolWriteOrigin, map[string]any{
			"request_id": "req-o-revoke", "actor": testActor, "action": "revoke",
			"edition_id": ed, "claim_id": claimID,
			"expected_revision": editionRev(t, f, ed),
		})
		if res.IsError {
			t.Fatalf("revoke failed: %s", resultText(t, res))
		}
		var revoked app.OriginView
		if err := json.Unmarshal([]byte(resultText(t, res)), &revoked); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if len(revoked.Claims) != 1 || revoked.Claims[0].State != domain.ClaimRevoked {
			t.Fatalf("the claim must be revoked and stay in history: %+v", revoked.Claims)
		}
	})

	t.Run("confirm requires the intent guard", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Origin Guard", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, work, domain.KindManga)
		res := callTool(t, f, ToolWriteOrigin, map[string]any{
			"request_id": "req-og-propose", "actor": testActor, "action": "propose",
			"edition_id": ed, "kind": "original", "provenance": "hermes-agent",
			"expected_revision": editionRev(t, f, ed),
		})
		if res.IsError {
			t.Fatalf("propose failed: %s", resultText(t, res))
		}
		var v app.OriginView
		if err := json.Unmarshal([]byte(resultText(t, res)), &v); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		claimID := v.Claims[0].ID
		res = callTool(t, f, ToolWriteOrigin, map[string]any{
			"request_id": "req-og-confirm", "actor": testActor, "action": "confirm",
			"edition_id": ed, "claim_id": claimID,
			"expected_revision": editionRev(t, f, ed),
		})
		if code := failureCode(t, res); code != "invalid_argument" {
			t.Fatalf("origin confirm without the guard failure code = %q, want invalid_argument", code)
		}
	})
}

func TestMCPActivity(t *testing.T) {
	testActor := map[string]any{"principal": "hermes-agent", "origin": "agent"}

	t.Run("the bounded feed lists observations with type filters", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Activity Work", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, work, domain.KindManga)

		res := callTool(t, f, ToolRecordProgress, map[string]any{
			"request_id": "req-act-p", "actor": testActor, "edition_id": ed,
			"unit": "chapter", "position": "5", "expected_revision": 0,
		})
		if res.IsError {
			t.Fatalf("record_progress failed: %s", resultText(t, res))
		}
		res = callTool(t, f, ToolSetConsumption, map[string]any{
			"request_id": "req-act-c", "actor": testActor, "edition_id": ed,
			"value": "in_progress", "expected_revision": 1,
		})
		if res.IsError {
			t.Fatalf("set_consumption failed: %s", resultText(t, res))
		}

		// The unfiltered feed has both observations, newest first.
		res = callTool(t, f, ToolListActivity, map[string]any{})
		if res.IsError {
			t.Fatalf("list_activity failed: %s", resultText(t, res))
		}
		var list app.ListPayload
		if err := json.Unmarshal([]byte(resultText(t, res)), &list); err != nil {
			t.Fatalf("unmarshal list: %v", err)
		}
		types := []string{}
		for _, item := range list.Items {
			b, _ := json.Marshal(item)
			var it struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(b, &it); err != nil {
				t.Fatalf("unmarshal item: %v", err)
			}
			types = append(types, it.Type)
		}
		if len(types) != 2 || types[0] != "consumption" || types[1] != "progress" {
			t.Fatalf("expected the two observations newest first: %+v", types)
		}

		// A type filter narrows the feed.
		res = callTool(t, f, ToolListActivity, map[string]any{"types": []any{"progress"}})
		if res.IsError {
			t.Fatalf("list_activity failed: %s", resultText(t, res))
		}
		list = app.ListPayload{}
		if err := json.Unmarshal([]byte(resultText(t, res)), &list); err != nil {
			t.Fatalf("unmarshal list: %v", err)
		}
		if len(list.Items) != 1 {
			t.Fatalf("the progress filter must return one row: %+v", list.Items)
		}
	})

	t.Run("an invalid type or limit fails", func(t *testing.T) {
		f := startLibraryFixture(t)
		// An unknown type is rejected by the schema.
		res := callTool(t, f, ToolListActivity, map[string]any{"types": []any{"bogus"}})
		if !res.IsError {
			t.Fatalf("an unknown type must fail: %s", resultText(t, res))
		}
		// An oversized limit is rejected by the schema bound.
		res = callTool(t, f, ToolListActivity, map[string]any{"limit": app.MaxActivityLimit + 1})
		if !res.IsError {
			t.Fatalf("an oversized limit must fail: %s", resultText(t, res))
		}
	})
}
