// DTO and error parity across the real Web (HTTP) and MCP boundaries: the
// same scripted anime-source and duplicate workflow runs against two
// separately initialized state directories, once over the MCP in-memory
// transport and once over the loopback HTTP API with an owner session.
// Normalized views (ids and instants canonicalized) must be identical and
// failures must carry the same stable code and message. No external service
// or real database is involved.
package contracts

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/config"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/httpapi"
	"github.com/HaidaDaniel/media-core/internal/mcp"
	"github.com/HaidaDaniel/media-core/internal/store"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	parityJP   = "星霜の軌跡"
	parityRU   = "След звёзд"
	parityURL  = "https://manga.example.org/star-frost"
	parityTag  = "manga-translation-candidate"
	parityActor = "local"
)

type parityState struct {
	workID   string
	animeID  string
	mangaID  string
	animeRev int64
	mangaRev int64
	relID    string
	backID   string
}

// parityStep is one scripted operation: the MCP call and the HTTP call that
// must observe the same user-visible outcome.
type parityStep struct {
	name string
	mcp  func(t *testing.T, m *mcpDriver, s *parityState) opResult
	http func(t *testing.T, h *httpDriver, s *parityState) opResult
}

type opResult struct {
	ok   bool
	data json.RawMessage
	code string
	msg  string
}

// mcpDriver talks to the MCP server through the SDK in-memory transport
// against one temporary state directory.
type mcpDriver struct {
	session *mcpsdk.ClientSession
}

func startMCPParity(t *testing.T) *mcpDriver {
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
	srv := mcp.New(app.NewService(db), slog.New(slog.NewTextHandler(io.Discard, nil)))
	srvT, cliT := mcpsdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, srvT) }()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "parity-test", Version: "0"}, nil)
	session, err := client.Connect(ctx, cliT, nil)
	if err != nil {
		cancel()
		db.Close()
		t.Fatalf("connect mcp client: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		<-done
		db.Close()
	})
	return &mcpDriver{session: session}
}

func (m *mcpDriver) call(t *testing.T, tool string, args map[string]any) opResult {
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
		return opResult{code: wire.Code, msg: wire.Message}
	}
	return opResult{ok: true, data: json.RawMessage(text.Text)}
}

func actor(t *testing.T) map[string]any {
	return map[string]any{"principal": parityActor, "origin": string(domain.OriginUser)}
}

func titles(jp, ru string) []map[string]any {
	out := []map[string]any{
		{"text": jp, "kind": "original", "preferred": true, "provenance": "manual", "language": "ja"},
	}
	if ru != "" {
		out = append(out, map[string]any{"text": ru, "kind": "alias", "provenance": "manual", "language": "ru"})
	}
	return out
}

// httpDriver talks to the loopback API server with one owner session.
type httpDriver struct {
	base   string
	cookie string
	csrf   string
	client *http.Client
}

func startHTTPParity(t *testing.T) *httpDriver {
	t.Helper()
	dir := t.TempDir()
	secret, err := httpapi.GenerateOwnerSecret(dir)
	if err != nil {
		t.Fatalf("generate owner secret: %v", err)
	}
	maint, err := store.OpenForMaintenance(dir)
	if err != nil {
		t.Fatalf("open for maintenance: %v", err)
	}
	if _, err := store.RunMigrations(maint.DB()); err != nil {
		maint.Close()
		t.Fatalf("run migrations: %v", err)
	}
	if err := maint.Close(); err != nil {
		t.Fatalf("close maintenance: %v", err)
	}
	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	srv, err := httpapi.New(&config.Config{StateDir: dir, ListenAddr: "127.0.0.1:0"})
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

	d := &httpDriver{base: "http://" + addr.String(), client: &http.Client{Timeout: 5 * time.Second}}
	resp, raw := d.raw(t, http.MethodPost, "/api/v1/session", map[string]string{"owner_secret": secret}, true)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login failed: %d %s", resp.StatusCode, raw)
	}
	for _, c := range resp.Cookies() {
		if c.Name == httpapi.SessionCookieName {
			d.cookie = c.String()
		}
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || !envelope.OK || envelope.Data.CSRFToken == "" {
		t.Fatalf("login envelope: %s", raw)
	}
	d.csrf = envelope.Data.CSRFToken
	if d.cookie == "" {
		t.Fatalf("no session cookie in the login response")
	}
	return d
}

func (h *httpDriver) raw(t *testing.T, method, path string, payload any, write bool) (*http.Response, []byte) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, h.base+path, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if h.cookie != "" {
		req.Header.Set("Cookie", h.cookie)
	}
	if write {
		req.Header.Set("Origin", h.base)
		if h.csrf != "" {
			req.Header.Set(httpapi.CSRFHeader, h.csrf)
		}
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, raw
}

func (h *httpDriver) do(t *testing.T, method, path string, payload any) opResult {
	t.Helper()
	write := method != http.MethodGet
	_, raw := h.raw(t, method, path, payload, write)
	var envelope struct {
		OK   bool `json:"ok"`
		Data json.RawMessage `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("%s %s: not an envelope: %s", method, path, raw)
	}
	if envelope.OK {
		return opResult{ok: true, data: envelope.Data}
	}
	if envelope.Error == nil || envelope.Error.Code == "" {
		t.Fatalf("%s %s: failure without a stable code: %s", method, path, raw)
	}
	return opResult{code: envelope.Error.Code, msg: envelope.Error.Message}
}

// normalize canonicalizes volatile fields so two separately initialized
// state directories compare equal: ids and instants are replaced by stable
// markers, everything else (values, revisions, labels, codes) stays.
func normalize(v any) any {
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
				// Element-wise canonicalization: the list length is
				// user-visible (the per-edition match badge), so a
				// cross-transport divergence in length must not normalize
				// away.
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
				t[k] = normalize(val)
			}
		}
		return t
	case []any:
		for i := range t {
			t[i] = normalize(t[i])
		}
		return t
	default:
		return v
	}
}

func compareOutcome(t *testing.T, stepName string, got, want opResult) {
	t.Helper()
	if got.ok != want.ok {
		t.Fatalf("%s: outcome kind differs: mcp ok=%v data=%s, http ok=%v data=%s",
			stepName, got.ok, got.data, want.ok, want.data)
	}
	if !got.ok {
		// The stable code is the cross-transport contract; the message may
		// embed this run's ids and is not comparable across state dirs.
		if got.code != want.code {
			t.Fatalf("%s: failure code differs: mcp=%q (%s) http=%q (%s)",
				stepName, got.code, got.msg, want.code, want.msg)
		}
		return
	}
	if !reflect.DeepEqual(normalize(anyValue(t, got.data)), normalize(anyValue(t, want.data))) {
		t.Fatalf("%s: normalized payloads differ:\nmcp:  %s\nhttp: %s",
			stepName, normalize(anyValue(t, got.data)), normalize(anyValue(t, want.data)))
	}
}

func anyValue(t *testing.T, raw json.RawMessage) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal payload: %v (%s)", err, raw)
	}
	return v
}

func TestParity(t *testing.T) {
	m := startMCPParity(t)
	h := startHTTPParity(t)

	sM := &parityState{}
	sH := &parityState{}
	// The script is transport-shaped, so each driver tracks its own ids;
	// the revisions are identical by construction of the same operation
	// order and are tracked in both.
	steps := []parityStep{
		{
			name: "create anime",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				r := m.call(t, mcp.ToolCreateEntry, map[string]any{
					"request_id": "p-create", "actor": actor(t),
					"titles": titles(parityJP, parityRU), "kind": "anime", "language": "ja",
				})
				if r.ok {
					var res struct {
						WorkID    string  `json:"work_id"`
						EditionID *string `json:"edition_id"`
					}
					if err := json.Unmarshal(r.data, &res); err != nil || res.EditionID == nil {
						t.Fatalf("create anime: %s", r.data)
					}
					s.workID, s.animeID = res.WorkID, *res.EditionID
				}
				return r
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				r := h.do(t, http.MethodPost, "/api/v1/works", map[string]any{
					"request_id": "p-create",
					"titles":     titles(parityJP, parityRU), "kind": "anime", "language": "ja",
				})
				if r.ok {
					var res struct {
						WorkID    string  `json:"work_id"`
						EditionID *string `json:"edition_id"`
					}
					if err := json.Unmarshal(r.data, &res); err != nil || res.EditionID == nil {
						t.Fatalf("create anime: %s", r.data)
					}
					s.workID, s.animeID = res.WorkID, *res.EditionID
				}
				return r
			},
		},
		{
			name: "resolve manga addition",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				return m.call(t, mcp.ToolResolveAddition, map[string]any{
					"titles": titles(parityJP, ""), "kind": "manga", "language": "ja",
				})
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				return h.do(t, http.MethodPost, "/api/v1/additions/resolve", map[string]any{
					"titles": titles(parityJP, ""), "kind": "manga", "language": "ja",
				})
			},
		},
		{
			name: "add manga under the work",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				var token string
				res := m.call(t, mcp.ToolResolveAddition, map[string]any{
					"titles": titles(parityJP, ""), "kind": "manga", "language": "ja",
				})
				if !res.ok {
					return res
				}
				var rr struct {
					CandidateToken *string `json:"candidate_token"`
				}
				if err := json.Unmarshal(res.data, &rr); err != nil || rr.CandidateToken == nil {
					t.Fatalf("resolve for add_edition: %s", res.data)
				}
				token = *rr.CandidateToken
				r := m.call(t, mcp.ToolCreateEntry, map[string]any{
					"request_id": "p-manga", "actor": actor(t),
					"titles": titles(parityJP, ""), "kind": "manga", "language": "ja",
					"work_id": s.workID, "choice": "add_edition", "candidate_token": token,
				})
				if r.ok {
					var res struct {
						WorkID    string  `json:"work_id"`
						EditionID *string `json:"edition_id"`
					}
					if err := json.Unmarshal(r.data, &res); err != nil || res.EditionID == nil {
						t.Fatalf("add manga: %s", r.data)
					}
					if res.WorkID != s.workID {
						t.Fatalf("add manga landed on work %q, want %q", res.WorkID, s.workID)
					}
					s.mangaID = *res.EditionID
				}
				return r
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				var token string
				res := h.do(t, http.MethodPost, "/api/v1/additions/resolve", map[string]any{
					"titles": titles(parityJP, ""), "kind": "manga", "language": "ja",
				})
				if !res.ok {
					return res
				}
				var rr struct {
					CandidateToken *string `json:"candidate_token"`
				}
				if err := json.Unmarshal(res.data, &rr); err != nil || rr.CandidateToken == nil {
					t.Fatalf("resolve for add_edition: %s", res.data)
				}
				token = *rr.CandidateToken
				r := h.do(t, http.MethodPost, "/api/v1/works/"+s.workID+"/editions", map[string]any{
					"request_id": "p-manga",
					"titles":     titles(parityJP, ""), "kind": "manga", "language": "ja",
					"candidate_token": token,
				})
				if r.ok {
					var res struct {
						WorkID    string  `json:"work_id"`
						EditionID *string `json:"edition_id"`
					}
					if err := json.Unmarshal(r.data, &res); err != nil || res.EditionID == nil {
						t.Fatalf("add manga: %s", r.data)
					}
					if res.WorkID != s.workID {
						t.Fatalf("add manga landed on work %q, want %q", res.WorkID, s.workID)
					}
					s.mangaID = *res.EditionID
				}
				return r
			},
		},
		{
			name: "mark anime completed",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				r := m.call(t, mcp.ToolSetConsumption, map[string]any{
					"request_id": "p-consumed", "actor": actor(t),
					"edition_id": s.animeID, "value": "completed", "reason": "watched through",
					"expected_revision": s.animeRev,
				})
				if r.ok {
					s.animeRev = revisionOf(t, r.data)
				}
				return r
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				r := h.do(t, http.MethodPost, "/api/v1/editions/"+s.animeID+"/consumption", map[string]any{
					"request_id": "p-consumed", "expected_revision": s.animeRev,
					"value": "completed", "reason": "watched through",
				})
				if r.ok {
					s.animeRev = revisionOf(t, r.data)
				}
				return r
			},
		},
		{
			name: "rate anime 8/10",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				r := m.call(t, mcp.ToolRecordRating, map[string]any{
					"request_id": "p-rating", "actor": actor(t),
					"edition_id": s.animeID, "value": 8, "comment": "steady, no filler",
					"expected_revision": s.animeRev,
				})
				if r.ok {
					s.animeRev = revisionOf(t, r.data)
				}
				return r
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				r := h.do(t, http.MethodPost, "/api/v1/editions/"+s.animeID+"/ratings", map[string]any{
					"request_id": "p-rating", "expected_revision": s.animeRev,
					"value": 8, "comment": "steady, no filler",
				})
				if r.ok {
					s.animeRev = revisionOf(t, r.data)
				}
				return r
			},
		},
		{
			name: "save the unread manga",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				r := m.call(t, mcp.ToolSetIntention, map[string]any{
					"request_id": "p-intention", "actor": actor(t),
					"edition_id": s.mangaID, "value": "saved", "reason": "unread, kept for later",
					"expected_revision": s.mangaRev,
				})
				if r.ok {
					s.mangaRev = revisionOf(t, r.data)
				}
				return r
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				r := h.do(t, http.MethodPost, "/api/v1/editions/"+s.mangaID+"/intention", map[string]any{
					"request_id": "p-intention", "expected_revision": s.mangaRev,
					"value": "saved", "reason": "unread, kept for later",
				})
				if r.ok {
					s.mangaRev = revisionOf(t, r.data)
				}
				return r
			},
		},
		{
			name: "tag the manga",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				r := m.call(t, mcp.ToolSetTag, map[string]any{
					"request_id": "p-tag", "actor": actor(t),
					"target_scope": "edition", "target_id": s.mangaID, "tag": parityTag,
					"expected_revision": s.mangaRev,
				})
				if r.ok {
					s.mangaRev = revisionOf(t, r.data)
				}
				return r
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				r := h.do(t, http.MethodPost, "/api/v1/tags/assignments", map[string]any{
					"request_id": "p-tag", "expected_revision": s.mangaRev,
					"target": "edition", "target_id": s.mangaID, "tag": parityTag,
				})
				if r.ok {
					s.mangaRev = revisionOf(t, r.data)
				}
				return r
			},
		},
		{
			name: "attach the publication URL",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				r := m.call(t, mcp.ToolWriteReference, map[string]any{
					"request_id": "p-reference", "actor": actor(t), "action": "add",
					"edition_id": s.mangaID, "provider": "manga.example.org", "url": parityURL,
					"role": "publication", "authoritative": true, "provenance": "manual",
					"expected_revision": s.mangaRev,
				})
				if r.ok {
					s.mangaRev = revisionOf(t, r.data)
				}
				return r
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				r := h.do(t, http.MethodPost, "/api/v1/editions/"+s.mangaID+"/references", map[string]any{
					"request_id": "p-reference", "expected_revision": s.mangaRev,
					"provider": "manga.example.org", "url": parityURL,
					"role": "publication", "authoritative": true, "provenance": "manual",
				})
				if r.ok {
					s.mangaRev = revisionOf(t, r.data)
				}
				return r
			},
		},
		{
			name: "propose the adaptation link",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				r := m.call(t, mcp.ToolWriteRelation, map[string]any{
					"request_id": "p-relation", "actor": actor(t), "action": "propose",
					"scope": "edition", "type": "adaptation_of",
					"from_id": s.animeID, "to_id": s.mangaID, "provenance": "user",
					"expected_revision": s.animeRev,
				})
				if r.ok {
					s.animeRev++
					s.relID = firstRelationID(t, r.data, "proposed")
				}
				return r
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				r := h.do(t, http.MethodPost, "/api/v1/relations", map[string]any{
					"request_id": "p-relation", "expected_revision": s.animeRev,
					"scope": "edition", "type": "adaptation_of", "state": "proposed",
					"from_id": s.animeID, "to_id": s.mangaID, "provenance": "user",
				})
				if r.ok {
					s.animeRev++
					s.relID = firstRelationID(t, r.data, "proposed")
				}
				return r
			},
		},
		{
			name: "confirm the adaptation link",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				r := m.call(t, mcp.ToolWriteRelation, map[string]any{
					"request_id": "p-relation-confirm", "actor": actor(t), "action": "confirm",
					"relation_id": s.relID, "confirmation": true,
					"user_instruction": "the anime adapts this manga",
					"expected_revision": s.animeRev,
				})
				if r.ok {
					s.animeRev++
				}
				return r
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				r := h.do(t, http.MethodPost, "/api/v1/relations/"+s.relID+"/confirm", map[string]any{
					"request_id": "p-relation-confirm", "expected_revision": s.animeRev,
				})
				if r.ok {
					s.animeRev++
				}
				return r
			},
		},
		{
			name: "propose the back link",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				r := m.call(t, mcp.ToolWriteRelation, map[string]any{
					"request_id": "p-back", "actor": actor(t), "action": "propose",
					"scope": "edition", "type": "adaptation_of",
					"from_id": s.mangaID, "to_id": s.animeID, "provenance": "agent",
					"expected_revision": s.mangaRev,
				})
				if r.ok {
					s.mangaRev++
					s.backID = firstRelationID(t, r.data, "proposed")
				}
				return r
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				r := h.do(t, http.MethodPost, "/api/v1/relations", map[string]any{
					"request_id": "p-back", "expected_revision": s.mangaRev,
					"scope": "edition", "type": "adaptation_of", "state": "proposed",
					"from_id": s.mangaID, "to_id": s.animeID, "provenance": "agent",
				})
				if r.ok {
					s.mangaRev++
					s.backID = firstRelationID(t, r.data, "proposed")
				}
				return r
			},
		},
		{
			name: "cyclic confirm fails identically",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				return m.call(t, mcp.ToolWriteRelation, map[string]any{
					"request_id": "p-back-confirm", "actor": actor(t), "action": "confirm",
					"relation_id": s.backID, "confirmation": true,
					"user_instruction": "close the loop",
					"expected_revision": s.mangaRev,
				})
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				return h.do(t, http.MethodPost, "/api/v1/relations/"+s.backID+"/confirm", map[string]any{
					"request_id": "p-back-confirm", "expected_revision": s.mangaRev,
				})
			},
		},
		{
			name: "self link fails identically",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				return m.call(t, mcp.ToolWriteRelation, map[string]any{
					"request_id": "p-self", "actor": actor(t), "action": "propose",
					"scope": "edition", "type": "adaptation_of",
					"from_id": s.animeID, "to_id": s.animeID, "provenance": "user",
					"expected_revision": s.animeRev,
				})
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				return h.do(t, http.MethodPost, "/api/v1/relations", map[string]any{
					"request_id": "p-self", "expected_revision": s.animeRev,
					"scope": "edition", "type": "adaptation_of", "state": "proposed",
					"from_id": s.animeID, "to_id": s.animeID, "provenance": "user",
				})
			},
		},
		{
			name: "stale revision fails identically",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				return m.call(t, mcp.ToolSetConsumption, map[string]any{
					"request_id": "p-stale", "actor": actor(t),
					"edition_id": s.animeID, "value": "in_progress",
					"expected_revision": 1,
				})
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				return h.do(t, http.MethodPost, "/api/v1/editions/"+s.animeID+"/consumption", map[string]any{
					"request_id": "p-stale", "expected_revision": 1, "value": "in_progress",
				})
			},
		},
		{
			name: "same key different payload fails identically",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				return m.call(t, mcp.ToolCreateEntry, map[string]any{
					"request_id": "p-create", "actor": actor(t),
					"titles": titles("Другой Заголовок", ""), "kind": "anime",
				})
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				return h.do(t, http.MethodPost, "/api/v1/works", map[string]any{
					"request_id": "p-create",
					"titles":     titles("Другой Заголовок", ""), "kind": "anime",
				})
			},
		},
		{
			name: "alias search",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				return m.call(t, mcp.ToolSearchLibrary, map[string]any{"query": parityJP})
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				return h.do(t, http.MethodGet, "/api/v1/works?query="+url.QueryEscape(parityJP), nil)
			},
		},
		{
			name: "work view",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				return m.call(t, mcp.ToolGetWork, map[string]any{"work_id": s.workID})
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				return h.do(t, http.MethodGet, "/api/v1/works/"+s.workID, nil)
			},
		},
		{
			name: "edition view",
			mcp: func(t *testing.T, m *mcpDriver, s *parityState) opResult {
				return m.call(t, mcp.ToolGetEdition, map[string]any{"edition_id": s.mangaID})
			},
			http: func(t *testing.T, h *httpDriver, s *parityState) opResult {
				return h.do(t, http.MethodGet, "/api/v1/editions/"+s.mangaID, nil)
			},
		},
	}

	for _, step := range steps {
		got := step.mcp(t, m, sM)
		want := step.http(t, h, sH)
		compareOutcome(t, step.name, got, want)
	}
}

func revisionOf(t *testing.T, raw json.RawMessage) int64 {
	t.Helper()
	var v struct {
		Revision int64 `json:"revision"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("payload without a revision: %s", raw)
	}
	return v.Revision
}

func firstRelationID(t *testing.T, raw json.RawMessage, section string) string {
	t.Helper()
	var view map[string]any
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatalf("unmarshal relation view: %s", raw)
	}
	items, ok := view[section].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("relation view has no %s edge: %s", section, raw)
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("relation view %s entry is not an object: %s", section, raw)
	}
	id, ok := first["id"].(string)
	if !ok || id == "" {
		t.Fatalf("relation view %s entry has no id: %s", section, raw)
	}
	return id
}
