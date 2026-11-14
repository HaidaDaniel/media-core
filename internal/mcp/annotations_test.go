// MCP request tests for the personal annotation tools: the "save manga
// for later" example (intention, tag, note, rating and a sourced
// reference), a stale revision conflict with the prior state intact, an
// exact retry that replays without duplicating, invalid shapes with
// stable codes, and correction/retraction paths that keep history. The
// client talks to the server through the SDK in-memory transport against
// a real temporary database.
package mcp

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func TestMCPAnnotation(t *testing.T) {
	testActor := map[string]any{"principal": "hermes-agent", "origin": "agent"}

	t.Run("save manga for later: intention, tag, note, rating and source", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Manga Source", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, work, domain.KindManga)

		// Save for later: the intention observation.
		res := callTool(t, f, ToolSetIntention, map[string]any{
			"request_id": "req-int", "actor": testActor, "edition_id": ed,
			"value": "saved", "expected_revision": 0,
		})
		if res.IsError {
			t.Fatalf("set_intention failed: %s", resultText(t, res))
		}
		var v app.EditionView
		if err := json.Unmarshal([]byte(resultText(t, res)), &v); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if v.Intention == nil || v.Intention.Value != domain.IntentionSaved {
			t.Fatalf("expected intention saved: %+v", v.Intention)
		}

		// An edition tag.
		res = callTool(t, f, ToolSetTag, map[string]any{
			"request_id": "req-tag", "actor": testActor,
			"target_scope": "edition", "target_id": ed, "tag": "fantasy",
			"expected_revision": v.Revision,
		})
		if res.IsError {
			t.Fatalf("set_tag failed: %s", resultText(t, res))
		}
		var tv app.EditionView
		if err := json.Unmarshal([]byte(resultText(t, res)), &tv); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if !containsString(tv.Tags, "fantasy") {
			t.Fatalf("expected the tag to be attached: %+v", tv.Tags)
		}

		// An edition note.
		res = callTool(t, f, ToolWriteNote, map[string]any{
			"request_id": "req-note", "actor": testActor, "action": "add",
			"target_scope": "edition", "target_id": ed,
			"text":              "The art direction in volume two is outstanding.",
			"expected_revision": tv.Revision,
		})
		if res.IsError {
			t.Fatalf("write_note add failed: %s", resultText(t, res))
		}
		var nv app.EditionView
		if err := json.Unmarshal([]byte(resultText(t, res)), &nv); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if len(nv.Notes) != 1 || nv.Notes[0].Text != "The art direction in volume two is outstanding." {
			t.Fatalf("expected the note to be recorded: %+v", nv.Notes)
		}
		noteID := nv.Notes[0].ID

		// A 1-10 rating.
		res = callTool(t, f, ToolRecordRating, map[string]any{
			"request_id": "req-rating", "actor": testActor, "edition_id": ed,
			"value": 9, "expected_revision": nv.Revision,
		})
		if res.IsError {
			t.Fatalf("record_rating failed: %s", resultText(t, res))
		}
		var rv app.EditionView
		if err := json.Unmarshal([]byte(resultText(t, res)), &rv); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if rv.Rating == nil || rv.Rating.Value == nil || *rv.Rating.Value != 9 {
			t.Fatalf("expected the rating 9: %+v", rv.Rating)
		}

		// A sourced external reference.
		res = callTool(t, f, ToolWriteReference, map[string]any{
			"request_id": "req-ref", "actor": testActor, "action": "add",
			"edition_id": ed, "provider": "mangadex", "external_id": "src-1",
			"role": "publication", "provenance": "manual",
			"expected_revision": rv.Revision,
		})
		if res.IsError {
			t.Fatalf("write_reference add failed: %s", resultText(t, res))
		}
		var refv app.EditionView
		if err := json.Unmarshal([]byte(resultText(t, res)), &refv); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if len(refv.References) != 1 || refv.References[0].Provider != "mangadex" {
			t.Fatalf("expected the reference to be attached: %+v", refv.References)
		}

		// The note revision stays reachable: the note id is stable.
		if noteID == "" {
			t.Fatalf("the note id must be stable for later revisions")
		}
	})

	t.Run("a stale revision is a conflict and the prior state stays intact", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Stale Annotation", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, work, domain.KindManga)
		res := callTool(t, f, ToolRecordRating, map[string]any{
			"request_id": "req-base", "actor": testActor, "edition_id": ed,
			"value": 7, "expected_revision": 0,
		})
		if res.IsError {
			t.Fatalf("record failed: %s", resultText(t, res))
		}
		res = callTool(t, f, ToolRecordRating, map[string]any{
			"request_id": "req-stale", "actor": testActor, "edition_id": ed,
			"value": 8, "expected_revision": 0,
		})
		if code := failureCode(t, res); code != "conflict" {
			t.Fatalf("stale revision failure code = %q, want conflict", code)
		}
		view, err := f.service.GetEdition(f.ctx, ed)
		if err != nil {
			t.Fatalf("GetEdition: %v", err)
		}
		if view.Rating == nil || view.Rating.Value == nil || *view.Rating.Value != 7 || view.Revision != 1 {
			t.Fatalf("the prior state must stay intact: %+v (revision %d)", view.Rating, view.Revision)
		}
	})

	t.Run("a retry returns the identical result without duplicating", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Retry Annotation", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, work, domain.KindManga)
		payload := map[string]any{
			"request_id": "req-retry", "actor": testActor, "edition_id": ed,
			"value": "planned", "expected_revision": 0,
		}
		first := callTool(t, f, ToolSetIntention, payload)
		if first.IsError {
			t.Fatalf("set_intention failed: %s", resultText(t, first))
		}
		second := callTool(t, f, ToolSetIntention, payload)
		if second.IsError {
			t.Fatalf("retry failed: %s", resultText(t, second))
		}
		sameJSON(t, resultText(t, first), resultText(t, second))
		view, err := f.service.GetEdition(f.ctx, ed)
		if err != nil {
			t.Fatalf("GetEdition: %v", err)
		}
		if view.Revision != 1 {
			t.Fatalf("the retry must not advance the revision: %d", view.Revision)
		}
	})

	t.Run("a correction and a retraction keep history", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Correction Annotation", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, work, domain.KindManga)

		// Record a rating, then correct it with a supersedes_id.
		res := callTool(t, f, ToolRecordRating, map[string]any{
			"request_id": "req-r7", "actor": testActor, "edition_id": ed,
			"value": 7, "expected_revision": 0,
		})
		if res.IsError {
			t.Fatalf("record failed: %s", resultText(t, res))
		}
		// The edition view does not carry the record id, so the
		// supersedes_id for the correction comes from the history.
		res = callTool(t, f, ToolGetHistory, map[string]any{"edition_id": ed})
		if res.IsError {
			t.Fatalf("get_history failed: %s", resultText(t, res))
		}
		var list app.ListPayload
		if err := json.Unmarshal([]byte(resultText(t, res)), &list); err != nil {
			t.Fatalf("unmarshal list: %v", err)
		}
		var rating7ID string
		for _, item := range list.Items {
			b, _ := json.Marshal(item)
			var it struct {
				ID    string  `json:"id"`
				Type  string  `json:"type"`
				Value *string `json:"value"`
			}
			if err := json.Unmarshal(b, &it); err != nil {
				t.Fatalf("unmarshal item: %v", err)
			}
			if it.Type == "rating" && it.Value != nil && *it.Value == "7" {
				rating7ID = it.ID
			}
		}
		if rating7ID == "" {
			t.Fatalf("the rating 7 row must be in the history")
		}
		view, err := f.service.GetEdition(f.ctx, ed)
		if err != nil {
			t.Fatalf("GetEdition: %v", err)
		}
		res = callTool(t, f, ToolRecordRating, map[string]any{
			"request_id": "req-r8", "actor": testActor, "edition_id": ed,
			"value": 8, "expected_revision": view.Revision,
			"supersedes_id": rating7ID,
		})
		if res.IsError {
			t.Fatalf("correction failed: %s", resultText(t, res))
		}
		var after app.EditionView
		if err := json.Unmarshal([]byte(resultText(t, res)), &after); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if after.Rating == nil || after.Rating.Value == nil || *after.Rating.Value != 8 {
			t.Fatalf("the correction must set the current rating: %+v", after.Rating)
		}

		// A note, then a retraction; the row stays in history.
		res = callTool(t, f, ToolWriteNote, map[string]any{
			"request_id": "req-note-add", "actor": testActor, "action": "add",
			"target_scope": "edition", "target_id": ed,
			"text": "First draft of the note.", "expected_revision": after.Revision,
		})
		if res.IsError {
			t.Fatalf("note add failed: %s", resultText(t, res))
		}
		var nv app.EditionView
		if err := json.Unmarshal([]byte(resultText(t, res)), &nv); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		// Note revisions guard the note's own revision, not the target's.
		noteID := nv.Notes[0].ID
		noteRev := nv.Notes[0].Revision
		res = callTool(t, f, ToolWriteNote, map[string]any{
			"request_id": "req-note-retract", "actor": testActor, "action": "retract",
			"note_id": noteID, "reason": "wrong edition",
			"expected_revision": noteRev,
		})
		if res.IsError {
			t.Fatalf("note retract failed: %s", resultText(t, res))
		}

		// The history keeps the corrected rating and the retracted note.
		res = callTool(t, f, ToolGetHistory, map[string]any{"edition_id": ed})
		if res.IsError {
			t.Fatalf("get_history failed: %s", resultText(t, res))
		}
		var list2 app.ListPayload
		if err := json.Unmarshal([]byte(resultText(t, res)), &list2); err != nil {
			t.Fatalf("unmarshal list: %v", err)
		}
		labels := map[string]string{}
		var noteLabels []string
		for _, item := range list2.Items {
			b, _ := json.Marshal(item)
			var it struct {
				Type  string  `json:"type"`
				Value *string `json:"value"`
				Label string  `json:"label"`
			}
			if err := json.Unmarshal(b, &it); err != nil {
				t.Fatalf("unmarshal item: %v", err)
			}
			key := it.Type
			if it.Value != nil {
				key += " " + *it.Value
			}
			if it.Type == "note" {
				noteLabels = append(noteLabels, it.Label)
			}
			labels[key] = it.Label
		}
		if labels["rating 7"] != "superseded" || labels["rating 8"] != "effective" {
			t.Fatalf("the corrected rating rows must stay in history: %+v", labels)
		}
		// The retraction is its own revision row (retracted); the
		// original row stays in history superseded.
		sort.Strings(noteLabels)
		if len(noteLabels) != 2 || noteLabels[0] != "retracted" || noteLabels[1] != "superseded" {
			t.Fatalf("the retracted note must stay in history: %+v", noteLabels)
		}
	})

	t.Run("invalid shapes fail with stable codes", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Invalid Annotation", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, work, domain.KindManga)

		// A rating outside 1-10 is rejected by the schema.
		res := callTool(t, f, ToolRecordRating, map[string]any{
			"request_id": "req-r0", "actor": testActor, "edition_id": ed,
			"value": 0, "expected_revision": 0,
		})
		if !res.IsError {
			t.Fatalf("a rating of 0 must fail: %s", resultText(t, res))
		}
		res = callTool(t, f, ToolRecordRating, map[string]any{
			"request_id": "req-r11", "actor": testActor, "edition_id": ed,
			"value": 11, "expected_revision": 0,
		})
		if !res.IsError {
			t.Fatalf("a rating of 11 must fail: %s", resultText(t, res))
		}
		// An unknown intention value is rejected by the schema.
		res = callTool(t, f, ToolSetIntention, map[string]any{
			"request_id": "req-int-bad", "actor": testActor, "edition_id": ed,
			"value": "curious", "expected_revision": 0,
		})
		if !res.IsError {
			t.Fatalf("an unknown intention must fail: %s", resultText(t, res))
		}
		// A note add without text is an invalid_argument.
		res = callTool(t, f, ToolWriteNote, map[string]any{
			"request_id": "req-note-notext", "actor": testActor, "action": "add",
			"target_scope": "edition", "target_id": ed, "expected_revision": 0,
		})
		if code := failureCode(t, res); code != "invalid_argument" {
			t.Fatalf("note without text failure code = %q, want invalid_argument", code)
		}
		// A reference add without a role is an invalid_argument.
		res = callTool(t, f, ToolWriteReference, map[string]any{
			"request_id": "req-ref-norole", "actor": testActor, "action": "add",
			"edition_id": ed, "provider": "mangadex", "external_id": "x",
			"provenance": "manual", "expected_revision": 0,
		})
		if code := failureCode(t, res); code != "invalid_argument" {
			t.Fatalf("reference without role failure code = %q, want invalid_argument", code)
		}
	})
}
