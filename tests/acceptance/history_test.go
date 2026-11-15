// Scenario B/D acceptance: reading progress with restart, reread and
// supersedes_id correction (chapter 42 to 48, a lower reread, a corrected
// mistake, unknown total retained, invalid positions rejected, progress
// never implying completed) and the retained negative impression (in
// progress then dropped, a revised negative note whose old text survives,
// a 2/10 rating, later lookup by alias, dropped filter and activity,
// source links and history intact, an independent saved manga). Both
// scenarios compare the shared service's views against the MCP transport's
// over the same database so both interfaces observe the identical
// effective and superseded values.
package acceptance

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"reflect"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/mcp"
	"github.com/HaidaDaniel/media-core/internal/store"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var historyActor = app.Actor{Principal: "history-acceptance", Origin: domain.OriginUser}

type historyState struct {
	svc *app.Service
	db  *store.Database
	dir string
}

func openHistory(t *testing.T) *historyState {
	t.Helper()
	dir := t.TempDir()
	maint, err := store.OpenForMaintenance(dir)
	if err != nil {
		t.Fatalf("OpenForMaintenance: %v", err)
	}
	if _, err := store.RunMigrations(maint.DB()); err != nil {
		maint.Close()
		t.Fatalf("RunMigrations: %v", err)
	}
	if err := maint.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	h := &historyState{svc: app.NewService(db), db: db, dir: dir}
	t.Cleanup(func() { db.Close() })
	return h
}

// restart closes the shared handle and reopens it, simulating the process
// restart the scenarios require: the state must survive in the database.
func (h *historyState) restart(t *testing.T) {
	t.Helper()
	if err := h.db.Close(); err != nil {
		t.Fatalf("close before restart: %v", err)
	}
	db, err := store.Open(h.dir)
	if err != nil {
		t.Fatalf("reopen after restart: %v", err)
	}
	h.db = db
	h.svc = app.NewService(db)
	t.Cleanup(func() { db.Close() })
}

type mcpOutcome struct {
	ok   bool
	data json.RawMessage
	code string
	msg  string
}

type mcpSession struct {
	session *mcpsdk.ClientSession
}

func openMCP(t *testing.T, db *store.Database) *mcpSession {
	t.Helper()
	srv := mcp.New(app.NewService(db), slog.New(slog.NewTextHandler(io.Discard, nil)))
	srvT, cliT := mcpsdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, srvT) }()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "history-acceptance", Version: "0"}, nil)
	session, err := client.Connect(ctx, cliT, nil)
	if err != nil {
		cancel()
		t.Fatalf("connect mcp client: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return &mcpSession{session: session}
}

func (m *mcpSession) call(t *testing.T, tool string, args map[string]any) mcpOutcome {
	t.Helper()
	res, err := m.session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: protocol error: %v", tool, err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("call %s: expected one content item, got %d", tool, len(res.Content))
	}
	text, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("call %s: expected text content, got %T", tool, res.Content[0])
	}
	if res.IsError {
		var wire struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(text.Text), &wire); err != nil {
			t.Fatalf("call %s: the tool error is not the wire error object: %s", tool, text.Text)
		}
		return mcpOutcome{code: wire.Code, msg: wire.Message}
	}
	return mcpOutcome{ok: true, data: json.RawMessage(text.Text)}
}

func mcpActor() map[string]any {
	return map[string]any{"principal": "history-acceptance", "origin": string(domain.OriginUser)}
}

// historyKey is one normalized history row: type, value, label. The
// position detail is folded into value for progress rows so the comparison
// is over the user-visible effective/superseded values, not the ids or
// instants.
type historyKey struct {
	typ   string
	value string
	label string
}

func historyKeys(t *testing.T, payload app.ListPayload) []historyKey {
	t.Helper()
	out := make([]historyKey, 0, len(payload.Items))
	for _, item := range payload.Items {
		ai, ok := item.(app.ActivityItem)
		if !ok {
			t.Fatalf("unexpected history item type %T", item)
		}
		value := ""
		if ai.Value != nil {
			value = *ai.Value
		}
		if ai.Type == app.ActivityProgress && ai.Detail != nil {
			// detail is "position/total"; the unknown total renders as a
			// trailing slash, which the display strips.
			value = value + " " + *ai.Detail
		}
		out = append(out, historyKey{typ: string(ai.Type), value: value, label: ai.Label})
	}
	return out
}

// sortedHistoryKeys orders the keys so both interfaces' rows compare as a
// set: the ordering by (recorded_at, id) is stable across interfaces for
// the same database, but the contract is the multiset of values and labels.
func sortedHistoryKeys(t *testing.T, keys []historyKey) []historyKey {
	t.Helper()
	out := append([]historyKey(nil), keys...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && lessHistoryKey(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func lessHistoryKey(a, b historyKey) bool {
	if a.typ != b.typ {
		return a.typ < b.typ
	}
	if a.value != b.value {
		return a.value < b.value
	}
	return a.label < b.label
}

// TestScenarioB drives reading progress with restart, reread and
// supersedes_id correction through the shared service and compares the
// effective and superseded values against the MCP transport.
func TestScenarioB(t *testing.T) {
	ctx := context.Background()
	h := openHistory(t)

	created, err := h.svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase: app.MutationBase{RequestID: "b-create", Actor: historyActor},
		Titles:       []store.TitleInput{{Text: "Тихий Портал", Kind: domain.TitleOriginal, Language: strP("ru"), Preferred: true, Provenance: "manual"}},
		Kind:         strKind(domain.KindWebNovel),
	})
	if err != nil || !created.Created || created.EditionID == nil {
		t.Fatalf("create: created=%+v err=%v", created, err)
	}
	editionID := *created.EditionID

	// Chapter 42 with the total left unknown.
	first, err := h.svc.RecordProgress(ctx, app.RecordProgressRequest{
		MutationBase: app.MutationBase{RequestID: "b-ch42", Actor: historyActor, ExpectedRevision: i64P(0)},
		EditionID:    editionID,
		Unit:         domain.UnitChapter,
		Position:     "42",
	})
	if err != nil || first.Progress.Chapter == nil || first.Progress.Chapter.Position != "42" || first.Progress.Chapter.Total != nil {
		t.Fatalf("chapter 42: view=%+v err=%v", first, err)
	}
	if first.Consumption == nil || first.Consumption.Value != domain.ConsumptionNotStarted {
		t.Fatalf("progress changed the consumption state: %+v", first.Consumption)
	}

	// Chapter 48, then the process restarts.
	if _, err := h.svc.RecordProgress(ctx, app.RecordProgressRequest{
		MutationBase: app.MutationBase{RequestID: "b-ch48", Actor: historyActor, ExpectedRevision: i64P(1)},
		EditionID:    editionID,
		Unit:         domain.UnitChapter,
		Position:     "48",
	}); err != nil {
		t.Fatalf("chapter 48: %v", err)
	}
	h.restart(t)

	// After the restart the latest is 48 and both historical rows survive.
	view, err := h.svc.GetEdition(ctx, editionID)
	if err != nil || view.Progress.Chapter == nil || view.Progress.Chapter.Position != "48" || view.Progress.Chapter.Total != nil {
		t.Fatalf("after restart: view=%+v err=%v", view, err)
	}
	// Both historical rows survive the restart; rows replaced by a newer
	// observation of the same unit keep the effective label (the current
	// value is the derived latest live observation, asserted above), while
	// only an explicit supersedes_id correction marks a row superseded.
	direct, err := h.svc.GetHistory(ctx, app.HistoryQuery{EditionID: &editionID, Limit: 20})
	if err != nil {
		t.Fatalf("direct history: %v", err)
	}
	wantRows := map[string]string{"chapter 42/": "effective", "chapter 48/": "effective"}
	gotRows := rowsByValue(t, direct)
	if !reflect.DeepEqual(gotRows, wantRows) {
		t.Fatalf("history after restart = %v, want %v", gotRows, wantRows)
	}

	// A lower reread position is allowed and does not delete history.
	reread, err := h.svc.RecordProgress(ctx, app.RecordProgressRequest{
		MutationBase: app.MutationBase{RequestID: "b-reread40", Actor: historyActor, ExpectedRevision: i64P(2)},
		EditionID:    editionID,
		Unit:         domain.UnitChapter,
		Position:     "40",
	})
	if err != nil || reread.Progress.Chapter.Position != "40" {
		t.Fatalf("reread 40: view=%+v err=%v", reread, err)
	}

	// A mistaken row is corrected with supersedes_id; both versions remain
	// visible, the mistaken one now the only superseded row.
	wrong, err := h.svc.RecordProgress(ctx, app.RecordProgressRequest{
		MutationBase: app.MutationBase{RequestID: "b-ch49", Actor: historyActor, ExpectedRevision: i64P(3)},
		EditionID:    editionID,
		Unit:         domain.UnitChapter,
		Position:     "49",
	})
	if err != nil || wrong.Progress.Chapter.ID == "" {
		t.Fatalf("mistake 49: view=%+v err=%v", wrong, err)
	}
	fixed, err := h.svc.RecordProgress(ctx, app.RecordProgressRequest{
		MutationBase: app.MutationBase{RequestID: "b-fix48", Actor: historyActor, ExpectedRevision: i64P(4)},
		EditionID:    editionID,
		Unit:         domain.UnitChapter,
		Position:     "48",
		SupersedesID: &wrong.Progress.Chapter.ID,
	})
	if err != nil || fixed.Progress.Chapter.Position != "48" {
		t.Fatalf("corrected 48: view=%+v err=%v", fixed, err)
	}
	after, err := h.svc.GetHistory(ctx, app.HistoryQuery{EditionID: &editionID, Limit: 20})
	if err != nil {
		t.Fatalf("history after correction: %v", err)
	}
	wantCorrected := map[string]string{
		"chapter 40/": "effective",
		"chapter 42/": "effective",
		"chapter 48/": "effective",
		"chapter 49/": "superseded",
	}
	if got := rowsByValue(t, after); !reflect.DeepEqual(got, wantCorrected) {
		t.Fatalf("history after correction = %v, want %v", got, wantCorrected)
	}
	if fixed.Progress.Chapter.Total != nil {
		t.Fatalf("the unknown total was carried forward: %+v", fixed.Progress.Chapter)
	}

	// Negative positions, page fractions and oversized precision are
	// rejected without writes.
	if _, err := h.svc.RecordProgress(ctx, app.RecordProgressRequest{
		MutationBase: app.MutationBase{RequestID: "b-negative", Actor: historyActor, ExpectedRevision: i64P(5)},
		EditionID:    editionID,
		Unit:         domain.UnitChapter,
		Position:     "-1",
	}); errorCode(t, err) != app.ErrInvalidArgument {
		t.Fatalf("negative position: want invalid_argument, got %v", err)
	}
	if _, err := h.svc.RecordProgress(ctx, app.RecordProgressRequest{
		MutationBase: app.MutationBase{RequestID: "b-page-fraction", Actor: historyActor, ExpectedRevision: i64P(5)},
		EditionID:    editionID,
		Unit:         domain.UnitPage,
		Position:     "12.5",
	}); errorCode(t, err) != app.ErrInvalidArgument {
		t.Fatalf("page fraction: want invalid_argument, got %v", err)
	}
	if _, err := h.svc.RecordProgress(ctx, app.RecordProgressRequest{
		MutationBase: app.MutationBase{RequestID: "b-precision", Actor: historyActor, ExpectedRevision: i64P(5)},
		EditionID:    editionID,
		Unit:         domain.UnitChapter,
		Position:     "1.2345",
	}); errorCode(t, err) != app.ErrInvalidArgument {
		t.Fatalf("five decimal digits: want invalid_argument, got %v", err)
	}
	if view, err := h.svc.GetEdition(ctx, editionID); err != nil || view.Progress.Chapter.Position != "48" {
		t.Fatalf("rejections mutated state: view=%+v err=%v", view, err)
	}

	// Both interfaces observe identical effective and superseded values:
	// the MCP transport over the same database must report the same current
	// chapter and the same history multiset.
	m := openMCP(t, h.db)
	edRes := m.call(t, mcp.ToolGetEdition, map[string]any{"edition_id": editionID})
	if !edRes.ok {
		t.Fatalf("mcp get_edition: %s %s", edRes.code, edRes.msg)
	}
	var mcpEdition app.EditionView
	if err := json.Unmarshal(edRes.data, &mcpEdition); err != nil {
		t.Fatalf("mcp get_edition payload: %v", err)
	}
	if mcpEdition.Progress.Chapter == nil || mcpEdition.Progress.Chapter.Position != "48" || mcpEdition.Progress.Chapter.Total != nil {
		t.Fatalf("mcp current chapter = %+v, want 48 with unknown total", mcpEdition.Progress.Chapter)
	}
	histRes := m.call(t, mcp.ToolGetHistory, map[string]any{"edition_id": editionID, "limit": 20})
	if !histRes.ok {
		t.Fatalf("mcp get_history: %s %s", histRes.code, histRes.msg)
	}
	var mcpHistory struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(histRes.data, &mcpHistory); err != nil {
		t.Fatalf("mcp get_history payload: %v", err)
	}
	var mcpKeys []historyKey
	for _, raw := range mcpHistory.Items {
		var item struct {
			Type   string  `json:"type"`
			Value  *string `json:"value"`
			Detail *string `json:"detail"`
			Label  string  `json:"label"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			t.Fatalf("mcp history item: %v", err)
		}
		value := ""
		if item.Value != nil {
			value = *item.Value
		}
		if item.Type == string(app.ActivityProgress) && item.Detail != nil {
			value = value + " " + *item.Detail
		}
		mcpKeys = append(mcpKeys, historyKey{typ: item.Type, value: value, label: item.Label})
	}
	freshDirect, err := h.svc.GetHistory(ctx, app.HistoryQuery{EditionID: &editionID, Limit: 20})
	if err != nil {
		t.Fatalf("direct history for parity: %v", err)
	}
	directKeys := historyKeys(t, freshDirect)
	if want, got := sortedHistoryKeys(t, directKeys), sortedHistoryKeys(t, mcpKeys); !reflect.DeepEqual(want, got) {
		t.Fatalf("mcp history %v differs from the shared service %v", got, want)
	}

	// An MCP-side write is visible through the shared service with the same
	// effective value.
	mWrite := m.call(t, mcp.ToolRecordProgress, map[string]any{
		"request_id":        "b-mcp-reread44",
		"actor":             mcpActor(),
		"edition_id":        editionID,
		"unit":              "chapter",
		"position":          "44",
		"expected_revision": 5,
	})
	if !mWrite.ok {
		t.Fatalf("mcp record_progress: %s %s", mWrite.code, mWrite.msg)
	}
	final, err := h.svc.GetEdition(ctx, editionID)
	if err != nil || final.Progress.Chapter.Position != "44" {
		t.Fatalf("mcp write not visible through the service: view=%+v err=%v", final, err)
	}
}

// rowsByValue folds the progress rows of a history payload by their
// displayed value ("unit position/total" with the unknown total rendered
// as a trailing slash) into the row label.
func rowsByValue(t *testing.T, payload app.ListPayload) map[string]string {
	t.Helper()
	rows := map[string]string{}
	for _, item := range payload.Items {
		ai, ok := item.(app.ActivityItem)
		if !ok {
			t.Fatalf("unexpected history item type %T", item)
		}
		if ai.Type != app.ActivityProgress {
			continue
		}
		value := ""
		if ai.Value != nil {
			value = *ai.Value
		}
		if ai.Detail != nil {
			value = value + " " + *ai.Detail
		}
		rows[value] = ai.Label
	}
	return rows
}

// TestScenarioD drives the retained negative impression: in progress then
// dropped, a revised negative note whose old text survives, a 2/10 rating,
// later lookup by alias, the dropped filter and the activity feed, source
// links and history intact, and an independent saved manga.
func TestScenarioD(t *testing.T) {
	ctx := context.Background()
	h := openHistory(t)

	created, err := h.svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase: app.MutationBase{RequestID: "d-create", Actor: historyActor},
		Titles: []store.TitleInput{
			{Text: "Мрачный Рассвет", Kind: domain.TitleOriginal, Language: strP("ru"), Preferred: true, Provenance: "manual"},
			{Text: "Grim Dawn", Kind: domain.TitleLocalized, Language: strP("en"), Provenance: "manual"},
		},
		Kind: strKind(domain.KindAnime),
	})
	if err != nil || !created.Created || created.EditionID == nil {
		t.Fatalf("create: created=%+v err=%v", created, err)
	}
	editionID := *created.EditionID

	// A source link attached before the drop must survive it.
	streamURL := strP("https://stream.example.org/grim-dawn")
	if _, err := h.svc.AddReference(ctx, app.AddReferenceRequest{
		MutationBase: app.MutationBase{RequestID: "d-reference", Actor: historyActor, ExpectedRevision: i64P(0)},
		EditionID:    editionID,
		Provider:     "stream.example.org",
		URL:          streamURL,
		Role:         domain.RolePublication,
		Provenance:   "manual",
	}); err != nil {
		t.Fatalf("add reference: %v", err)
	}

	// In progress, then dropped with a reason.
	if _, err := h.svc.SetConsumption(ctx, app.SetConsumptionRequest{
		MutationBase: app.MutationBase{RequestID: "d-in-progress", Actor: historyActor, ExpectedRevision: i64P(1)},
		EditionID:    editionID,
		Value:        string(domain.ConsumptionInProgress),
		Reason:       strP("started"),
	}); err != nil {
		t.Fatalf("in progress: %v", err)
	}
	dropped, err := h.svc.SetConsumption(ctx, app.SetConsumptionRequest{
		MutationBase: app.MutationBase{RequestID: "d-dropped", Actor: historyActor, ExpectedRevision: i64P(2)},
		EditionID:    editionID,
		Value:        string(domain.ConsumptionDropped),
		Reason:       strP("abandoned after episode 5"),
	})
	if err != nil || dropped.Consumption == nil || dropped.Consumption.Value != domain.ConsumptionDropped {
		t.Fatalf("dropped: view=%+v err=%v", dropped, err)
	}

	// A negative note, then an explicit revision that keeps the old text.
	noteView, err := h.svc.AddEditionNote(ctx, app.AddNoteRequest{
		MutationBase: app.MutationBase{RequestID: "d-note", Actor: historyActor, ExpectedRevision: i64P(3)},
		TargetID:     editionID,
		Text:         "Slow pacing and a disappointing ending; not for me.",
	})
	if err != nil || len(noteView.Notes) != 1 {
		t.Fatalf("add note: view=%+v err=%v", noteView, err)
	}
	noteID := noteView.Notes[0].ID
	if _, err := h.svc.ReviseNote(ctx, app.ReviseNoteRequest{
		MutationBase: app.MutationBase{RequestID: "d-note-revise", Actor: historyActor, ExpectedRevision: i64P(noteView.Notes[0].Revision)},
		NoteID:       noteID,
		Text:         "Even less enjoyable on a second look; skipping the sequel.",
	}); err != nil {
		t.Fatalf("revise note: %v", err)
	}
	revised, err := h.svc.GetEdition(ctx, editionID)
	if err != nil || len(revised.Notes) != 1 ||
		revised.Notes[0].Text != "Even less enjoyable on a second look; skipping the sequel." ||
		revised.Notes[0].Revision != 2 {
		t.Fatalf("revised note: view=%+v err=%v", revised, err)
	}

	// The negative rating.
	if _, err := h.svc.SetRating(ctx, app.SetRatingRequest{
		MutationBase: app.MutationBase{RequestID: "d-rating", Actor: historyActor, ExpectedRevision: i64P(3)},
		EditionID:    editionID,
		Value:        intP(2),
		Comment:      strP("waste of time"),
	}); err != nil {
		t.Fatalf("rating: %v", err)
	}

	// Later lookup: the alias finds the entry, the dropped filter finds it,
	// and the activity feed carries the dropped observation.
	alias, err := h.svc.SearchLibrary(ctx, app.LibraryFilter{Query: strP("Grim Dawn")})
	if err != nil || alias.Total != 1 || alias.Items[0].(app.WorkCard).WorkID != created.WorkID {
		t.Fatalf("alias lookup: page=%+v err=%v", alias, err)
	}
	droppedFilter, err := h.svc.SearchLibrary(ctx, app.LibraryFilter{Consumptions: []domain.Consumption{domain.ConsumptionDropped}})
	if err != nil || droppedFilter.Total != 1 || droppedFilter.Items[0].(app.WorkCard).WorkID != created.WorkID {
		t.Fatalf("dropped filter: page=%+v err=%v", droppedFilter, err)
	}
	feed, err := h.svc.ListActivity(ctx, app.ActivityQuery{Limit: 20})
	if err != nil {
		t.Fatalf("activity: %v", err)
	}
	foundDropped := false
	for _, item := range feed.Items {
		ai := item.(app.ActivityItem)
		if ai.Type == app.ActivityConsumption && ai.EditionID != nil && *ai.EditionID == editionID &&
			ai.Value != nil && *ai.Value == "dropped" && ai.Label == "effective" {
			foundDropped = true
		}
	}
	if !foundDropped {
		t.Fatalf("activity feed lacks the effective dropped observation: %+v", feed.Items)
	}

	// The history retains every superseded value: in_progress superseded by
	// dropped, both note revisions with their texts, and the rating.
	hist, err := h.svc.GetHistory(ctx, app.HistoryQuery{EditionID: &editionID, Limit: 20})
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	actual := map[string]string{}
	noteLabels := map[string]string{}
	for _, item := range hist.Items {
		ai := item.(app.ActivityItem)
		if ai.Value == nil {
			continue
		}
		if ai.Type == app.ActivityNote {
			noteLabels[*ai.Value] = ai.Label
			continue
		}
		actual[string(ai.Type)+" "+*ai.Value] = ai.Label
	}
	// A plain state transition leaves the earlier row effective (the
	// current state is the derived latest observation, asserted above);
	// note revisions are the superseded/current pair of this scenario.
	for wantKey, wantLabel := range map[string]string{
		"consumption in_progress": "effective",
		"consumption dropped":     "effective",
		"rating 2":                "effective",
	} {
		if got, ok := actual[wantKey]; !ok || got != wantLabel {
			t.Fatalf("history row %q = %q (present %t), want %q", wantKey, got, ok, wantLabel)
		}
	}
	if got, ok := noteLabels["Slow pacing and a disappointing ending; not for me."]; !ok || got != "superseded" {
		t.Fatalf("the old note text in history = %q (present %t), want superseded", got, ok)
	}
	if got, ok := noteLabels["Even less enjoyable on a second look; skipping the sequel."]; !ok || got != "current" {
		t.Fatalf("the current note text in history = %q (present %t), want current", got, ok)
	}

	// Dropping does not erase the source link or imply anything about the
	// publication.
	final, err := h.svc.GetEdition(ctx, editionID)
	if err != nil {
		t.Fatalf("final edition: %v", err)
	}
	if len(final.References) != 1 || final.References[0].URL == nil || *final.References[0].URL != "https://stream.example.org/grim-dawn" {
		t.Fatalf("the drop erased the source link: %+v", final.References)
	}

	// A saved manga remains fully independent of the dropped anime.
	manga, err := h.svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase: app.MutationBase{RequestID: "d-manga", Actor: historyActor},
		Titles:       []store.TitleInput{{Text: "Полночь", Kind: domain.TitleOriginal, Language: strP("ru"), Preferred: true, Provenance: "manual"}},
		Kind:         strKind(domain.KindManga),
	})
	if err != nil || !manga.Created || manga.EditionID == nil || manga.WorkID == created.WorkID {
		t.Fatalf("manga create: created=%+v err=%v", manga, err)
	}
	mangaID := *manga.EditionID
	if _, err := h.svc.SetIntention(ctx, app.SetIntentionRequest{
		MutationBase: app.MutationBase{RequestID: "d-manga-saved", Actor: historyActor, ExpectedRevision: i64P(0)},
		EditionID:    mangaID,
		Value:        string(domain.IntentionSaved),
		Reason:       strP("kept for later"),
	}); err != nil {
		t.Fatalf("manga intention: %v", err)
	}
	mangaView, err := h.svc.GetEdition(ctx, mangaID)
	if err != nil || mangaView.Intention == nil || mangaView.Intention.Value != domain.IntentionSaved ||
		mangaView.Consumption == nil || mangaView.Consumption.Value != domain.ConsumptionNotStarted {
		t.Fatalf("the saved manga drifted: view=%+v err=%v", mangaView, err)
	}
	all, err := h.svc.SearchLibrary(ctx, app.LibraryFilter{})
	if err != nil || all.Total != 2 {
		t.Fatalf("library after the drop: total=%d err=%v", all.Total, err)
	}

	// Both interfaces observe the identical effective and superseded
	// values.
	m := openMCP(t, h.db)
	edRes := m.call(t, mcp.ToolGetEdition, map[string]any{"edition_id": editionID})
	if !edRes.ok {
		t.Fatalf("mcp get_edition: %s %s", edRes.code, edRes.msg)
	}
	var mcpEdition app.EditionView
	if err := json.Unmarshal(edRes.data, &mcpEdition); err != nil {
		t.Fatalf("mcp get_edition payload: %v", err)
	}
	if mcpEdition.Consumption == nil || mcpEdition.Consumption.Value != domain.ConsumptionDropped {
		t.Fatalf("mcp consumption = %+v, want dropped", mcpEdition.Consumption)
	}
	if mcpEdition.Rating == nil || mcpEdition.Rating.Value == nil || *mcpEdition.Rating.Value != 2 {
		t.Fatalf("mcp rating = %+v, want 2/10", mcpEdition.Rating)
	}
	if len(mcpEdition.Notes) != 1 || mcpEdition.Notes[0].Text != "Even less enjoyable on a second look; skipping the sequel." {
		t.Fatalf("mcp notes = %+v, want the revised text", mcpEdition.Notes)
	}
	histRes := m.call(t, mcp.ToolGetHistory, map[string]any{"edition_id": editionID, "limit": 20})
	if !histRes.ok {
		t.Fatalf("mcp get_history: %s %s", histRes.code, histRes.msg)
	}
	var mcpHistory struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(histRes.data, &mcpHistory); err != nil {
		t.Fatalf("mcp get_history payload: %v", err)
	}
	var mcpKeys []historyKey
	for _, raw := range mcpHistory.Items {
		var item struct {
			Type  string  `json:"type"`
			Value *string `json:"value"`
			Label string  `json:"label"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			t.Fatalf("mcp history item: %v", err)
		}
		value := ""
		if item.Value != nil {
			value = *item.Value
		}
		mcpKeys = append(mcpKeys, historyKey{typ: item.Type, value: value, label: item.Label})
	}
	directKeys := historyKeys(t, hist)
	if want, got := sortedHistoryKeys(t, directKeys), sortedHistoryKeys(t, mcpKeys); !reflect.DeepEqual(want, got) {
		t.Fatalf("mcp history %v differs from the shared service %v", got, want)
	}
	actRes := m.call(t, mcp.ToolListActivity, map[string]any{"limit": 20})
	if !actRes.ok {
		t.Fatalf("mcp list_activity: %s %s", actRes.code, actRes.msg)
	}
	if !json.Valid(actRes.data) {
		t.Fatalf("mcp list_activity is not valid JSON: %s", actRes.data)
	}
}

func strKind(k domain.EditionKind) *domain.EditionKind { return &k }
