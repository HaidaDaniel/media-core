// Scenario E acceptance: Release Watcher is separate from reading. A pinned
// schema 1.0 event is imported and left pending, explicitly bound to a known
// edition, replayed (one projection, no duplicate), reused with a conflicting
// payload (a conflict, no replacement), and corrected by an explicit rebind
// that preserves the raw payload and the old binding history. The import and
// the binding never touch personal chapter read or intention.
package acceptance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func openService(t *testing.T) (*app.Service, *store.Database) {
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
	return app.NewService(db), db
}

func seedWorkEdition(t *testing.T, db *store.Database, now time.Time, text string, kind domain.EditionKind) (string, string) {
	t.Helper()
	w, err := db.CreateWork(context.Background(), now, []store.TitleInput{{Text: text, Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	e, err := db.AddEdition(context.Background(), now, w.ID, w.Revision, store.EditionInput{Kind: kind})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	return w.ID, e.ID
}

func readEnvelope(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "contracts", "release-watcher", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// TestScenarioE drives the release-binding vertical slice through the shared
// service end to end and asserts the Scenario E contract.
func TestScenarioE(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	svc, db := openService(t)

	workID, e1 := seedWorkEdition(t, db, now, "Scenario E Manga", domain.KindManga)
	// A second edition on the same work to rebind onto.
	w, err := db.GetWork(ctx, workID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	e2, err := db.AddEdition(ctx, now, workID, w.Revision, store.EditionInput{Kind: domain.KindWebNovel})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	e2ID := e2.ID

	importer := app.Actor{Principal: "acceptance-importer", Origin: domain.OriginImport}
	user := app.Actor{Principal: "acceptance-user", Origin: domain.OriginUser}
	publication := "pub_mu_1234"
	var zero int64

	// Record a personal intention on the first edition before any import or
	// binding; it must stay unchanged through the whole scenario.
	if _, err := svc.SetIntention(ctx, app.SetIntentionRequest{
		MutationBase: app.MutationBase{RequestID: "int-1", Actor: user, ExpectedRevision: &zero},
		EditionID:    e1,
		Value:        string(domain.IntentionPlanned),
	}); err != nil {
		t.Fatalf("SetIntention: %v", err)
	}

	// 1. Import the pinned schema 1.0 envelope: it stages one pending event.
	res, err := svc.ImportEvents(ctx, app.ImportRequest{RequestID: "imp-1", Actor: importer, Raw: readEnvelope(t, "envelope_valid.json")})
	if err != nil {
		t.Fatalf("ImportEvents: %v", err)
	}
	if res.Imported != 1 || res.Replayed != 0 || res.Conflicts != 0 {
		t.Fatalf("first import = %+v, want one imported", res)
	}
	pending, err := svc.PendingImports(ctx, app.PendingImportQuery{})
	if err != nil {
		t.Fatalf("PendingImports: %v", err)
	}
	if pending.Total != 1 {
		t.Fatalf("pending total = %d, want 1 (the event stays pending until bound)", pending.Total)
	}

	// 2. Explicitly bind the confirmed publication to the known edition.
	bound, err := svc.BindRelease(ctx, app.BindReleaseRequest{
		MutationBase:  app.MutationBase{RequestID: "bind-1", Actor: importer},
		PublicationID: publication,
		EditionID:     e1,
	})
	if err != nil {
		t.Fatalf("BindRelease: %v", err)
	}
	if bound.State != "active" || bound.EditionID != e1 {
		t.Fatalf("binding = %+v, want active on the first edition", bound)
	}

	// 3. The bound event leaves the queue and projects exactly once.
	pending, err = svc.PendingImports(ctx, app.PendingImportQuery{})
	if err != nil {
		t.Fatalf("PendingImports: %v", err)
	}
	if pending.Total != 0 {
		t.Fatalf("pending total after bind = %d, want 0", pending.Total)
	}
	rel, err := svc.ListReleases(ctx, app.ReleaseQuery{EditionID: e1})
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if rel.Total != 1 {
		t.Fatalf("projections for e1 = %d, want 1", rel.Total)
	}
	first, ok := rel.Items[0].(app.ReleaseProjectionView)
	if !ok {
		t.Fatalf("projection item type = %T, want ReleaseProjectionView", rel.Items[0])
	}
	if first.PublicationID == nil || *first.PublicationID != publication {
		t.Fatalf("projection not labeled by its publication: %+v", first.PublicationID)
	}

	// 4. Replaying the same envelope is a no-op; the projection stays one.
	res, err = svc.ImportEvents(ctx, app.ImportRequest{RequestID: "imp-replay", Actor: importer, Raw: readEnvelope(t, "envelope_valid.json")})
	if err != nil {
		t.Fatalf("replay ImportEvents: %v", err)
	}
	if res.Replayed != 1 || res.Imported != 0 {
		t.Fatalf("replay = %+v, want one replayed", res)
	}
	rel, err = svc.ListReleases(ctx, app.ReleaseQuery{EditionID: e1})
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if rel.Total != 1 {
		t.Fatalf("projections after replay = %d, want 1 (no duplicate)", rel.Total)
	}

	// 5. Reusing the event id with a conflicting payload is a conflict; the
	// existing projection is never replaced.
	conflict := map[string]any{
		"schema_version": "1.0",
		"ok":             true,
		"data": map[string]any{
			"events": []map[string]any{{
				"schema_version": "1.0",
				"event_id":       "evt_000000000000000001",
				"event_type":     "release.added",
				"publication_id": publication,
				"release_number": "99",
				"release_title":  "Conflicting",
			}},
		},
	}
	raw, _ := json.Marshal(conflict)
	res, err = svc.ImportEvents(ctx, app.ImportRequest{RequestID: "imp-conflict", Actor: importer, Raw: raw})
	if err != nil {
		t.Fatalf("conflict ImportEvents: %v", err)
	}
	if res.Conflicts != 1 {
		t.Fatalf("conflict import = %+v, want one conflict", res)
	}
	rel, err = svc.ListReleases(ctx, app.ReleaseQuery{EditionID: e1})
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	proj, _ := rel.Items[0].(app.ReleaseProjectionView)
	if rel.Total != 1 || proj.ReleaseNumber == nil || *proj.ReleaseNumber != "12" {
		t.Fatalf("the conflicting payload must not replace the projection: total=%d number=%v", rel.Total, proj.ReleaseNumber)
	}

	// 6. The explicit rebind moves the projection and preserves the history.
	rebound, err := svc.RebindRelease(ctx, app.BindReleaseRequest{
		MutationBase:  app.MutationBase{RequestID: "rebind-1", Actor: importer},
		PublicationID: publication,
		EditionID:     e2ID,
	})
	if err != nil {
		t.Fatalf("RebindRelease: %v", err)
	}
	if rebound.EditionID != e2ID || rebound.ReplacesID == nil {
		t.Fatalf("rebind = %+v, want the second edition with a replaces link", rebound)
	}
	rel1, _ := svc.ListReleases(ctx, app.ReleaseQuery{EditionID: e1})
	rel2, err := svc.ListReleases(ctx, app.ReleaseQuery{EditionID: e2ID})
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if rel1.Total != 0 || rel2.Total != 1 {
		t.Fatalf("after rebind: e1=%d e2=%d, want 0 and 1 (no duplicate active projection)", rel1.Total, rel2.Total)
	}
	history, err := svc.ListReleaseBindings(ctx, "", publication)
	if err != nil {
		t.Fatalf("ListReleaseBindings: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("binding history = %d rows, want 2 (the old binding stays, never deleted)", len(history))
	}

	// 7. Personal chapter read and intention are unchanged by the whole flow.
	view, err := svc.GetEdition(ctx, e1)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if view.Intention == nil || view.Intention.Value != domain.IntentionPlanned {
		t.Fatalf("intention changed by the import/binding: %+v", view.Intention)
	}
	// No consumption was ever recorded, so the chapter read stays unknown.
	if view.Consumption != nil {
		t.Fatalf("consumption changed by the import/binding: %+v", view.Consumption)
	}
}
