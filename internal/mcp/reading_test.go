// MCP request tests for the reading tools: progress observations are
// edition/unit scoped and survive as history (the current value is the
// shared service's, the same view HTTP serves), a reading statement never
// means completed, the idempotency key replays an exact retry without
// duplicating history, a stale revision is a conflict with the prior state
// intact, a correction supersedes the named row which stays in history,
// and invalid shapes fail with stable codes. The client talks to the
// server through the SDK in-memory transport against a real temporary
// database.
package mcp

import (
	"encoding/json"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func TestMCPReading(t *testing.T) {
	testActor := map[string]any{"principal": "hermes-agent", "origin": "agent"}

	t.Run("progress observations survive and the current value matches the shared service", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Reading Target", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, work, domain.KindManga)

		// Chapter 42.
		res := callTool(t, f, ToolRecordProgress, map[string]any{
			"request_id": "req-p42", "actor": testActor, "edition_id": ed,
			"unit": "chapter", "position": "42", "expected_revision": 0,
		})
		if res.IsError {
			t.Fatalf("record 42 failed: %s", resultText(t, res))
		}
		var v42 app.EditionView
		if err := json.Unmarshal([]byte(resultText(t, res)), &v42); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if v42.Progress.Chapter == nil || v42.Progress.Chapter.Position != "42" || v42.Revision != 1 {
			t.Fatalf("expected chapter 42 at revision 1: %+v", v42)
		}

		// Chapter 48.
		res = callTool(t, f, ToolRecordProgress, map[string]any{
			"request_id": "req-p48", "actor": testActor, "edition_id": ed,
			"unit": "chapter", "position": "48", "expected_revision": 1,
		})
		if res.IsError {
			t.Fatalf("record 48 failed: %s", resultText(t, res))
		}
		var v48 app.EditionView
		if err := json.Unmarshal([]byte(resultText(t, res)), &v48); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if v48.Progress.Chapter == nil || v48.Progress.Chapter.Position != "48" || v48.Revision != 2 {
			t.Fatalf("expected chapter 48 at revision 2: %+v", v48)
		}

		// A restart to a lower position is allowed for rereads.
		res = callTool(t, f, ToolRecordProgress, map[string]any{
			"request_id": "req-p1", "actor": testActor, "edition_id": ed,
			"unit": "chapter", "position": "1", "expected_revision": 2,
		})
		if res.IsError {
			t.Fatalf("restart record failed: %s", resultText(t, res))
		}
		restartText := resultText(t, res)
		var v1 app.EditionView
		if err := json.Unmarshal([]byte(restartText), &v1); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if v1.Progress.Chapter == nil || v1.Progress.Chapter.Position != "1" {
			t.Fatalf("expected the reread position 1: %+v", v1)
		}

		// Both progress observations survive in the history: the three
		// rows, all effective and visible, the current value the latest.
		res = callTool(t, f, ToolGetHistory, map[string]any{"edition_id": ed})
		if res.IsError {
			t.Fatalf("get_history failed: %s", resultText(t, res))
		}
		var list app.ListPayload
		if err := json.Unmarshal([]byte(resultText(t, res)), &list); err != nil {
			t.Fatalf("unmarshal list: %v", err)
		}
		var progress []struct {
			Detail string `json:"detail"`
			Label  string `json:"label"`
		}
		for _, item := range list.Items {
			b, _ := json.Marshal(item)
			var it struct {
				Type   string `json:"type"`
				Detail string `json:"detail"`
				Label  string `json:"label"`
			}
			if err := json.Unmarshal(b, &it); err != nil {
				t.Fatalf("unmarshal item: %v", err)
			}
			if it.Type == "progress" {
				progress = append(progress, struct {
					Detail string `json:"detail"`
					Label  string `json:"label"`
				}{it.Detail, it.Label})
			}
		}
		if len(progress) != 3 {
			t.Fatalf("expected the three progress observations to survive, got %+v", progress)
		}
		byValue := map[string]string{}
		for _, p := range progress {
			byValue[p.Detail] = p.Label
		}
		// Plain records never supersede one another: every observation
		// stays effective and visible, the current value is simply the
		// latest.
		if byValue["42/"] != "effective" || byValue["48/"] != "effective" || byValue["1/"] != "effective" {
			t.Fatalf("unexpected history labels: %+v", byValue)
		}

		// The current value matches the shared service view exactly:
		// the same payload HTTP serves.
		direct, err := f.service.GetEdition(f.ctx, ed)
		if err != nil {
			t.Fatalf("GetEdition: %v", err)
		}
		directJSON, err := json.Marshal(direct)
		if err != nil {
			t.Fatalf("marshal direct: %v", err)
		}
		sameJSON(t, restartText, string(directJSON))
	})

	t.Run("a reading statement never implicitly means completed", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Never Completed", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, work, domain.KindManga)

		res := callTool(t, f, ToolRecordProgress, map[string]any{
			"request_id": "req-read-only", "actor": testActor, "edition_id": ed,
			"unit": "chapter", "position": "10", "expected_revision": 0,
		})
		if res.IsError {
			t.Fatalf("record failed: %s", resultText(t, res))
		}
		var v app.EditionView
		if err := json.Unmarshal([]byte(resultText(t, res)), &v); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if v.Consumption != nil {
			t.Fatalf("progress must not invent a consumption state: %+v", v.Consumption)
		}

		// An explicit in_progress state survives a later reading
		// unchanged, never upgraded to completed.
		res = callTool(t, f, ToolSetConsumption, map[string]any{
			"request_id": "req-in-progress", "actor": testActor, "edition_id": ed,
			"value": "in_progress", "expected_revision": 1,
		})
		if res.IsError {
			t.Fatalf("set_consumption failed: %s", resultText(t, res))
		}
		res = callTool(t, f, ToolRecordProgress, map[string]any{
			"request_id": "req-read-again", "actor": testActor, "edition_id": ed,
			"unit": "chapter", "position": "11", "expected_revision": 2,
		})
		if res.IsError {
			t.Fatalf("record failed: %s", resultText(t, res))
		}
		var after app.EditionView
		if err := json.Unmarshal([]byte(resultText(t, res)), &after); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if after.Consumption == nil || after.Consumption.Value != domain.ConsumptionInProgress {
			t.Fatalf("a reading must not change the consumption state: %+v", after.Consumption)
		}
	})

	t.Run("a retry returns the identical result and history is not duplicated", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Retry Target", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, work, domain.KindManga)
		payload := map[string]any{
			"request_id": "req-retry", "actor": testActor, "edition_id": ed,
			"unit": "chapter", "position": "5", "expected_revision": 0,
		}
		first := callTool(t, f, ToolRecordProgress, payload)
		if first.IsError {
			t.Fatalf("record failed: %s", resultText(t, first))
		}
		second := callTool(t, f, ToolRecordProgress, payload)
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
		res := callTool(t, f, ToolGetHistory, map[string]any{"edition_id": ed})
		if res.IsError {
			t.Fatalf("get_history failed: %s", resultText(t, res))
		}
		var list app.ListPayload
		if err := json.Unmarshal([]byte(resultText(t, res)), &list); err != nil {
			t.Fatalf("unmarshal list: %v", err)
		}
		n := 0
		for range list.Items {
			n++
		}
		if n != 1 {
			t.Fatalf("the retry must not duplicate history: %d rows", n)
		}
	})

	t.Run("a stale revision is a conflict and the prior state stays intact", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Stale Target", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, work, domain.KindManga)
		res := callTool(t, f, ToolRecordProgress, map[string]any{
			"request_id": "req-base", "actor": testActor, "edition_id": ed,
			"unit": "chapter", "position": "10", "expected_revision": 0,
		})
		if res.IsError {
			t.Fatalf("record failed: %s", resultText(t, res))
		}
		res = callTool(t, f, ToolRecordProgress, map[string]any{
			"request_id": "req-stale", "actor": testActor, "edition_id": ed,
			"unit": "chapter", "position": "20", "expected_revision": 0,
		})
		if code := failureCode(t, res); code != "conflict" {
			t.Fatalf("stale revision failure code = %q, want conflict", code)
		}
		view, err := f.service.GetEdition(f.ctx, ed)
		if err != nil {
			t.Fatalf("GetEdition: %v", err)
		}
		if view.Progress.Chapter == nil || view.Progress.Chapter.Position != "10" || view.Revision != 1 {
			t.Fatalf("the prior state must stay intact: %+v (revision %d)", view.Progress, view.Revision)
		}
	})

	t.Run("a correction supersedes the named row and the retraction keeps history", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Correction Target", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, work, domain.KindManga)

		// Record chapter 42, take the record id.
		res := callTool(t, f, ToolRecordProgress, map[string]any{
			"request_id": "req-c42", "actor": testActor, "edition_id": ed,
			"unit": "chapter", "position": "42", "expected_revision": 0,
		})
		if res.IsError {
			t.Fatalf("record failed: %s", resultText(t, res))
		}
		var v app.EditionView
		if err := json.Unmarshal([]byte(resultText(t, res)), &v); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		recordID := v.Progress.Chapter.ID

		// A correction supersedes the named row; the row stays in
		// history.
		res = callTool(t, f, ToolRecordProgress, map[string]any{
			"request_id": "req-c43", "actor": testActor, "edition_id": ed,
			"unit": "chapter", "position": "43", "expected_revision": 1,
			"supersedes_id": recordID,
		})
		if res.IsError {
			t.Fatalf("correction failed: %s", resultText(t, res))
		}
		var after app.EditionView
		if err := json.Unmarshal([]byte(resultText(t, res)), &after); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if after.Progress.Chapter == nil || after.Progress.Chapter.Position != "43" {
			t.Fatalf("the correction must set the current position: %+v", after.Progress)
		}

		// The typed retraction removes the current position; the row
		// stays in history retracted.
		res = callTool(t, f, ToolRetractProgress, map[string]any{
			"request_id": "req-retract", "actor": testActor, "edition_id": ed,
			"record_id": after.Progress.Chapter.ID, "reason": "wrong chapter",
			"expected_revision": 2,
		})
		if res.IsError {
			t.Fatalf("retraction failed: %s", resultText(t, res))
		}
		var retracted app.EditionView
		if err := json.Unmarshal([]byte(resultText(t, res)), &retracted); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if retracted.Progress.Chapter != nil {
			t.Fatalf("the retraction must clear the current position: %+v", retracted.Progress)
		}

		res = callTool(t, f, ToolGetHistory, map[string]any{"edition_id": ed})
		if res.IsError {
			t.Fatalf("get_history failed: %s", resultText(t, res))
		}
		var list app.ListPayload
		if err := json.Unmarshal([]byte(resultText(t, res)), &list); err != nil {
			t.Fatalf("unmarshal list: %v", err)
		}
		labels := map[string]string{}
		for _, item := range list.Items {
			b, _ := json.Marshal(item)
			var it struct {
				Type   string  `json:"type"`
				Detail string  `json:"detail"`
				Label  string  `json:"label"`
				Reason *string `json:"reason"`
			}
			if err := json.Unmarshal(b, &it); err != nil {
				t.Fatalf("unmarshal item: %v", err)
			}
			if it.Type == "progress" {
				labels[it.Detail] = it.Label
			}
		}
		// 42 is superseded by the correction; 43 is superseded by the
		// retraction, which is its own row with a null position (the
		// empty detail) labelled retracted. Both corrected rows stay in
		// history, never deleted.
		if labels["42/"] != "superseded" || labels["43/"] != "superseded" || labels[""] != "retracted" {
			t.Fatalf("the corrected and retracted rows must stay in history: %+v", labels)
		}
	})

	t.Run("invalid shapes fail with stable codes", func(t *testing.T) {
		f := startLibraryFixture(t)
		work := seedWork(t, f.db, store.TitleInput{Text: "Invalid Target", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, work, domain.KindManga)

		// An unknown unit is rejected by the schema.
		res := callTool(t, f, ToolRecordProgress, map[string]any{
			"request_id": "req-bad-unit", "actor": testActor, "edition_id": ed,
			"unit": "sentence", "position": "1", "expected_revision": 0,
		})
		if !res.IsError {
			t.Fatalf("an unknown unit must fail: %s", resultText(t, res))
		}
		// A non-decimal position is an invalid_argument from the service.
		res = callTool(t, f, ToolRecordProgress, map[string]any{
			"request_id": "req-bad-position", "actor": testActor, "edition_id": ed,
			"unit": "chapter", "position": "abc", "expected_revision": 0,
		})
		if code := failureCode(t, res); code != "invalid_argument" {
			t.Fatalf("bad position failure code = %q, want invalid_argument", code)
		}
		// A consumption value outside the bounded set is rejected by the
		// schema.
		res = callTool(t, f, ToolSetConsumption, map[string]any{
			"request_id": "req-bad-value", "actor": testActor, "edition_id": ed,
			"value": "watching", "expected_revision": 0,
		})
		if !res.IsError {
			t.Fatalf("an unknown consumption value must fail: %s", resultText(t, res))
		}
		// A retraction without a reason is rejected by the schema.
		res = callTool(t, f, ToolRetractProgress, map[string]any{
			"request_id": "req-no-reason", "actor": testActor, "edition_id": ed,
			"record_id": "some-record", "expected_revision": 0,
		})
		if !res.IsError {
			t.Fatalf("a reasonless retraction must fail: %s", resultText(t, res))
		}
		// get_history requires exactly one target.
		res = callTool(t, f, ToolGetHistory, map[string]any{"work_id": work, "edition_id": ed})
		if code := failureCode(t, res); code != "invalid_argument" {
			t.Fatalf("two targets failure code = %q, want invalid_argument", code)
		}
		res = callTool(t, f, ToolGetHistory, map[string]any{})
		if code := failureCode(t, res); code != "invalid_argument" {
			t.Fatalf("no target failure code = %q, want invalid_argument", code)
		}
		// A percent total is an invalid_argument from the service.
		res = callTool(t, f, ToolRecordProgress, map[string]any{
			"request_id": "req-percent-total", "actor": testActor, "edition_id": ed,
			"unit": "percent", "position": "50", "total": "100", "expected_revision": 0,
		})
		if code := failureCode(t, res); code != "invalid_argument" {
			t.Fatalf("percent total failure code = %q, want invalid_argument", code)
		}
	})
}
