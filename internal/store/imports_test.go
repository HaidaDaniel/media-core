package store

import (
	"errors"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/importer"
)

func mustImportEvent(t *testing.T, db *Database, in EventInput) EventOutcome {
	t.Helper()
	out, err := db.InsertEvent(ctx(t), in)
	if err != nil {
		t.Fatalf("InsertEvent(%s): %v", in.EventID, err)
	}
	return out
}

func sampleEventInput(eventID, eventType string, publicationID *string, now time.Time) EventInput {
	count := 12
	return EventInput{
		EventID:       eventID,
		SchemaVersion: "1.0",
		EventType:     eventType,
		RawPayload:    `{"event_id":"` + eventID + `","event_type":"` + eventType + `"}`,
		Canonical:     `{"event_id":"` + eventID + `","event_type":"` + eventType + `"}`,
		Namespace:     importer.DefaultNamespace,
		PublicationID: publicationID,
		ReleaseTitle:  strPtr("Chapter 12"),
		CurrentCount:  &count,
		CreatedAt:     now,
	}
}

// TestImportEventIdempotent proves the idempotency rule at the store: a new
// event_id inserts, the same event_id with an identical canonical payload is
// a no-op replay, and the same event_id with a different payload is an
// identity conflict that never overwrites the stored row.
func TestImportEventIdempotent(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()

	first := mustImportEvent(t, db, sampleEventInput("evt_1", "release.added", strPtr("pub_1"), now))
	if first.Status != EventStatusImported {
		t.Fatalf("first insert status = %q, want imported", first.Status)
	}

	// Identical replay: a no-op.
	replay := mustImportEvent(t, db, sampleEventInput("evt_1", "release.added", strPtr("pub_1"), now.Add(time.Second)))
	if replay.Status != EventStatusReplayed {
		t.Fatalf("replay status = %q, want replayed", replay.Status)
	}

	// Same event_id, different payload: an identity conflict, no overwrite.
	conflict := sampleEventInput("evt_1", "release.added", strPtr("pub_1"), now.Add(2*time.Second))
	conflict.Canonical = `{"event_id":"evt_1","event_type":"release.added","release_title":"Chapter 12 (corrected)"}`
	conflict.RawPayload = conflict.Canonical
	_, err := db.InsertEvent(ctx(t), conflict)
	if !errors.Is(err, ErrEventConflict) {
		t.Fatalf("conflict error = %v, want ErrEventConflict", err)
	}

	// The stored row is unchanged: the original canonical survives.
	stored, err := findEvent(ctx(t), db.DB(), "evt_1")
	if err != nil {
		t.Fatalf("findEvent: %v", err)
	}
	if stored == nil {
		t.Fatal("stored event is missing")
	}
	if stored.Canonical != sampleEventInput("evt_1", "release.added", nil, now).Canonical {
		t.Fatalf("stored canonical = %q, want the original (never overwritten)", stored.Canonical)
	}
}

// TestImportEventPreservesRawAndTyped proves the raw payload (including
// unknown additive fields) and the typed projection are both preserved.
func TestImportEventPreservesRawAndTyped(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	raw := `{"schema_version":"1.0","event_id":"evt_9","event_type":"release.added","publication_id":"pub_9","release_title":"Ch 9","producer_extra":{"score":0.9},"tags":["scanlation"]}`
	canonical, err := importer.CanonicalJSON(mustRawJSON(t, raw))
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	in := EventInput{
		EventID:       "evt_9",
		SchemaVersion: "1.0",
		EventType:     "release.added",
		RawPayload:    raw,
		Canonical:     canonical,
		Namespace:     importer.DefaultNamespace,
		PublicationID: strPtr("pub_9"),
		ReleaseTitle:  strPtr("Ch 9"),
		CreatedAt:     now,
	}
	mustImportEvent(t, db, in)
	stored, err := findEvent(ctx(t), db.DB(), "evt_9")
	if err != nil || stored == nil {
		t.Fatalf("findEvent: stored=%v err=%v", stored, err)
	}
	if stored.RawPayload != raw {
		t.Fatalf("raw payload = %q, want the exact source payload", stored.RawPayload)
	}
	if stored.Canonical != canonical {
		t.Fatalf("canonical = %q, want %q", stored.Canonical, canonical)
	}
	if stored.PublicationID == nil || *stored.PublicationID != "pub_9" {
		t.Fatalf("publication_id = %v, want pub_9", stored.PublicationID)
	}
	if stored.ReleaseTitle == nil || *stored.ReleaseTitle != "Ch 9" {
		t.Fatalf("release_title = %v, want Ch 9", stored.ReleaseTitle)
	}
}

// TestImportPendingList proves the pending queue: every imported event is
// pending until bound, listed oldest first with a stable total, and
// keyset-paginated on (created_at, event_id).
func TestImportPendingList(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	base := libraryNow()
	for i, id := range []string{"evt_a", "evt_b", "evt_c", "evt_d"} {
		mustImportEvent(t, db, sampleEventInput(id, "release.added", strPtr("pub_"+id), base.Add(time.Duration(i)*time.Second)))
	}
	// Page 1: two rows, a next cursor.
	page1, total, next, err := db.ListPendingEvents(ctx(t), 2, nil)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if total != 4 || len(page1) != 2 {
		t.Fatalf("page 1: total=%d len=%d, want 4/2", total, len(page1))
	}
	if page1[0].EventID != "evt_a" || page1[1].EventID != "evt_b" {
		t.Fatalf("page 1 order = %v, want evt_a then evt_b", []string{page1[0].EventID, page1[1].EventID})
	}
	if next == nil {
		t.Fatal("page 1 must have a next cursor")
	}
	// Page 2: resumes after evt_b.
	page2, _, next2, err := db.ListPendingEvents(ctx(t), 2, next)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page2) != 2 || page2[0].EventID != "evt_c" || page2[1].EventID != "evt_d" {
		t.Fatalf("page 2 = %v, want evt_c then evt_d", []string{page2[0].EventID, page2[1].EventID})
	}
	if next2 != nil {
		t.Fatalf("page 2 next cursor = %v, want none", *next2)
	}
	// Seeking past the last row is empty.
	last := "2026-11-12T10:00:03.000Z|evt_d"
	page3, _, _, err := db.ListPendingEvents(ctx(t), 2, &last)
	if err != nil {
		t.Fatalf("seek past last: %v", err)
	}
	if len(page3) != 0 {
		t.Fatalf("seek past last = %d rows, want 0", len(page3))
	}
}

// TestImportPendingExcludesBound proves an event with an active binding for its
// (namespace, publication_id) leaves the pending queue, and an event without
// a publication id is always pending.
func TestImportPendingExcludesBound(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Bound Story")
	e, _ := mustEdition(t, db, now, w, domain.KindManga, "Bound Edition")

	mustImportEvent(t, db, sampleEventInput("evt_bound", "release.added", strPtr("pub_bound"), now))
	mustImportEvent(t, db, sampleEventInput("evt_unbound", "release.added", strPtr("pub_unbound"), now.Add(time.Second)))
	mustImportEvent(t, db, sampleEventInput("evt_nopub", "release.added", nil, now.Add(2*time.Second)))

	// Bind the bound event's publication to the edition.
	insertActiveBinding(t, db, importer.DefaultNamespace, "pub_bound", e.ID, now.Add(3*time.Second))

	bound, err := db.HasActiveBinding(ctx(t), importer.DefaultNamespace, "pub_bound")
	if err != nil {
		t.Fatalf("HasActiveBinding: %v", err)
	}
	if !bound {
		t.Fatal("pub_bound must have an active binding")
	}

	pending, total, _, err := db.ListPendingEvents(ctx(t), 50, nil)
	if err != nil {
		t.Fatalf("ListPendingEvents: %v", err)
	}
	if total != 2 {
		t.Fatalf("pending total = %d, want 2 (evt_unbound, evt_nopub)", total)
	}
	ids := make([]string, 0, len(pending))
	for _, p := range pending {
		ids = append(ids, p.EventID)
	}
	if len(ids) != 2 || ids[0] != "evt_unbound" || ids[1] != "evt_nopub" {
		t.Fatalf("pending = %v, want evt_unbound then evt_nopub (evt_bound excluded)", ids)
	}
}

// TestImportPendingListNullPublicationCursor proves keyset pagination is
// applied to events without a publication id (the always-pending class): a
// cursor must resume after the last served row, never re-serve it.
func TestImportPendingListNullPublicationCursor(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	base := libraryNow()
	for i, id := range []string{"evt_n1", "evt_n2", "evt_n3"} {
		mustImportEvent(t, db, sampleEventInput(id, "release.added", nil, base.Add(time.Duration(i)*time.Second)))
	}
	page1, total, next, err := db.ListPendingEvents(ctx(t), 2, nil)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if total != 3 || len(page1) != 2 || next == nil {
		t.Fatalf("page 1: total=%d len=%d next=%v, want 3/2/cursor", total, len(page1), next)
	}
	page2, _, next2, err := db.ListPendingEvents(ctx(t), 2, next)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page2) != 1 || page2[0].EventID != "evt_n3" {
		t.Fatalf("page 2 = %v, want the single row after the cursor (no re-served row)", []string{page2[0].EventID})
	}
	if next2 != nil {
		t.Fatalf("page 2 next = %v, want none", *next2)
	}
}

// insertActiveBinding inserts one active binding row directly; binding
// creation is the next task, the store table and pending exclusion are here.
func insertActiveBinding(t *testing.T, db *Database, namespace, publicationID, editionID string, now time.Time) {
	t.Helper()
	id, err := domain.NewUUID()
	if err != nil {
		t.Fatalf("NewUUID: %v", err)
	}
	if _, err := db.DB().Exec(
		`INSERT INTO import_bindings (id, namespace, publication_id, edition_id, state, created_by, created_at)
		 VALUES (?, ?, ?, ?, 'active', 'test', ?)`,
		id, namespace, publicationID, editionID, instantUTC(now)); err != nil {
		t.Fatalf("insert binding: %v", err)
	}
}

func mustRawJSON(t *testing.T, s string) []byte {
	t.Helper()
	return []byte(s)
}
