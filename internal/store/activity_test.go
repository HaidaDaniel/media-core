package store

import (
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

func activityNow() time.Time { return time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC) }

func getEdition(t *testing.T, db *Database, id string) Edition {
	t.Helper()
	e, err := db.GetEdition(ctx(t), id)
	if err != nil {
		t.Fatalf("GetEdition(%s): %v", id, err)
	}
	return e
}

func editionRevision(t *testing.T, db *Database, id string) int64 {
	t.Helper()
	return getEdition(t, db, id).Revision
}

// firstRowID returns the oldest row id of one edition-scoped table, so a
// fixture can supersede a previously written observation.
func firstRowID(t *testing.T, db *Database, table, editionID string) string {
	t.Helper()
	var id string
	err := db.db.QueryRowContext(ctx(t), "SELECT id FROM "+table+" WHERE edition_id = ? ORDER BY recorded_at, id LIMIT 1", editionID).Scan(&id)
	if err != nil {
		t.Fatalf("first %s row: %v", table, err)
	}
	return id
}

// TestActivityRecent projects every history source of one work with two
// editions and checks the recorded ordering, the correction labels, the
// reserved imported-release absence and the target enrichment.
func TestActivityRecent(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	now := activityNow()
	w := mustWork(t, db, now, "Alpha")
	eA, w := mustEdition(t, db, now, w, domain.KindManga, "Alpha Manga")
	eB, w := mustEdition(t, db, now, w, domain.KindAnime, "Alpha Anime")

	// Work-level rows first, out of order against the edition rows.
	w = mustAddWorkNote(t, db, now.Add(17*time.Second), w, "Work level thought")
	w = mustAttachWorkTag(t, db, now.Add(18*time.Second), w, w.Revision, "alpha")

	// A confirmed directed relation between the editions; the revision
	// guard is on the from entity.
	res := mustAddRelation(t, db, now.Add(13*time.Second), RelationInput{
		Scope: domain.ScopeEdition, Type: domain.RelationAdaptationOf,
		FromID: eA.ID, ToID: eB.ID, State: domain.ClaimConfirmed, Provenance: "manual",
	}, editionRevision(t, db, eA.ID))

	// eA observations, inserted out of recorded order; the relation may
	// have bumped eA, so its revision is fetched fresh.
	eA = getEdition(t, db, eA.ID)
	eA = mustSetConsumption(t, db, now.Add(2*time.Second), eA, eA.Revision, ConsumptionInput{Value: "completed"})
	firstConsumption := firstRowID(t, db, "consumption_observations", eA.ID)
	eA = mustSetConsumption(t, db, now.Add(1*time.Second), eA, eA.Revision, ConsumptionInput{Value: "in_progress", SupersedesID: strPtr(firstConsumption)})
	eA = mustRecordProgress(t, db, now.Add(3*time.Second), eA, eA.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "5", Total: strPtr("20")})
	progressRecord := eA.Progress[0].ID
	eA, err := db.RetractProgress(ctx(t), now.Add(4*time.Second), eA.ID, eA.Revision, progressRecord, "miscounted", progressActor(), nil)
	if err != nil {
		t.Fatalf("RetractProgress: %v", err)
	}
	eA = mustSetIntention(t, db, now.Add(5*time.Second), eA, eA.Revision, IntentionInput{Value: "saved"})
	eA = mustSetRating(t, db, now.Add(6*time.Second), eA, RatingInput{Value: mustInt(7)})
	firstRating := firstRowID(t, db, "rating_observations", eA.ID)
	eA = mustSetRating(t, db, now.Add(7*time.Second), eA, RatingInput{SupersedesID: strPtr(firstRating)})
	eA = mustAddEditionNote(t, db, now.Add(8*time.Second), eA, "First thought")
	noteID := eA.Notes[0].ID
	nt, err := db.ReviseNote(ctx(t), now.Add(9*time.Second), noteID, 1, NoteInput{Text: "Second thought"}, NoteActor{Principal: "local", Origin: "user"}, nil)
	if err != nil {
		t.Fatalf("ReviseNote: %v", err)
	}
	eA = nt.Edition
	eA, err = db.RecordPublication(ctx(t), now.Add(10*time.Second), eA.ID, eA.Revision, PublicationInput{Value: domain.PublicationOngoing, Source: strPtr("manual"), Provenance: "manual"})
	if err != nil {
		t.Fatalf("RecordPublication: %v", err)
	}
	eA = mustAddReference(t, db, now.Add(11*time.Second), eA, ReferenceRequest{Provider: "mangaupdates", ExternalID: strPtr("alpha"), Role: domain.RolePublication, Provenance: "manual"})
	referenceID := eA.References[0].ID
	eA, err = db.RetireReference(ctx(t), now.Add(12*time.Second), eA.ID, eA.Revision, referenceID, strPtr("wrong site"), referenceActor, nil)
	if err != nil {
		t.Fatalf("RetireReference: %v", err)
	}
	origin, err := db.RecordOrigin(ctx(t), now.Add(14*time.Second), OriginInput{EditionID: eA.ID, Kind: domain.OriginOriginal, State: domain.ClaimProposed, Provenance: "manual"}, eA.Revision, relationActor, nil)
	if err != nil {
		t.Fatalf("RecordOrigin: %v", err)
	}
	// The origin bump is not chained through the result, so the revision
	// is fetched fresh before the tag mutations.
	eA = getEdition(t, db, eA.ID)
	eA = mustAttachEditionTag(t, db, now.Add(15*time.Second), eA, eA.Revision, "cool")
	eA = mustDetachEditionTag(t, db, now.Add(16*time.Second), eA, eA.Revision, "cool")
	// A row belonging only to eB; the relation may have bumped eB, so the
	// revision is fetched fresh.
	eB = getEdition(t, db, eB.ID)
	eB = mustSetConsumption(t, db, now.Add(19*time.Second), eB, eB.Revision, ConsumptionInput{Value: "in_progress"})

	rows, total, err := db.RecentActivity(ctx(t), ActivityQuery{})
	if err != nil {
		t.Fatalf("RecentActivity: %v", err)
	}
	if total != 17 {
		t.Fatalf("total = %d, want 17", total)
	}
	if len(rows) != 17 {
		t.Fatalf("rows = %d, want 17", len(rows))
	}
	// The order is (recorded_at, id) descending, whatever the insertion
	// order was. Corrections that update a row in place (the reference
	// retirement, the tag detach) leave no row of their own, so the tag
	// row records its attach instant with the detached label.
	wantOrder := []time.Duration{19, 18, 17, 15, 14, 13, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	for i, d := range wantOrder {
		want := now.Add(d * time.Second)
		if !rows[i].RecordedAt.Equal(want) {
			t.Fatalf("rows[%d].RecordedAt = %v, want %v", i, rows[i].RecordedAt, want)
		}
	}
	byID := map[string]ActivityRow{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	if got := byID[firstConsumption].Label; got != "superseded" {
		t.Fatalf("first consumption label = %q, want superseded", got)
	}
	if r := rows[16]; r.Label != "effective" || r.Value == nil || *r.Value != "in_progress" {
		t.Fatalf("last consumption = %+v, want effective in_progress", r)
	}
	if got := byID[progressRecord].Label; got != "superseded" {
		t.Fatalf("progress record label = %q, want superseded", got)
	}
	// The retraction row keeps the reason and renders as retracted.
	if r := rows[13]; r.Type != ActivityProgress || r.Label != "retracted" || r.Reason == nil || *r.Reason != "miscounted" {
		t.Fatalf("progress retraction = %+v, want retracted with the reason", r)
	}
	if got := byID[firstRating].Label; got != "superseded" {
		t.Fatalf("first rating label = %q, want superseded", got)
	}
	if r := rows[10]; r.Type != ActivityRating || r.Label != "retracted" || r.Value != nil {
		t.Fatalf("rating clear = %+v, want retracted with a null value", r)
	}
	if r := rows[8]; r.Type != ActivityNote || r.Label != "current" || r.Value == nil || *r.Value != "Second thought" {
		t.Fatalf("current note revision = %+v, want current with the second text", r)
	}
	if r := rows[9]; r.Type != ActivityNote || r.Label != "superseded" {
		t.Fatalf("first note revision = %+v, want superseded", r)
	}
	if r := rows[7]; r.Type != ActivityPublication || r.Actor != nil {
		t.Fatalf("publication row = %+v, want no actor (the table has none)", r)
	}
	if r := rows[6]; r.Type != ActivityReference || r.Label != "retired" || r.Reason == nil || *r.Reason != "wrong site" {
		t.Fatalf("retired reference = %+v, want retired with the reason", r)
	}
	if r := byID[referenceID]; r.Label != "retired" {
		t.Fatalf("reference label = %q, want retired", r.Label)
	}
	if r := byID[res.Relation.ID]; r.Type != ActivityRelation || r.Label != "confirmed" {
		t.Fatalf("relation row = %+v, want confirmed", r)
	}
	if r := byID[origin.Claim.ID]; r.Type != ActivityOrigin || r.Label != "proposed" {
		t.Fatalf("origin row = %+v, want proposed", r)
	}
	// The tag attach and its detach are one assignment row, recorded at
	// the attach instant and labeled detached.
	if r := rows[3]; r.Type != ActivityTag || r.Label != "detached" || r.Value == nil || *r.Value != "cool" {
		t.Fatalf("detached tag = %+v, want detached cool", r)
	}
	// No imported-release row exists yet: the variant is reserved.
	for _, r := range rows {
		if r.Type == ActivityImportedRelease {
			t.Fatalf("unexpected imported-release row %+v before the importer lands", r)
		}
	}
	// Enrichment: an eA row carries the current title, the active note and
	// no active references (the only one is retired); the work note row
	// carries the work title and the work note; the eB row carries the eB
	// title.
	if r := rows[16]; r.EditionID == nil || *r.EditionID != eA.ID || r.Title == nil || *r.Title != "Alpha Manga" {
		t.Fatalf("eA row = %+v, want the edition preferred title", r)
	}
	if r := rows[16]; len(r.Notes) != 1 || r.Notes[0].Text != "Second thought" {
		t.Fatalf("eA enrichment notes = %+v, want the current note", r.Notes)
	}
	if r := rows[16]; len(r.References) != 0 {
		t.Fatalf("eA enrichment references = %+v, want none active", r.References)
	}
	if r := rows[2]; r.Type != ActivityNote || r.WorkID != w.ID || r.Title == nil || *r.Title != "Alpha" || len(r.Notes) != 1 || r.Notes[0].Text != "Work level thought" {
		t.Fatalf("work note row = %+v, want the work title and the work note", r)
	}
	if r := rows[0]; r.EditionID == nil || *r.EditionID != eB.ID || r.Title == nil || *r.Title != "Alpha Anime" {
		t.Fatalf("eB row = %+v, want the eB title", r)
	}
}

// TestActivityHistory projects the bounded complete history of one work
// and one edition and checks edition isolation, relation endpoint
// visibility, the work span and seek pagination.
func TestActivityHistory(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	now := activityNow()
	w := mustWork(t, db, now, "Alpha")
	eA, w := mustEdition(t, db, now, w, domain.KindManga, "Alpha Manga")
	eB, w := mustEdition(t, db, now, w, domain.KindAnime, "Alpha Anime")
	eC, _ := mustEdition(t, db, now, w, domain.KindManga, "Alpha Second")

	// eA rows.
	eA = mustSetConsumption(t, db, now.Add(1*time.Second), eA, eA.Revision, ConsumptionInput{Value: "in_progress"})
	eA = mustAttachEditionTag(t, db, now.Add(2*time.Second), eA, eA.Revision, "cool")
	// A relation where eA is the to endpoint, so eA sees it only through
	// the endpoint column; the guard is on the from entity.
	res := mustAddRelation(t, db, now.Add(3*time.Second), RelationInput{
		Scope: domain.ScopeEdition, Type: domain.RelationAdaptationOf,
		FromID: eB.ID, ToID: eA.ID, State: domain.ClaimConfirmed, Provenance: "manual",
	}, editionRevision(t, db, eB.ID))
	// eB rows; the relation may have bumped eB.
	eB = getEdition(t, db, eB.ID)
	eB = mustSetConsumption(t, db, now.Add(4*time.Second), eB, eB.Revision, ConsumptionInput{Value: "paused"})

	// The work history spans both editions' rows and the relation.
	rows, total, err := db.EntityHistory(ctx(t), HistoryQuery{WorkID: strPtr(w.ID)})
	if err != nil {
		t.Fatalf("EntityHistory(work): %v", err)
	}
	if total != 4 {
		t.Fatalf("work total = %d, want 4", total)
	}
	ids := map[string]bool{}
	for _, r := range rows {
		if r.WorkID != w.ID {
			t.Fatalf("work history row %+v outside the work", r)
		}
		ids[r.ID] = true
	}
	if !ids[res.Relation.ID] {
		t.Fatalf("work history misses the relation row")
	}

	// The eA history holds eA's own rows and the relation where eA is an
	// endpoint, never eB's own rows.
	rows, total, err = db.EntityHistory(ctx(t), HistoryQuery{EditionID: strPtr(eA.ID)})
	if err != nil {
		t.Fatalf("EntityHistory(eA): %v", err)
	}
	if total != 3 {
		t.Fatalf("eA total = %d, want 3", total)
	}
	sawRelation := false
	for _, r := range rows {
		if r.ID == res.Relation.ID {
			sawRelation = true
		}
		if r.EditionID != nil && *r.EditionID == eB.ID && r.Type != ActivityRelation {
			t.Fatalf("eA history leaks an eB row %+v", r)
		}
	}
	if !sawRelation {
		t.Fatalf("eA history misses the relation where eA is an endpoint")
	}

	// An edition without rows is an empty history, not an error.
	rows, total, err = db.EntityHistory(ctx(t), HistoryQuery{EditionID: strPtr(eC.ID)})
	if err != nil {
		t.Fatalf("EntityHistory(eC): %v", err)
	}
	if total != 0 || len(rows) != 0 {
		t.Fatalf("eC history = %d rows, total %d, want empty", len(rows), total)
	}

	// Seek pagination over the work history: pages of two, no skips and no
	// duplicates, the order preserved.
	var all []ActivityRow
	page, total, err := db.EntityHistory(ctx(t), HistoryQuery{WorkID: strPtr(w.ID), Limit: 2})
	if err != nil {
		t.Fatalf("EntityHistory page 1: %v", err)
	}
	if total != 4 || len(page) != 2 {
		t.Fatalf("page 1 = %d rows, total %d, want 2 of 4", len(page), total)
	}
	all = append(all, page...)
	for len(page) == 2 {
		seek := HistoryQuery{
			WorkID:          strPtr(w.ID),
			Limit:           2,
			AfterRecordedAt: InstantKey(page[len(page)-1].RecordedAt),
			AfterID:         page[len(page)-1].ID,
		}
		page, total, err = db.EntityHistory(ctx(t), seek)
		if err != nil {
			t.Fatalf("EntityHistory next page: %v", err)
		}
		all = append(all, page...)
	}
	seen := map[string]bool{}
	for i, r := range all {
		if seen[r.ID] {
			t.Fatalf("duplicated history row %s at page boundary", r.ID)
		}
		seen[r.ID] = true
		if i > 0 && all[i-1].RecordedAt.Before(r.RecordedAt) {
			t.Fatalf("history order broken at %d: %v after %v", i, r.RecordedAt, all[i-1].RecordedAt)
		}
	}
	if len(all) != 4 {
		t.Fatalf("paginated history = %d rows, want 4", len(all))
	}
}

// TestActivityImportedReleaseReserved keeps the reserved imported-release
// variant defined and empty: the feed filtered by it returns nothing, and
// an unknown type is a distinct failure.
func TestActivityImportedReleaseReserved(t *testing.T) {
	if !ActivityImportedRelease.Valid() {
		t.Fatalf("imported_release must stay a defined activity type for 0043")
	}
	db := openMigrated(t, t.TempDir())
	now := activityNow()
	w := mustWork(t, db, now, "Alpha")
	eA, _ := mustEdition(t, db, now, w, domain.KindManga, "Alpha Manga")
	eA = mustSetConsumption(t, db, now.Add(time.Second), eA, eA.Revision, ConsumptionInput{Value: "in_progress"})
	rows, total, err := db.RecentActivity(ctx(t), ActivityQuery{Types: []ActivityType{ActivityImportedRelease}})
	if err != nil {
		t.Fatalf("RecentActivity(imported_release): %v", err)
	}
	if total != 0 || len(rows) != 0 {
		t.Fatalf("imported_release feed = %d rows, total %d, want empty before the importer", len(rows), total)
	}
	if _, _, err := db.RecentActivity(ctx(t), ActivityQuery{Types: []ActivityType{ActivityType("nope")}}); err == nil {
		t.Fatalf("unknown activity type: err = nil, want a failure")
	}
}

// TestLibraryFacets keeps the library listing's counts and filter facets
// correct once the annotation and relation tables carry data: the facets
// count filtered works before pagination and the annotation state never
// changes a count.
func TestLibraryFacets(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	now := activityNow()
	w1 := mustWork(t, db, now, "Facet One")
	e1, _ := mustEdition(t, db, now, w1, domain.KindManga, "Facet One Manga")
	e2w := mustWork(t, db, now, "Facet Two")
	e2, _ := mustEdition(t, db, now, e2w, domain.KindManga, "Facet Two Manga")
	w3 := mustWork(t, db, now, "Facet Three")
	e3, _ := mustEdition(t, db, now, w3, domain.KindManga, "Facet Three Manga")
	mustWork(t, db, now, "Facet Four")

	// Annotation state on e1: it enriches, it never re-counts.
	e1 = mustSetConsumption(t, db, now.Add(time.Second), e1, e1.Revision, ConsumptionInput{Value: "in_progress"})
	e1 = mustAddEditionNote(t, db, now.Add(2*time.Second), e1, "A note")
	e1 = mustAddReference(t, db, now.Add(3*time.Second), e1, ReferenceRequest{Provider: "mangaupdates", ExternalID: strPtr("one"), Role: domain.RolePublication, Provenance: "manual"})
	e2 = mustSetRating(t, db, now.Add(4*time.Second), e2, RatingInput{Value: mustInt(8)})
	e3 = mustAttachEditionTag(t, db, now.Add(5*time.Second), e3, e3.Revision, "cool")

	// The whole library counts four works.
	cards, total, err := db.SearchLibrary(ctx(t), LibraryQuery{Sort: SortTitle, Limit: MaxLibraryLimit})
	if err != nil {
		t.Fatalf("SearchLibrary: %v", err)
	}
	if total != 4 || len(cards) != 4 {
		t.Fatalf("library = %d cards, total %d, want 4 of 4", len(cards), total)
	}
	// The facets count their one matching work each.
	if cards, total, _ := db.SearchLibrary(ctx(t), LibraryQuery{Sort: SortTitle, Limit: MaxLibraryLimit, Consumptions: []domain.Consumption{domain.ConsumptionInProgress}}); total != 1 || cards[0].ID != w1.ID {
		t.Fatalf("consumption facet = %d, want 1 matching Facet One", total)
	}
	if cards, total, _ := db.SearchLibrary(ctx(t), LibraryQuery{Sort: SortTitle, Limit: MaxLibraryLimit, MinRating: func() *int { n := 8; return &n }()}); total != 1 || cards[0].ID != e2w.ID {
		t.Fatalf("rating facet = %d, want 1 matching Facet Two", total)
	}
	if cards, total, _ := db.SearchLibrary(ctx(t), LibraryQuery{Sort: SortTitle, Limit: MaxLibraryLimit, Tags: []string{"cool"}}); total != 1 || cards[0].ID != w3.ID {
		t.Fatalf("tag facet = %d, want 1 matching Facet Three", total)
	}
	// Pagination: the total stays the pre-pagination count and the pages
	// are disjoint.
	page, total, err := db.SearchLibrary(ctx(t), LibraryQuery{Sort: SortTitle, Limit: 2})
	if err != nil {
		t.Fatalf("SearchLibrary page 1: %v", err)
	}
	if total != 4 || len(page) != 2 {
		t.Fatalf("page 1 = %d cards, total %d, want 2 of 4", len(page), total)
	}
	next, total, err := db.SearchLibrary(ctx(t), LibraryQuery{Sort: SortTitle, Limit: 2, AfterKey: page[1].SortKey, AfterID: page[1].ID})
	if err != nil {
		t.Fatalf("SearchLibrary page 2: %v", err)
	}
	if total != 4 || len(next) != 2 {
		t.Fatalf("page 2 = %d cards, total %d, want 2 of 4", len(next), total)
	}
	if page[0].ID == page[1].ID || page[1].ID == next[0].ID || page[0].ID == next[0].ID || page[0].ID == next[1].ID {
		t.Fatalf("pages overlap: %+v / %+v", page, next)
	}
}
