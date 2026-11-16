// Scenario F acceptance: the complete M5 library — works, aliases,
// relations, progress, consumption, intention, notes with revisions,
// ratings, references, origins and import history — survives a consistent
// backup taken while another process keeps writing, and a restore into a
// fresh isolated state directory. The restored state is compared through
// both interfaces (Web and MCP) with ids and instants normalized. The
// owner secret never travels with the archive, the destination's own
// security files survive the restore, and every invalid archive is
// rejected before the destination is touched.
package acceptance

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/backup"
	"github.com/HaidaDaniel/media-core/internal/config"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/httpapi"
	"github.com/HaidaDaniel/media-core/internal/mcp"
	"github.com/HaidaDaniel/media-core/internal/store"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type fixtureIDs struct {
	w1, e1, e2 string // anime work: anime edition, manga edition
	w2, e3     string // web novel work: web novel edition
	w3, e4     string // release manga work: manga edition
}

// seedScenarioF builds the complete M5 fixture through the existing use
// cases, covering every data table: works and titles with aliases,
// editions, consumption, intention, progress with a correction, notes with
// an explicit revision, ratings, references, a confirmed and a proposed
// relation, a confirmed origin, and the import/binding history.
func seedScenarioF(t *testing.T, svc *app.Service) fixtureIDs {
	t.Helper()
	ctx := context.Background()

	// W1: anime with Japanese and Russian titles.
	anime := domain.KindAnime
	ja := strP("ja")
	ru := strP("ru")
	w1, err := svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase: app.MutationBase{RequestID: "f-anime", Actor: acceptanceActor},
		Titles: []store.TitleInput{
			{Text: "星霜の軌跡", Kind: domain.TitleOriginal, Language: ja, Preferred: true, Provenance: "manual"},
			{Text: "След звёзд", Kind: domain.TitleAlias, Language: ru, Provenance: "manual"},
		},
		Kind:     &anime,
		Language: ja,
	})
	if err != nil || !w1.Created || w1.WorkID == "" || w1.EditionID == nil {
		t.Fatalf("create anime: %+v err=%v", w1, err)
	}
	e1 := *w1.EditionID
	done, err := svc.SetConsumption(ctx, app.SetConsumptionRequest{
		MutationBase: app.MutationBase{RequestID: "f-anime-done", Actor: acceptanceActor, ExpectedRevision: i64P(0)},
		EditionID:    e1,
		Value:        string(domain.ConsumptionCompleted),
		Reason:       strP("watched through"),
	})
	if err != nil {
		t.Fatalf("anime consumption: %v", err)
	}
	rated, err := svc.SetRating(ctx, app.SetRatingRequest{
		MutationBase: app.MutationBase{RequestID: "f-anime-rating", Actor: acceptanceActor, ExpectedRevision: i64P(done.Revision)},
		EditionID:    e1,
		Value:        intP(8),
		Comment:      strP("steady, no filler"),
	})
	if err != nil {
		t.Fatalf("anime rating: %v", err)
	}

	// W1 manga edition: not_started + saved + tag + note (revised) +
	// publication reference + confirmed origin.
	manga := domain.KindManga
	added, err := svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase:   app.MutationBase{RequestID: "f-manga", Actor: acceptanceActor},
		Titles:         []store.TitleInput{{Text: "星霜の軌跡", Kind: domain.TitleOriginal, Language: ja, Provenance: "manual"}},
		Kind:           &manga,
		Language:       ja,
		WorkID:         &w1.WorkID,
		Choice:         strChoice(app.ChoiceAddEdition),
		CandidateToken: nil,
	})
	if err != nil {
		// The exact title match produces candidates; resolve first.
		resolved, rerr := svc.ResolveAddition(ctx, app.AdditionRequest{
			MutationBase: app.MutationBase{Actor: acceptanceActor},
			Titles:       []store.TitleInput{{Text: "星霜の軌跡", Kind: domain.TitleOriginal, Language: ja, Provenance: "manual"}},
			Kind:         &manga,
			Language:     ja,
		})
		if rerr != nil || resolved.Outcome != app.OutcomeCandidates || resolved.CandidateToken == nil {
			t.Fatalf("resolve manga: %+v err=%v", resolved, rerr)
		}
		added, err = svc.CreateLibraryEntry(ctx, app.AdditionRequest{
			MutationBase:   app.MutationBase{RequestID: "f-manga", Actor: acceptanceActor},
			Titles:         []store.TitleInput{{Text: "星霜の軌跡", Kind: domain.TitleOriginal, Language: ja, Provenance: "manual"}},
			Kind:           &manga,
			Language:       ja,
			WorkID:         &w1.WorkID,
			Choice:         strChoice(app.ChoiceAddEdition),
			CandidateToken: resolved.CandidateToken,
		})
		if err != nil {
			t.Fatalf("add manga: %+v err=%v", added, err)
		}
	}
	e2 := *added.EditionID
	mv, err := svc.GetEdition(ctx, e2)
	if err != nil {
		t.Fatalf("get manga: %v", err)
	}
	saved, err := svc.SetIntention(ctx, app.SetIntentionRequest{
		MutationBase: app.MutationBase{RequestID: "f-manga-saved", Actor: acceptanceActor, ExpectedRevision: i64P(mv.Revision)},
		EditionID:    e2,
		Value:        string(domain.IntentionSaved),
	})
	if err != nil {
		t.Fatalf("manga intention: %v", err)
	}
	tagged, err := svc.SetEditionTag(ctx, app.TagRequest{
		MutationBase: app.MutationBase{RequestID: "f-manga-tag", Actor: acceptanceActor, ExpectedRevision: i64P(saved.Revision)},
		TargetID:     e2,
		Tag:          "manga-translation-candidate",
	})
	if err != nil {
		t.Fatalf("manga tag: %v", err)
	}
	noted, err := svc.AddEditionNote(ctx, app.AddNoteRequest{
		MutationBase: app.MutationBase{RequestID: "f-manga-note", Actor: acceptanceActor, ExpectedRevision: i64P(tagged.Revision)},
		TargetID:     e2,
		Text:         "Defer smartphone reading until a stable reader is chosen.",
	})
	if err != nil {
		t.Fatalf("manga note: %v", err)
	}
	noteView, err := svc.GetEdition(ctx, e2)
	if err != nil || len(noteView.Notes) != 1 {
		t.Fatalf("note view: %+v err=%v", noteView, err)
	}
	if _, err := svc.ReviseNote(ctx, app.ReviseNoteRequest{
		MutationBase: app.MutationBase{RequestID: "f-manga-note-rev", Actor: acceptanceActor, ExpectedRevision: i64P(noteView.Notes[0].Revision)},
		NoteID:       noteView.Notes[0].ID,
		Text:         "Reader chosen: defer until the scanlation stabilizes.",
	}); err != nil {
		t.Fatalf("revise note: %v", err)
	}
	linked, err := svc.AddReference(ctx, app.AddReferenceRequest{
		MutationBase:  app.MutationBase{RequestID: "f-manga-url", Actor: acceptanceActor, ExpectedRevision: i64P(noted.Revision)},
		EditionID:     e2,
		Provider:      "manga.example.org",
		URL:           strP("https://manga.example.org/star-frost"),
		Role:          domain.RolePublication,
		Authoritative: true,
		Provenance:    "manual",
	})
	if err != nil || len(linked.References) != 1 {
		t.Fatalf("manga reference: %+v err=%v", linked, err)
	}
	rel, err := svc.AddRelation(ctx, app.AddRelationRequest{
		MutationBase: app.MutationBase{RequestID: "f-relation", Actor: acceptanceActor, ExpectedRevision: i64P(rated.Revision)},
		Scope:        domain.ScopeEdition,
		Type:         domain.RelationAdaptationOf,
		FromID:       e1,
		ToID:         e2,
		State:        domain.ClaimConfirmed,
		Provenance:   "user",
	})
	if err != nil || len(rel.Confirmed) != 1 {
		t.Fatalf("adaptation relation: %+v err=%v", rel, err)
	}
	mv2, err := svc.GetEdition(ctx, e2)
	if err != nil {
		t.Fatalf("get manga for origin: %v", err)
	}
	origin, err := svc.RecordOrigin(ctx, app.RecordOriginRequest{
		MutationBase: app.MutationBase{RequestID: "f-origin", Actor: acceptanceActor, ExpectedRevision: i64P(mv2.Revision)},
		EditionID:    e2,
		Kind:         domain.OriginOriginal,
		State:        domain.ClaimProposed,
		Provenance:   "guessed",
	})
	if err != nil || len(origin.Claims) != 1 {
		t.Fatalf("origin: %+v err=%v", origin, err)
	}
	mv3, err := svc.GetEdition(ctx, e2)
	if err != nil {
		t.Fatalf("get manga for origin confirm: %v", err)
	}
	if _, err := svc.ConfirmOrigin(ctx, app.ConfirmOriginRequest{
		MutationBase: app.MutationBase{RequestID: "f-origin-confirm", Actor: acceptanceActor, ExpectedRevision: i64P(mv3.Revision)},
		EditionID:    e2,
		ClaimID:      origin.Claims[0].ID,
	}); err != nil {
		t.Fatalf("confirm origin: %v", err)
	}

	// W2: web novel with the progress history, including a correction.
	webnovel := domain.KindWebNovel
	w2r, err := svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase: app.MutationBase{RequestID: "f-webnovel", Actor: acceptanceActor},
		Titles:       []store.TitleInput{{Text: "Chronicle of Hours", Kind: domain.TitleOriginal, Provenance: "manual"}},
		Kind:         &webnovel,
	})
	if err != nil || !w2r.Created {
		t.Fatalf("create web novel: %+v err=%v", w2r, err)
	}
	e3 := *w2r.EditionID
	reading, err := svc.SetConsumption(ctx, app.SetConsumptionRequest{
		MutationBase: app.MutationBase{RequestID: "f-novel-reading", Actor: acceptanceActor, ExpectedRevision: i64P(0)},
		EditionID:    e3,
		Value:        string(domain.ConsumptionInProgress),
	})
	if err != nil {
		t.Fatalf("novel consumption: %v", err)
	}
	positions := []struct {
		pos string
		sup *string
	}{
		{"42", nil}, {"48", nil}, {"40", nil},
	}
	prev := reading.Revision
	var lastView app.EditionView
	for i, p := range positions {
		lastView, err = svc.RecordProgress(ctx, app.RecordProgressRequest{
			MutationBase: app.MutationBase{RequestID: fmt.Sprintf("f-ch-%d", i), Actor: acceptanceActor, ExpectedRevision: i64P(prev)},
			EditionID:    e3,
			Unit:         domain.UnitChapter,
			Position:     p.pos,
			SupersedesID: p.sup,
		})
		if err != nil {
			t.Fatalf("progress %s: %v", p.pos, err)
		}
		prev = lastView.Revision
	}
	// The mistaken 49 and its explicit correction.
	wrong, err := svc.RecordProgress(ctx, app.RecordProgressRequest{
		MutationBase: app.MutationBase{RequestID: "f-ch-49", Actor: acceptanceActor, ExpectedRevision: i64P(prev)},
		EditionID:    e3,
		Unit:         domain.UnitChapter,
		Position:     "49",
	})
	if err != nil {
		t.Fatalf("progress 49: %v", err)
	}
	_, err = svc.RecordProgress(ctx, app.RecordProgressRequest{
		MutationBase: app.MutationBase{RequestID: "f-ch-fix", Actor: acceptanceActor, ExpectedRevision: i64P(wrong.Revision)},
		EditionID:    e3,
		Unit:         domain.UnitChapter,
		Position:     "48",
		SupersedesID: &wrong.Progress.Chapter.ID,
	})
	if err != nil {
		t.Fatalf("progress correction: %v", err)
	}

	// W3: release manga with the import history.
	w3r, err := svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase: app.MutationBase{RequestID: "f-releasemanga", Actor: acceptanceActor},
		Titles:       []store.TitleInput{{Text: "Release Manga", Kind: domain.TitleOriginal, Provenance: "manual"}},
		Kind:         &manga,
	})
	if err != nil || !w3r.Created {
		t.Fatalf("create release manga: %+v err=%v", w3r, err)
	}
	e4 := *w3r.EditionID
	importer := app.Actor{Principal: "acceptance-importer", Origin: domain.OriginImport}
	res, err := svc.ImportEvents(ctx, app.ImportRequest{RequestID: "f-import", Actor: importer, Raw: readEnvelope(t, "envelope_valid.json")})
	if err != nil || res.Imported != 1 {
		t.Fatalf("import: %+v err=%v", res, err)
	}
	if _, err := svc.BindRelease(ctx, app.BindReleaseRequest{
		MutationBase:  app.MutationBase{RequestID: "f-bind", Actor: importer},
		PublicationID: "pub_mu_1234",
		EditionID:     e4,
	}); err != nil {
		t.Fatalf("bind release: %v", err)
	}
	// The proposed (unconfirmed) relation to the web novel.
	w2v, err := svc.GetWork(ctx, w2r.WorkID)
	if err != nil {
		t.Fatalf("get web novel work: %v", err)
	}
	_, err = svc.AddRelation(ctx, app.AddRelationRequest{
		MutationBase: app.MutationBase{RequestID: "f-relation-proposed", Actor: acceptanceActor, ExpectedRevision: i64P(w2v.Revision)},
		Scope:        domain.ScopeWork,
		Type:         domain.RelationSequelOf,
		FromID:       w3r.WorkID,
		ToID:         w2r.WorkID,
		State:        domain.ClaimProposed,
		Provenance:   "user",
	})
	if err != nil {
		t.Fatalf("proposed relation: %v", err)
	}

	return fixtureIDs{w1: w1.WorkID, e1: e1, e2: e2, w2: w2r.WorkID, e3: e3, w3: w3r.WorkID, e4: e4}
}

// openFreshService initializes one temporary state dir and returns a
// service over it.
func openFreshService(t *testing.T) (string, *app.Service, *store.Database) {
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
	t.Cleanup(func() { db.Close() })
	return dir, app.NewService(db), db
}

// assertRestoredFixture re-asserts the whole fixture through the shared
// service on the restored state: every entity, value and retained history
// row the backup carried.
func assertRestoredFixture(t *testing.T, svc *app.Service, ids fixtureIDs, minChapters int) {
	t.Helper()
	ctx := context.Background()

	// The three works and their titles.
	list, err := svc.SearchLibrary(ctx, app.LibraryFilter{Limit: 100})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if list.Total != 3 {
		t.Fatalf("restored works = %d, want 3", list.Total)
	}
	w1, err := svc.GetWork(ctx, ids.w1)
	if err != nil {
		t.Fatalf("get work 1: %v", err)
	}
	var hasJP, hasRU bool
	for _, title := range w1.Titles {
		if title.Text == "星霜の軌跡" {
			hasJP = true
		}
		if title.Text == "След звёзд" {
			hasRU = true
		}
	}
	if !hasJP || !hasRU {
		t.Fatalf("work 1 titles = %+v, want the Japanese original and the Russian alias", w1.Titles)
	}

	// The anime edition: completed, rated 8 with the comment.
	e1, err := svc.GetEdition(ctx, ids.e1)
	if err != nil {
		t.Fatalf("get anime: %v", err)
	}
	if e1.Consumption == nil || e1.Consumption.Value != domain.ConsumptionCompleted {
		t.Fatalf("anime consumption = %+v, want completed", e1.Consumption)
	}
	if e1.Rating == nil || e1.Rating.Value == nil || *e1.Rating.Value != 8 || e1.Rating.Comment == nil || *e1.Rating.Comment != "steady, no filler" {
		t.Fatalf("anime rating = %+v, want 8 with the comment", e1.Rating)
	}

	// The manga edition: saved, tagged, the revised note, the reference,
	// the confirmed origin.
	e2, err := svc.GetEdition(ctx, ids.e2)
	if err != nil {
		t.Fatalf("get manga: %v", err)
	}
	if e2.Intention == nil || e2.Intention.Value != domain.IntentionSaved {
		t.Fatalf("manga intention = %+v, want saved", e2.Intention)
	}
	if len(e2.Tags) != 1 || e2.Tags[0] != "manga-translation-candidate" {
		t.Fatalf("manga tags = %v", e2.Tags)
	}
	if len(e2.Notes) != 1 || e2.Notes[0].Text != "Reader chosen: defer until the scanlation stabilizes." {
		t.Fatalf("manga note = %+v, want the revised text", e2.Notes)
	}
	if len(e2.References) != 1 || e2.References[0].URL == nil || *e2.References[0].URL != "https://manga.example.org/star-frost" {
		t.Fatalf("manga references = %+v", e2.References)
	}
	origins, err := svc.ListOrigins(ctx, ids.e2)
	if err != nil {
		t.Fatalf("list origins: %v", err)
	}
	if len(origins.Claims) != 1 || origins.Claims[0].State != domain.ClaimConfirmed {
		t.Fatalf("manga origins = %+v, want the confirmed claim", origins)
	}

	// The note history retains both revisions in order.
	hist, err := svc.GetHistory(ctx, app.HistoryQuery{EditionID: strP(ids.e2), Limit: 100})
	if err != nil {
		t.Fatalf("manga history: %v", err)
	}
	type noteRow struct {
		text  string
		label string
	}
	var noteRows []noteRow
	for _, raw := range hist.Items {
		item, ok := raw.(app.ActivityItem)
		if !ok {
			t.Fatalf("note history item is not an ActivityItem: %T", raw)
		}
		if item.Type == app.ActivityNote && item.Value != nil {
			noteRows = append(noteRows, noteRow{text: *item.Value, label: item.Label})
		}
	}
	if len(noteRows) != 2 {
		t.Fatalf("note history = %+v, want both revisions", noteRows)
	}
	sort.Slice(noteRows, func(i, j int) bool { return noteRows[i].text < noteRows[j].text })
	want := []noteRow{
		{text: "Defer smartphone reading until a stable reader is chosen.", label: "superseded"},
		{text: "Reader chosen: defer until the scanlation stabilizes.", label: "current"},
	}
	if !reflect.DeepEqual(noteRows, want) {
		t.Fatalf("note history = %+v, want both revisions with their labels", noteRows)
	}

	// The web novel: in_progress plus the full chapter history including
	// the correction pair, plus whatever the concurrent writer committed.
	e3, err := svc.GetEdition(ctx, ids.e3)
	if err != nil {
		t.Fatalf("get novel: %v", err)
	}
	if e3.Consumption == nil || e3.Consumption.Value != domain.ConsumptionInProgress {
		t.Fatalf("novel consumption = %+v, want in_progress", e3.Consumption)
	}
	nhist, err := svc.GetHistory(ctx, app.HistoryQuery{EditionID: strP(ids.e3), Limit: 100})
	if err != nil {
		t.Fatalf("novel history: %v", err)
	}
	var chapters []string
	superseded := 0
	for _, raw := range nhist.Items {
		item, ok := raw.(app.ActivityItem)
		if !ok {
			t.Fatalf("chapter history item is not an ActivityItem: %T", raw)
		}
		if item.Type != app.ActivityProgress {
			continue
		}
		if item.Detail == nil {
			t.Fatalf("chapter history item %s has no position", item.ID)
		}
		chapters = append(chapters, strings.Split(*item.Detail, "/")[0])
		if item.Label == "superseded" {
			superseded++
		}
	}
	if len(chapters) < minChapters {
		t.Fatalf("chapter history = %v, want at least %d rows", chapters, minChapters)
	}
	if superseded != 1 {
		t.Fatalf("superseded chapter rows = %d, want exactly 1 (the corrected 49)", superseded)
	}
	sort.Strings(chapters)
	for _, want := range []string{"40", "42", "48", "48", "49"} {
		found := false
		for _, c := range chapters {
			if c == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("chapter history %v is missing %s", chapters, want)
		}
	}

	// The relations: the confirmed adaptation on the editions and the
	// proposed sequel between the works.
	rels, err := svc.ListRelations(ctx, domain.ScopeEdition, ids.e1)
	if err != nil {
		t.Fatalf("edition relations: %v", err)
	}
	if len(rels.Confirmed) != 1 || rels.Confirmed[0].ToID != ids.e2 {
		t.Fatalf("adaptation relations = %+v, want the confirmed edge", rels)
	}
	wrels, err := svc.ListRelations(ctx, domain.ScopeWork, ids.w3)
	if err != nil {
		t.Fatalf("work relations: %v", err)
	}
	if len(wrels.Proposed) != 1 || wrels.Proposed[0].ToID != ids.w2 {
		t.Fatalf("proposed relations = %+v, want the proposed sequel", wrels)
	}

	// The import history: the bound event, one projection, the binding.
	binding, err := svc.GetReleaseBinding(ctx, "", "pub_mu_1234")
	if err != nil || binding == nil || binding.EditionID != ids.e4 {
		t.Fatalf("release binding = %+v err=%v, want active on the release manga", binding, err)
	}
	rel, err := svc.ListReleases(ctx, app.ReleaseQuery{EditionID: ids.e4})
	if err != nil || rel.Total != 1 {
		t.Fatalf("release projections = %+v err=%v, want exactly 1", rel, err)
	}
}

// compareWebAndMCP runs the same reads through the Web API and MCP on the
// restored state and requires the normalized payloads to match.
func compareWebAndMCP(t *testing.T, restoredDir, secret string, ids fixtureIDs) {
	t.Helper()

	// Web: the loopback API with an owner session.
	db, err := store.Open(restoredDir)
	if err != nil {
		t.Fatalf("open restored: %v", err)
	}
	defer db.Close()
	srv, err := httpapi.New(&config.Config{StateDir: restoredDir, ListenAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	srv.WithCatalog(app.NewService(db))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()
	var addr net.Addr
	for addr == nil {
		select {
		case err := <-done:
			t.Fatalf("serve exited early: %v", err)
		case <-time.After(5 * time.Millisecond):
			addr = srv.Addr()
		}
	}
	t.Cleanup(func() {
		cancel()
		<-done
	})
	base := "http://" + addr.String()
	client := &http.Client{Timeout: 10 * time.Second}
	loginReq, err := http.NewRequest(http.MethodPost, base+"/api/v1/session", strings.NewReader(`{"owner_secret":"`+secret+`"}`))
	if err != nil {
		t.Fatalf("new login request: %v", err)
	}
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set("Origin", "http://"+addr.String())
	login, err := client.Do(loginReq)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", login.StatusCode)
	}
	var cookie string
	for _, c := range login.Cookies() {
		if c.Name == httpapi.SessionCookieName {
			cookie = c.String()
		}
	}
	webGet := func(path string) any {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, base+path, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Cookie", cookie)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status = %d body = %s", path, resp.StatusCode, body)
		}
		var env struct {
			OK   bool            `json:"ok"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &env); err != nil || !env.OK {
			t.Fatalf("GET %s: not the ok envelope: %s", path, body)
		}
		var v any
		if err := json.Unmarshal(env.Data, &v); err != nil {
			t.Fatalf("GET %s: data is not JSON: %v", path, err)
		}
		return v
	}

	// MCP: the in-memory transport over the same restored state.
	mdb, err := store.Open(restoredDir)
	if err != nil {
		t.Fatalf("open restored for mcp: %v", err)
	}
	mcpSrv := mcp.New(app.NewService(mdb), slog.New(slog.NewTextHandler(io.Discard, nil)))
	srvT, cliT := mcpsdk.NewInMemoryTransports()
	mctx, mcancel := context.WithCancel(context.Background())
	mdone := make(chan error, 1)
	go func() { mdone <- mcpSrv.Serve(mctx, srvT) }()
	mclient := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "scenario-f", Version: "0"}, nil)
	msession, err := mclient.Connect(mctx, cliT, nil)
	if err != nil {
		mcancel()
		t.Fatalf("connect mcp: %v", err)
	}
	t.Cleanup(func() {
		mcancel()
		<-mdone
		mdb.Close()
	})
	mcpCall := func(tool string, args map[string]any) any {
		t.Helper()
		res, err := msession.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: tool, Arguments: args})
		if err != nil {
			t.Fatalf("call %s: %v", tool, err)
		}
		if len(res.Content) != 1 {
			t.Fatalf("call %s: want one content item", tool)
		}
		text, ok := res.Content[0].(*mcpsdk.TextContent)
		if !ok {
			t.Fatalf("call %s: content is not text", tool)
		}
		if res.IsError {
			t.Fatalf("call %s: unexpected error: %s", tool, text.Text)
		}
		var v any
		if err := json.Unmarshal([]byte(text.Text), &v); err != nil {
			t.Fatalf("call %s: payload is not JSON: %v", tool, err)
		}
		return v
	}

	steps := []struct {
		name string
		web  any
		mcp  any
	}{
		{"works list", webGet("/api/v1/works?limit=100"), mcpCall("search_library", map[string]any{"limit": 100})},
		{"work detail", webGet("/api/v1/works/" + ids.w1), mcpCall("get_work", map[string]any{"work_id": ids.w1})},
		{"edition detail", webGet("/api/v1/editions/" + ids.e3), mcpCall("get_edition", map[string]any{"edition_id": ids.e3})},
		{"edition history", webGet("/api/v1/editions/" + ids.e3 + "/history?limit=100"), mcpCall("get_history", map[string]any{"edition_id": ids.e3, "limit": 100})},
		{"work relations", webGet("/api/v1/works/" + ids.w1 + "/relations"), mcpCall("list_related", map[string]any{"scope": "work", "entity_id": ids.w1, "depth": 1})},
		{"activity", webGet("/api/v1/activity?limit=100"), mcpCall("list_activity", map[string]any{"limit": 100})},
	}
	for _, step := range steps {
		if !reflect.DeepEqual(normalizeF(step.web), normalizeF(step.mcp)) {
			t.Fatalf("%s: Web and MCP payloads differ after normalization:\nweb: %v\nmcp: %v", step.name, step.web, step.mcp)
		}
	}
}

// normalizeF strips ids and instants and sorts the heterogeneous
// collections so the two transport payloads are comparable; it is the
// acceptance normalizer from the parity suite.
func normalizeF(v any) any {
	idKeys := map[string]bool{
		"id": true, "work_id": true, "edition_id": true, "relation_id": true,
		"claim_id": true, "reference_id": true, "note_id": true, "record_id": true,
		"replaces_id": true, "supersedes_id": true, "next_cursor": true,
		"candidate_token": true, "request_fingerprint": true,
		"from_id": true, "to_id": true, "entity_id": true, "target_id": true,
	}
	timeKeys := map[string]bool{
		"created_at": true, "updated_at": true, "observed_at": true, "recorded_at": true,
		"confirmed_at": true, "revoked_at": true, "archived_at": true, "retired_at": true,
	}
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			switch {
			case idKeys[k]:
				t[k] = "<id>"
			case k == "matched_edition_ids":
				if ids, ok := val.([]any); ok {
					norm := make([]any, len(ids))
					for i := range norm {
						norm[i] = "<id>"
					}
					t[k] = norm
				}
			case timeKeys[k]:
				t[k] = "<time>"
			default:
				t[k] = normalizeF(val)
			}
		}
		return t
	case []any:
		for i := range t {
			t[i] = normalizeF(t[i])
		}
		return t
	default:
		return v
	}
}

// TestScenarioF is the Scenario F contract: the complete M5 fixture
// survives a consistent backup taken under concurrent writes and a
// restore into a fresh isolated state directory, verified through both
// interfaces, with every invalid archive rejected before the destination
// is touched and the destination's own security files surviving.
func TestScenarioF(t *testing.T) {
	ctx := context.Background()
	srcDir, svc, srcDB := openFreshService(t)

	// The source owner secret: it must never travel with the archive.
	srcSecret, err := httpapi.GenerateOwnerSecret(srcDir)
	if err != nil {
		t.Fatalf("generate owner secret: %v", err)
	}

	ids := seedScenarioF(t, svc)

	// A second process keeps writing while the backup runs. Warm up two
	// committed chapter rows first so the snapshot provably contains
	// post-fixture work, then hammer until the backup is done.
	writerDB, err := store.Open(srcDir)
	if err != nil {
		t.Fatalf("open writer handle: %v", err)
	}
	t.Cleanup(func() { writerDB.Close() })
	writerSvc := app.NewService(writerDB)
	hammer := make(chan struct{})
	var hammerCount atomic.Int32
	var hammerMu sync.Mutex
	var hammerErr error
	go func() {
		fail := func(e error) {
			hammerMu.Lock()
			hammerErr = e
			hammerMu.Unlock()
		}
		pos := 50
		for {
			select {
			case <-hammer:
				return
			default:
			}
			view, err := writerSvc.GetEdition(ctx, ids.e3)
			if err != nil {
				fail(err)
				return
			}
			if _, err := writerSvc.RecordProgress(ctx, app.RecordProgressRequest{
				MutationBase: app.MutationBase{RequestID: fmt.Sprintf("f-hammer-%d", pos), Actor: acceptanceActor, ExpectedRevision: i64P(view.Revision)},
				EditionID:    ids.e3,
				Unit:         domain.UnitChapter,
				Position:     fmt.Sprintf("%d", pos),
			}); err != nil {
				fail(err)
				return
			}
			pos++
			hammerCount.Add(1)
			time.Sleep(time.Millisecond)
		}
	}()
	time.Sleep(50 * time.Millisecond)
	hammerMu.Lock()
	if hammerErr != nil {
		hammerMu.Unlock()
		t.Fatalf("warm-up writer: %v", hammerErr)
	}
	hammerMu.Unlock()
	warmup := hammerCount.Load()

	// The consistent backup while the writer runs.
	archiveDir := t.TempDir()
	res, err := svc.CreateBackup(ctx, app.BackupRequest{DestDir: archiveDir, Name: "scenario-f"})
	if err != nil {
		close(hammer)
		t.Fatalf("CreateBackup: %v", err)
	}
	close(hammer)
	hammerMu.Lock()
	if hammerErr != nil {
		hammerMu.Unlock()
		t.Fatalf("concurrent writer: %v", hammerErr)
	}
	hammerMu.Unlock()
	archive := res.Archive

	// The archive is exactly the snapshot and the manifest: no owner
	// secret, no state file, and the manifest hash matches the file.
	entries, err := os.ReadDir(archive)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("archive holds %d entries, want exactly the database and the manifest", len(entries))
	}
	for _, e := range entries {
		if e.Name() == "owner-secret" {
			t.Fatalf("the owner secret leaked into the archive")
		}
	}
	manifestRaw, err := os.ReadFile(filepath.Join(archive, backup.ManifestFileName))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if strings.Contains(string(manifestRaw), srcSecret) {
		t.Fatalf("the manifest carries the owner secret")
	}
	var manifest backup.Manifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatalf("manifest is not JSON: %v", err)
	}
	if manifest.Format != backup.ArchiveFormatName {
		t.Fatalf("manifest format = %q", manifest.Format)
	}

	// The destination: a fresh isolated state dir with its own owner
	// secret, which must survive the restore.
	destDir := filepath.Join(t.TempDir(), "restored")
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		t.Fatalf("create destination: %v", err)
	}
	destSecret, err := httpapi.GenerateOwnerSecret(destDir)
	if err != nil {
		t.Fatalf("destination owner secret: %v", err)
	}
	destSecretFile := filepath.Join(destDir, "owner-secret")
	before, err := os.ReadFile(destSecretFile)
	if err != nil {
		t.Fatalf("read destination secret: %v", err)
	}

	// Rejections, each before the destination is touched: a traversal
	// entry, a corrupt hash, an oversized (size-mismatching) archive and
	// a newer schema.
	destUntouched := func(step string) {
		t.Helper()
		entries, err := os.ReadDir(destDir)
		if err != nil {
			t.Fatalf("%s: read destination: %v", step, err)
		}
		if len(entries) != 1 || entries[0].Name() != "owner-secret" {
			t.Fatalf("%s: the destination changed before the rejection: %v", step, entries)
		}
	}

	t.Run("traversal entry is rejected", func(t *testing.T) {
		bad := t.TempDir()
		if err := os.Symlink(filepath.Join(archive, backup.DBFileName), filepath.Join(bad, backup.DBFileName)); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		raw, err := os.ReadFile(filepath.Join(archive, backup.ManifestFileName))
		if err != nil {
			t.Fatalf("read manifest: %v", err)
		}
		if err := os.WriteFile(filepath.Join(bad, backup.ManifestFileName), raw, 0o600); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
		_, err = app.Restore(ctx, app.RestoreRequest{Archive: bad, StateDir: destDir})
		if errorCode(t, err) != app.ErrInvalidArgument {
			t.Fatalf("traversal restore: want invalid_argument, got %v", err)
		}
		destUntouched("traversal")
	})

	t.Run("corrupt hash is rejected", func(t *testing.T) {
		bad := t.TempDir()
		if err := copyDir(t, archive, bad); err != nil {
			t.Fatalf("copy archive: %v", err)
		}
		dbFile := filepath.Join(bad, backup.DBFileName)
		data, err := os.ReadFile(dbFile)
		if err != nil {
			t.Fatalf("read db: %v", err)
		}
		data[len(data)/2] ^= 0x01
		if err := os.WriteFile(dbFile, data, 0o600); err != nil {
			t.Fatalf("tamper db: %v", err)
		}
		_, err = app.Restore(ctx, app.RestoreRequest{Archive: bad, StateDir: destDir})
		if errorCode(t, err) != app.ErrInvalidArgument {
			t.Fatalf("corrupt restore: want invalid_argument, got %v", err)
		}
		destUntouched("corrupt hash")
	})

	t.Run("size mismatch is rejected", func(t *testing.T) {
		bad := t.TempDir()
		if err := copyDir(t, archive, bad); err != nil {
			t.Fatalf("copy archive: %v", err)
		}
		mp := filepath.Join(bad, backup.ManifestFileName)
		raw, err := os.ReadFile(mp)
		if err != nil {
			t.Fatalf("read manifest: %v", err)
		}
		var m backup.Manifest
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("manifest: %v", err)
		}
		m.DB.Size += 1
		out, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			t.Fatalf("marshal manifest: %v", err)
		}
		if err := os.WriteFile(mp, out, 0o600); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
		_, err = app.Restore(ctx, app.RestoreRequest{Archive: bad, StateDir: destDir})
		if errorCode(t, err) != app.ErrInvalidArgument {
			t.Fatalf("size-mismatch restore: want invalid_argument, got %v", err)
		}
		destUntouched("size mismatch")
	})

	t.Run("newer schema is rejected", func(t *testing.T) {
		bad := t.TempDir()
		if _, err := backup.SyntheticArchive(bad, manifest.Schema+1, time.Now().UTC()); err != nil {
			t.Fatalf("synthetic archive: %v", err)
		}
		_, err = app.Restore(ctx, app.RestoreRequest{Archive: bad, StateDir: destDir})
		if errorCode(t, err) != app.ErrSchemaMismatch {
			t.Fatalf("newer-schema restore: want schema_mismatch, got %v", err)
		}
		destUntouched("newer schema")
	})

	// The restore into the fresh isolated destination.
	if _, err := app.Restore(ctx, app.RestoreRequest{Archive: archive, StateDir: destDir}); err != nil {
		t.Fatalf("restore: %v", err)
	}

	// Destination security survives: the secret is byte-identical, the
	// modes are the restrictive ones, and the source secret never arrived.
	after, err := os.ReadFile(destSecretFile)
	if err != nil {
		t.Fatalf("read destination secret after: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("the destination owner secret changed across the restore")
	}
	srcSecretBytes, err := hex.DecodeString(srcSecret)
	if err != nil {
		t.Fatalf("decode source secret: %v", err)
	}
	if bytes.Equal(after, srcSecretBytes) {
		t.Fatalf("the destination secret was replaced by the source secret")
	}
	for path, want := range map[string]os.FileMode{
		destDir:                                  0o700,
		destSecretFile:                           0o600,
		filepath.Join(destDir, store.DBFileName): 0o600,
	} {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if fi.Mode().Perm() != want {
			t.Fatalf("%s mode = %o, want %o", path, fi.Mode().Perm(), want)
		}
	}

	// The restored state carries the complete fixture. The snapshot
	// boundary sits between the warm-up and the hammer writes.
	restoredDir, rsvc, _ := openFreshServiceAt(t, destDir)
	assertRestoredFixture(t, rsvc, ids, int(5+warmup))
	_ = restoredDir
	_ = srcDB

	// Both interfaces see the same restored state.
	compareWebAndMCP(t, destDir, destSecret, ids)

	// An active handle blocks the replacement with the locked error, and
	// a refused replacement leaves the destination byte-identical.
	handle, err := store.Open(destDir)
	if err != nil {
		t.Fatalf("open destination handle: %v", err)
	}
	dbBytes, err := os.ReadFile(filepath.Join(destDir, store.DBFileName))
	if err != nil {
		t.Fatalf("read destination db: %v", err)
	}
	_, err = app.Restore(ctx, app.RestoreRequest{Archive: archive, StateDir: destDir, Force: true})
	if errorCode(t, err) != app.ErrLocked {
		t.Fatalf("restore under an active handle: want locked, got %v", err)
	}
	handle.Close()
	_, err = app.Restore(ctx, app.RestoreRequest{Archive: archive, StateDir: destDir})
	if errorCode(t, err) != app.ErrConflict {
		t.Fatalf("restore over an existing destination without force: want conflict, got %v", err)
	}
	afterBytes, err := os.ReadFile(filepath.Join(destDir, store.DBFileName))
	if err != nil {
		t.Fatalf("read destination db after: %v", err)
	}
	if !bytes.Equal(dbBytes, afterBytes) {
		t.Fatalf("the destination database changed across the refused restores")
	}
}

// openFreshServiceAt opens a service over an existing initialized state
// dir (the restored destination).
func openFreshServiceAt(t *testing.T, dir string) (string, *app.Service, *store.Database) {
	t.Helper()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open %s: %v", dir, err)
	}
	t.Cleanup(func() { db.Close() })
	return dir, app.NewService(db), db
}

// copyDir copies a flat archive directory.
func copyDir(t *testing.T, src, dst string) error {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), raw, 0o600); err != nil {
			return err
		}
	}
	return nil
}
