// MCP request tests for the catalog read tools: the alias search over
// normalized titles, the matched-edition narrowing, the DTO equivalence
// with the shared service (the same payload HTTP and CLI serve), the
// unknown-value and schema-bounding failures, the stale-cursor rejection,
// and the side-effect-free guarantee. The client talks to the server
// through the SDK's in-memory transport against a real temporary
// database; the stdio framing itself is covered by the subprocess tests.
package mcp

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type libraryFixture struct {
	ctx     context.Context
	service *app.Service
	db      *store.Database
	session *mcpsdk.ClientSession
}

func startLibraryFixture(t *testing.T) *libraryFixture {
	t.Helper()
	stateDir := t.TempDir()
	db, err := store.OpenForMaintenance(stateDir)
	if err != nil {
		t.Fatalf("open for maintenance: %v", err)
	}
	if _, err := store.RunMigrations(db.DB()); err != nil {
		db.Close()
		t.Fatalf("run migrations: %v", err)
	}
	service := app.NewService(db)
	srv := New(service, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srvT, cliT := mcpsdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, srvT) }()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "media-core-test", Version: "0"}, nil)
	session, err := client.Connect(ctx, cliT, nil)
	if err != nil {
		cancel()
		db.Close()
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		<-done
		db.Close()
	})
	return &libraryFixture{ctx: ctx, service: service, db: db, session: session}
}

func seedWork(t *testing.T, db *store.Database, titles ...store.TitleInput) string {
	t.Helper()
	w, err := db.CreateWork(context.Background(), time.Now(), titles)
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	return w.ID
}

func seedWorkTitle(t *testing.T, db *store.Database, workID string, in store.TitleInput) {
	t.Helper()
	w, err := db.GetWork(context.Background(), workID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if _, err := db.AddWorkTitle(context.Background(), time.Now(), workID, w.Revision, in); err != nil {
		t.Fatalf("AddWorkTitle: %v", err)
	}
}

func seedEdition(t *testing.T, db *store.Database, workID string, kind domain.EditionKind) string {
	t.Helper()
	w, err := db.GetWork(context.Background(), workID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	e, err := db.AddEdition(context.Background(), time.Now(), workID, w.Revision, store.EditionInput{Kind: kind})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	return e.ID
}

func callTool(t *testing.T, f *libraryFixture, name string, args map[string]any) *mcpsdk.CallToolResult {
	t.Helper()
	res, err := f.session.CallTool(f.ctx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s) protocol error: %v", name, err)
	}
	return res
}

func resultText(t *testing.T, res *mcpsdk.CallToolResult) string {
	t.Helper()
	if len(res.Content) != 1 {
		t.Fatalf("expected one content item, got %d", len(res.Content))
	}
	text, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", res.Content[0])
	}
	return text.Text
}

// sameJSON compares two JSON documents semantically: the SDK re-marshals
// structured output through a map, which orders keys alphabetically, so
// byte identity is not the equivalence the contract requires.
func sameJSON(t *testing.T, got, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("JSON differs semantically:\ngot  %s\nwant %s", got, want)
	}
}

// firstCard unmarshals the first work card of a list payload.
func firstCard(t *testing.T, res *mcpsdk.CallToolResult) (workID string, matched []string) {
	t.Helper()
	var list struct {
		Items []struct {
			WorkID            string   `json:"work_id"`
			MatchedEditionIDs []string `json:"matched_edition_ids"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(resultText(t, res)), &list); err != nil {
		t.Fatalf("unmarshal list payload: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected exactly one card, got %d", len(list.Items))
	}
	return list.Items[0].WorkID, list.Items[0].MatchedEditionIDs
}

func failureCode(t *testing.T, res *mcpsdk.CallToolResult) string {
	t.Helper()
	if !res.IsError {
		t.Fatalf("expected a tool error, got a success: %s", resultText(t, res))
	}
	var wire struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(resultText(t, res)), &wire); err != nil {
		t.Fatalf("the tool error is not the wire error object: %v", err)
	}
	return wire.Code
}

func TestMCPLibrary(t *testing.T) {
	t.Run("alias search returns the shared work-card DTO with the matching editions", func(t *testing.T) {
		f := startLibraryFixture(t)
		workA := seedWork(t, f.db, store.TitleInput{Text: "進撃の巨人", Kind: domain.TitleOriginal, Provenance: "manual"})
		seedWorkTitle(t, f.db, workA, store.TitleInput{Text: "Shingeki no Kyojin", Kind: domain.TitleRomanized, Provenance: "manual"})
		edManga := seedEdition(t, f.db, workA, domain.KindManga)
		edNovel := seedEdition(t, f.db, workA, domain.KindLightNovel)
		seedWork(t, f.db, store.TitleInput{Text: "Unrelated Title", Kind: domain.TitleOriginal, Provenance: "manual"})

		res := callTool(t, f, ToolSearchLibrary, map[string]any{"query": "shingeki"})
		if res.IsError {
			t.Fatalf("search failed: %s", resultText(t, res))
		}
		// The catalog result is equivalent to the shared service
		// payload: the transport renders the DTO, it owns none of it.
		direct, err := f.service.SearchLibrary(f.ctx, app.LibraryFilter{Query: ptr("shingeki")})
		if err != nil {
			t.Fatalf("direct SearchLibrary: %v", err)
		}
		directJSON, err := json.Marshal(direct)
		if err != nil {
			t.Fatalf("marshal direct payload: %v", err)
		}
		sameJSON(t, resultText(t, res), string(directJSON))
		// The lowercase romaji alias matched, and a work-title match
		// counts for every edition of the work.
		workID, matched := firstCard(t, res)
		if workID != workA {
			t.Fatalf("matched work = %q, want %q", workID, workA)
		}
		if !containsString(matched, edManga) || !containsString(matched, edNovel) {
			t.Fatalf("matched editions = %v, want both %v and %v", matched, edManga, edNovel)
		}
	})

	t.Run("the kind filter narrows the matched editions", func(t *testing.T) {
		f := startLibraryFixture(t)
		workA := seedWork(t, f.db, store.TitleInput{Text: "Alias Search Target", Kind: domain.TitleOriginal, Provenance: "manual"})
		edManga := seedEdition(t, f.db, workA, domain.KindManga)
		seedEdition(t, f.db, workA, domain.KindLightNovel)

		res := callTool(t, f, ToolSearchLibrary, map[string]any{
			"query": "alias search",
			"kinds": []string{"manga"},
		})
		if res.IsError {
			t.Fatalf("search failed: %s", resultText(t, res))
		}
		_, matched := firstCard(t, res)
		if len(matched) != 1 || matched[0] != edManga {
			t.Fatalf("matched editions = %v, want only the manga edition %v", matched, edManga)
		}
	})

	t.Run("get_work returns the shared work DTO", func(t *testing.T) {
		f := startLibraryFixture(t)
		workA := seedWork(t, f.db, store.TitleInput{Text: "Work Detail Target", Kind: domain.TitleOriginal, Provenance: "manual"})
		seedEdition(t, f.db, workA, domain.KindManga)

		res := callTool(t, f, ToolGetWork, map[string]any{"work_id": workA})
		if res.IsError {
			t.Fatalf("get_work failed: %s", resultText(t, res))
		}
		direct, err := f.service.GetWork(f.ctx, workA)
		if err != nil {
			t.Fatalf("direct GetWork: %v", err)
		}
		directJSON, err := json.Marshal(direct)
		if err != nil {
			t.Fatalf("marshal direct view: %v", err)
		}
		sameJSON(t, resultText(t, res), string(directJSON))
	})

	t.Run("get_edition returns the shared edition DTO without fabricated facts", func(t *testing.T) {
		f := startLibraryFixture(t)
		workA := seedWork(t, f.db, store.TitleInput{Text: "Edition Detail Target", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, workA, domain.KindManga)

		res := callTool(t, f, ToolGetEdition, map[string]any{"edition_id": ed})
		if res.IsError {
			t.Fatalf("get_edition failed: %s", resultText(t, res))
		}
		direct, err := f.service.GetEdition(f.ctx, ed)
		if err != nil {
			t.Fatalf("direct GetEdition: %v", err)
		}
		directJSON, err := json.Marshal(direct)
		if err != nil {
			t.Fatalf("marshal direct view: %v", err)
		}
		sameJSON(t, resultText(t, res), string(directJSON))
		// A fresh edition has no recorded personal state: the DTO
		// carries null and empty collections, never invented values.
		var view app.EditionView
		if err := json.Unmarshal([]byte(resultText(t, res)), &view); err != nil {
			t.Fatalf("unmarshal view: %v", err)
		}
		if view.Publication != nil || view.Consumption != nil || view.Intention != nil || view.Rating != nil {
			t.Fatalf("unrecorded personal state must be null: %+v", view)
		}
		if len(view.Tags) != 0 || len(view.References) != 0 || len(view.Notes) != 0 {
			t.Fatalf("unrecorded collections must be empty: %+v", view)
		}
	})

	t.Run("unknown work and edition IDs fail with a typed not_found", func(t *testing.T) {
		f := startLibraryFixture(t)
		if code := failureCode(t, callTool(t, f, ToolGetWork, map[string]any{"work_id": "w-does-not-exist"})); code != "not_found" {
			t.Fatalf("get_work failure code = %q, want not_found", code)
		}
		if code := failureCode(t, callTool(t, f, ToolGetEdition, map[string]any{"edition_id": "e-does-not-exist"})); code != "not_found" {
			t.Fatalf("get_edition failure code = %q, want not_found", code)
		}
	})

	t.Run("the schema rejects unknown fields, unknown values and out-of-bounds values", func(t *testing.T) {
		f := startLibraryFixture(t)
		cases := []struct {
			name string
			args map[string]any
		}{
			{"unknown field", map[string]any{"query": "", "bogus_field": 1}},
			{"unknown kind", map[string]any{"kinds": []string{"not_a_kind"}}},
			{"unknown sort", map[string]any{"sort": "bogus"}},
			{"zero limit", map[string]any{"limit": 0}},
			{"oversized limit", map[string]any{"limit": 101}},
			{"zero minimum rating", map[string]any{"min_rating": 0}},
			{"oversized query", map[string]any{"query": string(make([]byte, app.MaxLibraryQueryRunes+1))}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				res := callTool(t, f, ToolSearchLibrary, tc.args)
				if !res.IsError {
					t.Fatalf("expected a tool error for %s, got a success: %s", tc.name, resultText(t, res))
				}
			})
		}
	})

	t.Run("a stale cursor from a different request is rejected", func(t *testing.T) {
		f := startLibraryFixture(t)
		workA := seedWork(t, f.db, store.TitleInput{Text: "Cursor Page One", Kind: domain.TitleOriginal, Provenance: "manual"})
		seedWork(t, f.db, store.TitleInput{Text: "Cursor Page Two", Kind: domain.TitleOriginal, Provenance: "manual"})

		page1 := callTool(t, f, ToolSearchLibrary, map[string]any{"limit": 1})
		if page1.IsError {
			t.Fatalf("first page failed: %s", resultText(t, page1))
		}
		var list app.ListPayload
		if err := json.Unmarshal([]byte(resultText(t, page1)), &list); err != nil {
			t.Fatalf("unmarshal page: %v", err)
		}
		if list.NextCursor == nil || *list.NextCursor == "" {
			t.Fatalf("expected a next cursor for a second page: %+v", list)
		}

		// The same cursor with a different filter is a different
		// request: the shared service rejects it.
		res := callTool(t, f, ToolSearchLibrary, map[string]any{
			"limit":  1,
			"cursor": *list.NextCursor,
			"kinds":  []string{"manga"},
		})
		if code := failureCode(t, res); code != "invalid_argument" {
			t.Fatalf("stale cursor failure code = %q, want invalid_argument", code)
		}

		// The same request continues from the cursor.
		page2 := callTool(t, f, ToolSearchLibrary, map[string]any{"limit": 1, "cursor": *list.NextCursor})
		if page2.IsError {
			t.Fatalf("second page failed: %s", resultText(t, page2))
		}
		var second struct {
			WorkID string `json:"work_id"`
		}
		var secondList struct {
			Items []json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal([]byte(resultText(t, page2)), &secondList); err != nil {
			t.Fatalf("unmarshal second page: %v", err)
		}
		if err := json.Unmarshal(secondList.Items[0], &second); err != nil {
			t.Fatalf("unmarshal second card: %v", err)
		}
		if second.WorkID == "" || second.WorkID == workA {
			t.Fatalf("second page must carry the other work, got %q", second.WorkID)
		}
	})

	t.Run("the read tools leave the database unmutated", func(t *testing.T) {
		f := startLibraryFixture(t)
		workA := seedWork(t, f.db, store.TitleInput{Text: "Side Effect Target", Kind: domain.TitleOriginal, Provenance: "manual"})
		ed := seedEdition(t, f.db, workA, domain.KindManga)

		works, editions := counts(t, f.db)
		w, err := f.db.GetWork(context.Background(), workA)
		if err != nil {
			t.Fatalf("GetWork: %v", err)
		}
		revision := w.Revision

		callTool(t, f, ToolSearchLibrary, map[string]any{"query": "side effect"})
		callTool(t, f, ToolGetWork, map[string]any{"work_id": workA})
		callTool(t, f, ToolGetEdition, map[string]any{"edition_id": ed})
		callTool(t, f, ToolSearchLibrary, map[string]any{"kinds": []string{"manga"}})

		if gotWorks, gotEditions := counts(t, f.db); gotWorks != works || gotEditions != editions {
			t.Fatalf("catalog reads mutated the library: works %d -> %d, editions %d -> %d", works, gotWorks, editions, gotEditions)
		}
		after, err := f.db.GetWork(context.Background(), workA)
		if err != nil {
			t.Fatalf("GetWork: %v", err)
		}
		if after.Revision != revision {
			t.Fatalf("catalog reads bumped the work revision: %d -> %d", revision, after.Revision)
		}
	})
}

func counts(t *testing.T, db *store.Database) (int, int) {
	t.Helper()
	var works, editions int
	if err := db.DB().QueryRow("SELECT count(*) FROM works").Scan(&works); err != nil {
		t.Fatalf("count works: %v", err)
	}
	if err := db.DB().QueryRow("SELECT count(*) FROM editions").Scan(&editions); err != nil {
		t.Fatalf("count editions: %v", err)
	}
	return works, editions
}

func ptr[T any](v T) *T { return &v }

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
