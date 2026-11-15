package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

const importFixtureDir = "../../tests/contracts/release-watcher"

func loadImportFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(importFixtureDir, name))
	if err != nil {
		t.Fatalf("load fixture %s: %v", name, err)
	}
	return b
}

func importReq(raw []byte) ImportRequest {
	return ImportRequest{
		RequestID: "import-test",
		Namespace: "release-watcher",
		Actor:     Actor{Principal: "local", Origin: domain.OriginUser},
		Raw:       raw,
	}
}

// TestImportEventsIdempotent proves the idempotency rule through the
// service: a new event imports, a canonical-equal replay is a no-op, and a
// same-id different-payload is a per-item conflict that never overwrites.
func TestImportEventsIdempotent(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	first, err := s.ImportEvents(ctx, importReq(loadImportFixture(t, "envelope_valid.json")))
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if first.Imported != 1 || first.Replayed != 0 || first.Conflicts != 0 {
		t.Fatalf("first = %+v, want 1 imported", first)
	}
	if len(first.Items) != 1 || first.Items[0].Status != ImportStatusImported {
		t.Fatalf("first items = %+v, want one imported", first.Items)
	}
	if !first.Items[0].Pending {
		t.Fatal("first import must be pending (no binding yet)")
	}

	replay, err := s.ImportEvents(ctx, importReq(loadImportFixture(t, "envelope_replay.json")))
	if err != nil {
		t.Fatalf("replay import: %v", err)
	}
	if replay.Replayed != 1 || replay.Imported != 0 {
		t.Fatalf("replay = %+v, want 1 replayed", replay)
	}
	if replay.Items[0].Status != ImportStatusReplayed {
		t.Fatalf("replay status = %q, want replayed", replay.Items[0].Status)
	}

	conflict, err := s.ImportEvents(ctx, importReq(loadImportFixture(t, "envelope_conflict.json")))
	if err != nil {
		t.Fatalf("conflict import: %v", err)
	}
	if conflict.Conflicts != 1 || conflict.Imported != 0 || conflict.Replayed != 0 {
		t.Fatalf("conflict = %+v, want 1 conflict", conflict)
	}
	if conflict.Items[0].Status != ImportStatusConflict {
		t.Fatalf("conflict status = %q, want conflict", conflict.Items[0].Status)
	}
}

// TestImportEventsUnsupportedMajorWritesNothing proves an unsupported schema
// major is rejected before any write: an error is returned and the pending
// queue stays empty.
func TestImportEventsUnsupportedMajorWritesNothing(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()

	_, err := s.ImportEvents(ctx, importReq(loadImportFixture(t, "envelope_unsupported_major.json")))
	if err == nil {
		t.Fatal("unsupported major: want an error")
	}
	var ae *AppError
	if !errors.As(err, &ae) || ae.Code != ErrInvalidArgument {
		t.Fatalf("error = %v, want invalid_argument", err)
	}
	// Nothing was written.
	payload, perr := s.PendingImports(ctx, PendingImportQuery{Limit: 50})
	if perr != nil {
		t.Fatalf("PendingImports: %v", perr)
	}
	if payload.Total != 0 || len(payload.Items) != 0 {
		t.Fatalf("pending after rejected import = %+v, want empty", payload)
	}
}

// TestImportEventsUnknownTypeKeptPending proves an unknown event type is
// imported (not projected) and kept pending.
func TestImportEventsUnknownTypeKeptPending(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	res, err := s.ImportEvents(ctx, importReq(loadImportFixture(t, "envelope_unknown_type.json")))
	if err != nil {
		t.Fatalf("import unknown type: %v", err)
	}
	if res.Imported != 1 {
		t.Fatalf("import = %+v, want 1 imported", res)
	}
	if res.Items[0].EventType != "release.flagged" {
		t.Fatalf("event_type = %q, want release.flagged", res.Items[0].EventType)
	}
	if !res.Items[0].Pending {
		t.Fatal("unknown type must stay pending")
	}
}

// TestImportEventsUnknownFieldsPreserved proves additive fields are imported
// and preserved in the raw payload.
func TestImportEventsUnknownFieldsPreserved(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	res, err := s.ImportEvents(ctx, importReq(loadImportFixture(t, "envelope_unknown_fields.json")))
	if err != nil {
		t.Fatalf("import unknown fields: %v", err)
	}
	if res.Imported != 1 {
		t.Fatalf("import = %+v, want 1 imported", res)
	}
	payload, perr := s.PendingImports(ctx, PendingImportQuery{Limit: 50})
	if perr != nil {
		t.Fatalf("PendingImports: %v", perr)
	}
	if payload.Total != 1 || len(payload.Items) != 1 {
		t.Fatalf("pending = %+v, want one event", payload)
	}
	p, ok := payload.Items[0].(PendingImport)
	if !ok {
		t.Fatalf("pending item type = %T, want PendingImport", payload.Items[0])
	}
	if p.ReleaseID == nil || *p.ReleaseID != "rel_9012" {
		t.Fatalf("release_id = %v, want rel_9012", p.ReleaseID)
	}
}

// TestImportPendingMalformedCursor proves a malformed cursor is a caller
// input error (invalid_argument), not a storage failure.
func TestImportPendingMalformedCursor(t *testing.T) {
	s := newTestService(t)
	bad := "no-pipe-here"
	_, err := s.PendingImports(context.Background(), PendingImportQuery{Limit: 10, Cursor: &bad})
	if err == nil {
		t.Fatal("malformed cursor: want an error")
	}
	var ae *AppError
	if !errors.As(err, &ae) || ae.Code != ErrInvalidArgument {
		t.Fatalf("error = %v, want invalid_argument", err)
	}
}

// TestImportPendingPagination proves the pending queue lists imported
// events oldest first with a stable total and keyset pagination.
func TestImportPendingPagination(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	// Two separate envelopes with distinct event ids.
	if _, err := s.ImportEvents(ctx, importReq(loadImportFixture(t, "envelope_valid.json"))); err != nil {
		t.Fatalf("import 1: %v", err)
	}
	if _, err := s.ImportEvents(ctx, importReq(loadImportFixture(t, "envelope_unknown_fields.json"))); err != nil {
		t.Fatalf("import 2: %v", err)
	}
	payload, err := s.PendingImports(ctx, PendingImportQuery{Limit: 1})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if payload.Total != 2 || len(payload.Items) != 1 || payload.NextCursor == nil {
		t.Fatalf("page 1 = %+v, want total 2, one item, a next cursor", payload)
	}
	page2, err := s.PendingImports(ctx, PendingImportQuery{Limit: 1, Cursor: payload.NextCursor})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page2.Items) != 1 || page2.NextCursor != nil {
		t.Fatalf("page 2 = %+v, want one item and no next cursor", page2)
	}
	// The two pages are distinct events.
	first := payload.Items[0].(PendingImport)
	second := page2.Items[0].(PendingImport)
	if first.EventID == second.EventID {
		t.Fatalf("pages repeated the same event %q", first.EventID)
	}
}
