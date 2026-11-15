package store

import (
	"errors"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/importer"
)

// TestReleaseBindingIdempotent proves the explicit map rule: binding the same
// confirmed publication to the same edition is a no-op (the same active row
// is returned), while binding it to a different edition is an explicit map
// conflict, never a silent re-point. A title is never inferred.
func TestReleaseBindingIdempotent(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Release Story")
	e1, w := mustEdition(t, db, now, w, domain.KindManga, "Manga Edition")
	e2, w := mustEdition(t, db, now, w, domain.KindWebNovel, "Web Novel Edition")

	first, err := db.BindReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_1", e1.ID, "tester", now)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	if first.State != BindingStateActive || first.EditionID != e1.ID {
		t.Fatalf("first binding = %+v, want active for e1", first)
	}

	replay, err := db.BindReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_1", e1.ID, "tester", now.Add(time.Second))
	if err != nil {
		t.Fatalf("rebind no-op: %v", err)
	}
	if replay.ID != first.ID {
		t.Fatalf("replay binding id = %s, want the same active row %s", replay.ID, first.ID)
	}

	_, err = db.BindReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_1", e2.ID, "tester", now.Add(2*time.Second))
	if !errors.Is(err, ErrBindingConflict) {
		t.Fatalf("bind to a different edition: err = %v, want ErrBindingConflict", err)
	}
}

// TestReleaseBindingRebind proves a correction: rebind revokes the prior
// active binding (recording who and when) and creates its replacement linked
// by replaces_id, so the old binding history is retained and exactly one
// active binding remains.
func TestReleaseBindingRebind(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Release Story")
	e1, w := mustEdition(t, db, now, w, domain.KindManga, "Manga Edition")
	e2, w := mustEdition(t, db, now, w, domain.KindWebNovel, "Web Novel Edition")

	old, err := db.BindReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_2", e1.ID, "tester", now)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	newB, err := db.RebindReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_2", e2.ID, "corrector", now.Add(time.Second))
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if newB.EditionID != e2.ID || newB.ReplacesID == nil || *newB.ReplacesID != old.ID {
		t.Fatalf("replacement = %+v, want active for e2 replacing %s", newB, old.ID)
	}

	active, err := db.GetReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_2")
	if err != nil {
		t.Fatalf("GetReleaseBinding: %v", err)
	}
	if active == nil || active.ID != newB.ID {
		t.Fatalf("active binding = %+v, want the replacement", active)
	}

	history, err := db.ListReleaseBindings(ctx(t), importer.DefaultNamespace, "pub_2")
	if err != nil {
		t.Fatalf("ListReleaseBindings: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("binding history = %d rows, want 2 (revoked predecessor + active replacement)", len(history))
	}
	// The predecessor is revoked with the correcting actor and instant.
	revoked := history[0]
	if revoked.State != BindingStateRevoked || revoked.RevokedBy == nil || *revoked.RevokedBy != "corrector" || revoked.RevokedAt == nil {
		t.Fatalf("predecessor = %+v, want revoked by the corrector with an instant", revoked)
	}
}

// TestReleaseBindingRevoke proves revoking un-projects a publication: the
// active binding is revoked, GetReleaseBinding reports none, and revoking
// again is a distinguishable not-found, never a silent success.
func TestReleaseBindingRevoke(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Release Story")
	e, _ := mustEdition(t, db, now, w, domain.KindManga, "Manga Edition")

	if _, err := db.BindReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_3", e.ID, "tester", now); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if err := db.RevokeReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_3", "tester", now.Add(time.Second)); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	active, err := db.GetReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_3")
	if err != nil {
		t.Fatalf("GetReleaseBinding: %v", err)
	}
	if active != nil {
		t.Fatalf("active binding = %+v, want none after revoke", active)
	}
	if err := db.RevokeReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_3", "tester", now.Add(2*time.Second)); !errors.Is(err, ErrBindingNotFound) {
		t.Fatalf("second revoke: err = %v, want ErrBindingNotFound", err)
	}
}

// TestReleaseProjectionList proves the derived release history of an edition:
// the imported events of every actively bound publication, ordered
// (created_at, event_id), keyset-paginated, each row carrying the target work
// and the producer's labeled counts (never summed).
func TestReleaseProjectionList(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	base := libraryNow()
	w := mustWork(t, db, base, "Release Story")
	e, _ := mustEdition(t, db, base, w, domain.KindManga, "Manga Edition")

	countA := 42
	countB := 43
	mustImportEvent(t, db, EventInput{
		EventID: "evt_r1", SchemaVersion: "1.0", EventType: "release.added",
		RawPayload: `{"event_id":"evt_r1"}`, Canonical: `{"event_id":"evt_r1"}`,
		Namespace: importer.DefaultNamespace, PublicationID: strPtr("pub_r"),
		ReleaseNumber: strPtr("12"), ReleaseTitle: strPtr("Chapter 12"), CurrentCount: &countA, CreatedAt: base,
	})
	mustImportEvent(t, db, EventInput{
		EventID: "evt_r2", SchemaVersion: "1.0", EventType: "release.count_changed",
		RawPayload: `{"event_id":"evt_r2"}`, Canonical: `{"event_id":"evt_r2"}`,
		Namespace: importer.DefaultNamespace, PublicationID: strPtr("pub_r"),
		ReleaseNumber: strPtr("13"), CurrentCount: &countB, CreatedAt: base.Add(time.Second),
	})
	if _, err := db.BindReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_r", e.ID, "tester", base.Add(2*time.Second)); err != nil {
		t.Fatalf("bind: %v", err)
	}

	page, total, next, err := db.ListReleaseProjections(ctx(t), e.ID, 1, nil)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if total != 2 || len(page) != 1 || next == nil {
		t.Fatalf("page 1: total=%d len=%d next=%v, want 2/1/cursor", total, len(page), next)
	}
	got := page[0]
	if got.EventID != "evt_r1" || got.WorkID != w.ID || got.EditionID != e.ID {
		t.Fatalf("first projection = %+v, want evt_r1 on work %s edition %s", got, w.ID, e.ID)
	}
	if got.PublicationID == nil || *got.PublicationID != "pub_r" {
		t.Fatalf("projection publication = %v, want pub_r", got.PublicationID)
	}
	if got.CurrentCount == nil || *got.CurrentCount != 42 {
		t.Fatalf("projection count = %v, want 42 (the producer's labeled value)", got.CurrentCount)
	}
	page2, _, next2, err := db.ListReleaseProjections(ctx(t), e.ID, 1, next)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page2) != 1 || page2[0].EventID != "evt_r2" || next2 != nil {
		t.Fatalf("page 2 = %+v next=%v, want the single count-changed row", page2, next2)
	}
}

// TestReleaseProjectionRebindNoDuplicate proves a correction: after a rebind
// the events project under the new edition only, the old edition's
// projection is empty, and no duplicate active projection is left behind.
// The raw events and the old binding history stay intact.
func TestReleaseProjectionRebindNoDuplicate(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Release Story")
	e1, w := mustEdition(t, db, now, w, domain.KindManga, "Manga Edition")
	e2, w := mustEdition(t, db, now, w, domain.KindWebNovel, "Web Novel Edition")

	mustImportEvent(t, db, sampleEventInput("evt_rb", "release.added", strPtr("pub_rb"), now))
	if _, err := db.BindReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_rb", e1.ID, "tester", now.Add(time.Second)); err != nil {
		t.Fatalf("bind: %v", err)
	}
	// Before the correction the event projects under e1.
	if rows, _, _, err := db.ListReleaseProjections(ctx(t), e1.ID, 50, nil); err != nil || len(rows) != 1 {
		t.Fatalf("pre-rebind e1 projections: rows=%d err=%v, want 1", len(rows), err)
	}
	if _, err := db.RebindReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_rb", e2.ID, "corrector", now.Add(2*time.Second)); err != nil {
		t.Fatalf("rebind: %v", err)
	}
	// After the correction: e2 projects it, e1 no longer does.
	got2, total2, _, err := db.ListReleaseProjections(ctx(t), e2.ID, 50, nil)
	if err != nil || total2 != 1 || len(got2) != 1 || got2[0].EditionID != e2.ID {
		t.Fatalf("post-rebind e2 projections: total=%d rows=%d err=%v, want exactly one on e2", total2, len(got2), err)
	}
	got1, total1, _, err := db.ListReleaseProjections(ctx(t), e1.ID, 50, nil)
	if err != nil || total1 != 0 || len(got1) != 0 {
		t.Fatalf("post-rebind e1 projections: total=%d rows=%d, want none (no duplicate active projection)", total1, len(got1))
	}
	// The raw event and the old binding history are preserved.
	if stored, err := findEvent(ctx(t), db.DB(), "evt_rb"); err != nil || stored == nil {
		t.Fatalf("raw event: stored=%v err=%v, want the immutable row intact", stored, err)
	}
	if history, err := db.ListReleaseBindings(ctx(t), importer.DefaultNamespace, "pub_rb"); err != nil || len(history) != 2 {
		t.Fatalf("binding history: rows=%d err=%v, want the revoked predecessor and the replacement", len(history), err)
	}
}

// TestReleaseActivityImportedRelease proves the activity projection includes
// an explicitly labeled imported-release row only for an actively bound
// event, and drops it when the binding is revoked.
func TestReleaseActivityImportedRelease(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Release Story")
	e, _ := mustEdition(t, db, now, w, domain.KindManga, "Manga Edition")

	mustImportEvent(t, db, sampleEventInput("evt_act", "release.added", strPtr("pub_act"), now))
	// Unbound: no imported-release row.
	if rows, _, err := db.RecentActivity(ctx(t), ActivityQuery{Types: []ActivityType{ActivityImportedRelease}}); err != nil || len(rows) != 0 {
		t.Fatalf("unbound activity: rows=%d err=%v, want none", len(rows), err)
	}
	if _, err := db.BindReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_act", e.ID, "tester", now.Add(time.Second)); err != nil {
		t.Fatalf("bind: %v", err)
	}
	rows, _, err := db.RecentActivity(ctx(t), ActivityQuery{Types: []ActivityType{ActivityImportedRelease}})
	if err != nil {
		t.Fatalf("RecentActivity: %v", err)
	}
	if len(rows) != 1 || rows[0].Type != ActivityImportedRelease || rows[0].EditionID == nil || *rows[0].EditionID != e.ID {
		t.Fatalf("bound activity = %+v, want one imported_release row on the edition", rows)
	}
	if rows[0].Label != "imported" {
		t.Fatalf("activity label = %q, want imported (explicitly labeled)", rows[0].Label)
	}
	if err := db.RevokeReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_act", "tester", now.Add(2*time.Second)); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if rows, _, err := db.RecentActivity(ctx(t), ActivityQuery{Types: []ActivityType{ActivityImportedRelease}}); err != nil || len(rows) != 0 {
		t.Fatalf("revoked activity: rows=%d err=%v, want none", len(rows), err)
	}
}

// TestReleaseActivityOpaqueSourceTime proves the review fix: an imported
// release whose producer source_time is not a parseable RFC3339 instant
// degrades to an absent observed_at instead of failing the whole activity
// feed, while a well-formed source_time is still parsed and kept.
func TestReleaseActivityOpaqueSourceTime(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Release Story")
	e, _ := mustEdition(t, db, now, w, domain.KindManga, "Manga Edition")

	mustImportEvent(t, db, EventInput{
		EventID: "evt_opaque", SchemaVersion: "1.0", EventType: "release.added",
		RawPayload: `{"event_id":"evt_opaque"}`, Canonical: `{"event_id":"evt_opaque"}`,
		Namespace: importer.DefaultNamespace, PublicationID: strPtr("pub_opaque"),
		ReleaseNumber: strPtr("12"), ReleaseTitle: strPtr("Chapter 12"),
		SourceTime: strPtr("14/11/2026 10:00"), CreatedAt: now,
	})
	well := "2026-11-14T10:00:00Z"
	mustImportEvent(t, db, EventInput{
		EventID: "evt_well", SchemaVersion: "1.0", EventType: "release.added",
		RawPayload: `{"event_id":"evt_well"}`, Canonical: `{"event_id":"evt_well"}`,
		Namespace: importer.DefaultNamespace, PublicationID: strPtr("pub_well"),
		ReleaseNumber: strPtr("13"), ReleaseTitle: strPtr("Chapter 13"),
		SourceTime: &well, CreatedAt: now.Add(time.Second),
	})
	if _, err := db.BindReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_opaque", e.ID, "tester", now.Add(2*time.Second)); err != nil {
		t.Fatalf("bind opaque: %v", err)
	}
	if _, err := db.BindReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_well", e.ID, "tester", now.Add(3*time.Second)); err != nil {
		t.Fatalf("bind well: %v", err)
	}

	// The whole feed (every type) must not fail on the opaque timestamp.
	all, _, err := db.RecentActivity(ctx(t), ActivityQuery{})
	if err != nil {
		t.Fatalf("RecentActivity(all): %v, want no error (an opaque producer source_time must not break the feed)", err)
	}
	var opaque, wellRow ActivityRow
	for _, r := range all {
		if r.Type != ActivityImportedRelease || r.Value == nil {
			continue
		}
		switch *r.Value {
		case "12":
			opaque = r
		case "13":
			wellRow = r
		}
	}
	if opaque.ID == "" {
		t.Fatalf("missing the opaque imported_release row in %+v", all)
	}
	if opaque.ObservedAt != nil {
		t.Fatalf("opaque observed_at = %v, want nil (unparseable producer source_time degrades to absent)", *opaque.ObservedAt)
	}
	if wellRow.ID == "" {
		t.Fatalf("missing the well-formed imported_release row in %+v", all)
	}
	if wellRow.ObservedAt == nil {
		t.Fatalf("well-formed observed_at = nil, want the parsed producer source_time")
	}
}

// TestReleaseRebindSameEditionNoOp proves the review fix: rebinding a
// publication to the edition it is already bound to is an idempotent no-op
// (the same active row is returned, no duplicate active row is inserted),
// not a storage error.
func TestReleaseRebindSameEditionNoOp(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Release Story")
	e, _ := mustEdition(t, db, now, w, domain.KindManga, "Manga Edition")

	first, err := db.BindReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_same", e.ID, "tester", now)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	again, err := db.RebindReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_same", e.ID, "corrector", now.Add(time.Second))
	if err != nil {
		t.Fatalf("rebind to the same edition: %v, want an idempotent no-op, not a storage error", err)
	}
	if again.ID != first.ID {
		t.Fatalf("rebind no-op id = %s, want the existing active row %s", again.ID, first.ID)
	}
	active, err := db.GetReleaseBinding(ctx(t), importer.DefaultNamespace, "pub_same")
	if err != nil {
		t.Fatalf("GetReleaseBinding: %v", err)
	}
	if active == nil || active.ID != first.ID {
		t.Fatalf("active binding = %+v, want the original row", active)
	}
	history, err := db.ListReleaseBindings(ctx(t), importer.DefaultNamespace, "pub_same")
	if err != nil {
		t.Fatalf("ListReleaseBindings: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("binding history = %d rows, want 1 (no duplicate active row)", len(history))
	}
}
