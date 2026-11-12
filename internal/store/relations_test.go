package store

import (
	"errors"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

var relationActor = RelationActor{Principal: "local", Origin: "user"}

func mustAddRelation(t *testing.T, db *Database, now time.Time, in RelationInput, revision int64) RelationResult {
	t.Helper()
	res, err := db.AddRelation(ctx(t), now, in, revision, relationActor, nil)
	if err != nil {
		t.Fatalf("AddRelation(%+v, rev %d): %v", in, revision, err)
	}
	return res
}

func editionAdaptation(from, to string, state domain.ClaimState) RelationInput {
	return RelationInput{
		Scope:      domain.ScopeEdition,
		Type:       domain.RelationAdaptationOf,
		FromID:     from,
		ToID:       to,
		State:      state,
		Provenance: "manual",
	}
}

// TestRelationAddConfirmRevoke proves the relation lifecycle on editions:
// a proposed edge stays proposed and bumps the canonical from entity, an
// explicit confirmation records the confirmer, a re-confirmation and a
// re-revocation are no-ops without a revision bump, and a revoked edge
// keeps its row, state and instants in the list view.
func TestRelationAddConfirmRevoke(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Anime Story")
	eAnime, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindAnime, "Anime")
	eManga, _ := mustEdition(t, db, now.Add(2*time.Second), w, domain.KindManga, "Manga Source")

	// A proposed adaptation edge: proposed stays proposed.
	res := mustAddRelation(t, db, now.Add(3*time.Second), editionAdaptation(eAnime.ID, eManga.ID, domain.ClaimProposed), eAnime.Revision)
	if res.Relation.State != domain.ClaimProposed || res.Relation.ConfirmedBy != nil {
		t.Fatalf("edge = %+v, want proposed without confirmation", res.Relation)
	}
	if res.EntityID != eAnime.ID || res.Revision != eAnime.Revision+1 {
		t.Fatalf("result = (%s, %d), want the adaptation's entity bumped", res.EntityID, res.Revision)
	}
	if res.Relation.FromID != eAnime.ID || res.Relation.ToID != eManga.ID {
		t.Fatalf("direction = %s -> %s, want adaptation -> source", res.Relation.FromID, res.Relation.ToID)
	}

	// The explicit confirmation records the confirmer and bumps again.
	confirmed, err := db.ConfirmRelation(ctx(t), now.Add(4*time.Second), res.Relation.ID, res.Revision, relationActor, nil)
	if err != nil {
		t.Fatalf("ConfirmRelation: %v", err)
	}
	if confirmed.Relation.State != domain.ClaimConfirmed || confirmed.Relation.ConfirmedBy == nil || *confirmed.Relation.ConfirmedBy != "local" || confirmed.Relation.ConfirmedAt == nil {
		t.Fatalf("edge = %+v, want confirmed with the confirmer", confirmed.Relation)
	}
	if confirmed.Revision != res.Revision+1 {
		t.Fatalf("revision = %d, want %d (confirmation bumps the from entity)", confirmed.Revision, res.Revision+1)
	}

	// Re-confirming is a no-op without a revision bump.
	noOp, err := db.ConfirmRelation(ctx(t), now.Add(5*time.Second), res.Relation.ID, confirmed.Revision, relationActor, nil)
	if err != nil {
		t.Fatalf("re-confirm: %v", err)
	}
	if noOp.Revision != confirmed.Revision {
		t.Fatalf("revision = %d, want %d (re-confirmation is a no-op)", noOp.Revision, confirmed.Revision)
	}

	// Revoking keeps the row in history.
	revoked, err := db.RevokeRelation(ctx(t), now.Add(6*time.Second), res.Relation.ID, confirmed.Revision, relationActor, nil)
	if err != nil {
		t.Fatalf("RevokeRelation: %v", err)
	}
	if revoked.Relation.State != domain.ClaimRevoked || revoked.Relation.RevokedBy == nil || revoked.Relation.RevokedAt == nil {
		t.Fatalf("edge = %+v, want revoked with the revoker and instant", revoked.Relation)
	}
	// Re-revoking is a no-op without a revision bump.
	noOp, err = db.RevokeRelation(ctx(t), now.Add(7*time.Second), res.Relation.ID, revoked.Revision, relationActor, nil)
	if err != nil {
		t.Fatalf("re-revoke: %v", err)
	}
	if noOp.Revision != revoked.Revision {
		t.Fatalf("revision = %d, want %d (re-revocation is a no-op)", noOp.Revision, revoked.Revision)
	}
	// The list keeps the revoked edge in its history bucket.
	infos, err := db.ListRelations(ctx(t), domain.ScopeEdition, eAnime.ID)
	if err != nil {
		t.Fatalf("ListRelations: %v", err)
	}
	if len(infos) != 1 || infos[0].State != domain.ClaimRevoked || infos[0].ID != res.Relation.ID {
		t.Fatalf("relations = %+v, want the revoked edge in history", infos)
	}
}

// TestRelationDuplicateReturnsExisting proves the duplicate active edge of
// the same scope, type and canonical endpoints returns the existing edge
// without a revision bump, and that symmetric endpoints are stored in
// canonical order so (a,b) and (b,a) are one edge.
func TestRelationDuplicateReturnsExisting(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Versions")
	eA, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Line A")
	eB, _ := mustEdition(t, db, now.Add(2*time.Second), w, domain.KindManga, "Line B")

	first := mustAddRelation(t, db, now.Add(3*time.Second), RelationInput{
		Scope: domain.ScopeEdition, Type: domain.RelationAlternativeVersion,
		FromID: eB.ID, ToID: eA.ID, State: domain.ClaimConfirmed, Provenance: "manual",
	}, 0)
	canonicalFrom, canonicalTo := domain.CanonicalEndpoints(eB.ID, eA.ID)
	if first.Relation.FromID != canonicalFrom || first.Relation.ToID != canonicalTo {
		t.Fatalf("stored edge = %s -> %s, want canonical order %s -> %s", first.Relation.FromID, first.Relation.ToID, canonicalFrom, canonicalTo)
	}
	// The reversed endpoints are the same symmetric edge.
	second := mustAddRelation(t, db, now.Add(4*time.Second), RelationInput{
		Scope: domain.ScopeEdition, Type: domain.RelationAlternativeVersion,
		FromID: eA.ID, ToID: eB.ID, State: domain.ClaimConfirmed, Provenance: "manual",
	}, first.Revision)
	if second.Relation.ID != first.Relation.ID {
		t.Fatalf("duplicate edge = %s, want the existing edge %s", second.Relation.ID, first.Relation.ID)
	}
	if second.Revision != first.Revision {
		t.Fatalf("revision = %d, want %d (a duplicate bumps nothing)", second.Revision, first.Revision)
	}
	var rows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM media_relations WHERE scope = 'edition' AND relation = 'alternative_version'`).Scan(&rows); err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if rows != 1 {
		t.Fatalf("edges = %d, want 1 for the symmetric pair", rows)
	}
}

// TestRelationInvalidEndpoints proves self links, mixed-scope links,
// wrong-scope types and absent endpoints are rejected with
// distinguishable errors and no partial mutation.
func TestRelationInvalidEndpoints(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Invalid Links")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Solo")
	w2 := mustWork(t, db, now.Add(2*time.Second), "Second Work")
	e2, _ := mustEdition(t, db, now.Add(3*time.Second), w2, domain.KindManga, "Other")
	missing := "00000000-0000-4000-8000-000000000000"

	cases := []struct {
		name string
		in   RelationInput
		want error
	}{
		{"self link", editionAdaptation(e.ID, e.ID, domain.ClaimProposed), ErrInvalidRelation},
		{"mixed scope: work id in edition link", RelationInput{
			Scope: domain.ScopeEdition, Type: domain.RelationRelated,
			FromID: e.ID, ToID: w.ID, State: domain.ClaimProposed, Provenance: "manual",
		}, ErrInvalidRelation},
		{"mixed scope: edition id in work link", RelationInput{
			Scope: domain.ScopeWork, Type: domain.RelationRelated,
			FromID: w.ID, ToID: e.ID, State: domain.ClaimProposed, Provenance: "manual",
		}, ErrInvalidRelation},
		{"missing edition endpoint", editionAdaptation(e.ID, missing, domain.ClaimProposed), ErrEditionNotFound},
		{"missing work endpoint", RelationInput{
			Scope: domain.ScopeWork, Type: domain.RelationRelated,
			FromID: w.ID, ToID: missing, State: domain.ClaimProposed, Provenance: "manual",
		}, ErrWorkNotFound},
		{"work-scoped type in edition scope", RelationInput{
			Scope: domain.ScopeEdition, Type: domain.RelationSequelOf,
			FromID: e.ID, ToID: e2.ID, State: domain.ClaimProposed, Provenance: "manual",
		}, ErrInvalidRelation},
		{"edition-scoped type in work scope", RelationInput{
			Scope: domain.ScopeWork, Type: domain.RelationAdaptationOf,
			FromID: w.ID, ToID: w2.ID, State: domain.ClaimProposed, Provenance: "manual",
		}, ErrInvalidRelation},
		{"blank provenance", RelationInput{
			Scope: domain.ScopeEdition, Type: domain.RelationAdaptationOf,
			FromID: e.ID, ToID: e2.ID, State: domain.ClaimProposed, Provenance: "   ",
		}, ErrInvalidRelation},
	}
	edgesBefore := func() int {
		var n int
		if err := db.DB().QueryRow(`SELECT COUNT(*) FROM media_relations`).Scan(&n); err != nil {
			t.Fatalf("count edges: %v", err)
		}
		return n
	}
	for _, c := range cases {
		before := edgesBefore()
		if _, err := db.AddRelation(ctx(t), now, c.in, 0, relationActor, nil); !errors.Is(err, c.want) {
			t.Fatalf("%s: err = %v, want %v", c.name, err, c.want)
		}
		if after := edgesBefore(); after != before {
			t.Fatalf("%s: edges = %d, want %d (no partial mutation)", c.name, after, before)
		}
	}
}

// TestRelationCycleRejected proves a cyclic confirmed source link is
// rejected in the same write transaction with no partial mutation: the
// direct pair, the mixed adaptation/translation source graph, a three-hop
// source loop, the sequel graph and the side-story graph, while proposed
// edges and undirected related cycles never trip the check.
func TestRelationCycleRejected(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Cycle Story")
	eA, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindAnime, "A")
	eB, w := mustEdition(t, db, now.Add(2*time.Second), w, domain.KindManga, "B")
	eC, w := mustEdition(t, db, now.Add(3*time.Second), w, domain.KindBook, "C")

	// A -> B confirmed; B -> A would loop the source graph.
	mustAddRelation(t, db, now.Add(4*time.Second), editionAdaptation(eA.ID, eB.ID, domain.ClaimConfirmed), 0)
	if _, err := db.AddRelation(ctx(t), now, editionAdaptation(eB.ID, eA.ID, domain.ClaimConfirmed), 0, relationActor, nil); !errors.Is(err, ErrRelationCycle) {
		t.Fatalf("direct source cycle: err = %v, want ErrRelationCycle", err)
	}
	var rows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM media_relations WHERE from_id = ? AND to_id = ?`, eB.ID, eA.ID).Scan(&rows); err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if rows != 0 {
		t.Fatal("the cyclic edge was written (partial mutation)")
	}
	// The mixed translation/adaptation source graph: A translation_of B is
	// proposed (no cycle yet) and confirming B adaptation_of A loops the
	// combined confirmed graph through A adaptation_of B.
	mustAddRelation(t, db, now.Add(5*time.Second), RelationInput{
		Scope: domain.ScopeEdition, Type: domain.RelationTranslationOf,
		FromID: eA.ID, ToID: eB.ID, State: domain.ClaimProposed, Provenance: "manual",
	}, 1)
	if _, err := db.AddRelation(ctx(t), now, editionAdaptation(eB.ID, eA.ID, domain.ClaimConfirmed), 0, relationActor, nil); !errors.Is(err, ErrRelationCycle) {
		t.Fatalf("mixed source cycle: err = %v, want ErrRelationCycle", err)
	}
	// Confirming the proposed translation edge is allowed: no cycle.
	proposed, err := db.ListRelations(ctx(t), domain.ScopeEdition, eA.ID)
	if err != nil {
		t.Fatalf("ListRelations: %v", err)
	}
	var translationID string
	for _, r := range proposed {
		if r.Type == domain.RelationTranslationOf {
			translationID = r.ID
		}
	}
	if _, err := db.ConfirmRelation(ctx(t), now.Add(6*time.Second), translationID, 2, relationActor, nil); err != nil {
		t.Fatalf("confirming the proposed translation: %v", err)
	}
	// Three-hop source loop: A -> B -> C -> A.
	mustAddRelation(t, db, now.Add(7*time.Second), RelationInput{
		Scope: domain.ScopeEdition, Type: domain.RelationTranslationOf,
		FromID: eB.ID, ToID: eC.ID, State: domain.ClaimConfirmed, Provenance: "manual",
	}, 0)
	if _, err := db.AddRelation(ctx(t), now, RelationInput{
		Scope: domain.ScopeEdition, Type: domain.RelationAdaptationOf,
		FromID: eC.ID, ToID: eA.ID, State: domain.ClaimConfirmed, Provenance: "manual",
	}, 0, relationActor, nil); !errors.Is(err, ErrRelationCycle) {
		t.Fatalf("three-hop source cycle: err = %v, want ErrRelationCycle", err)
	}
	// The sequel graph and the side-story graph check separately.
	w1 := mustWork(t, db, now.Add(8*time.Second), "S1")
	w2 := mustWork(t, db, now.Add(9*time.Second), "S2")
	w3 := mustWork(t, db, now.Add(10*time.Second), "S3")
	workRev := func(id string) int64 {
		fresh, err := db.GetWork(ctx(t), id)
		if err != nil {
			t.Fatalf("GetWork: %v", err)
		}
		return fresh.Revision
	}
	canonicalFrom := func(a, b string) string {
		f, _ := domain.CanonicalEndpoints(a, b)
		return f
	}
	mustAddRelation(t, db, now.Add(11*time.Second), RelationInput{
		Scope: domain.ScopeWork, Type: domain.RelationSequelOf,
		FromID: w2.ID, ToID: w1.ID, State: domain.ClaimConfirmed, Provenance: "manual",
	}, workRev(w2.ID))
	if _, err := db.AddRelation(ctx(t), now, RelationInput{
		Scope: domain.ScopeWork, Type: domain.RelationSequelOf,
		FromID: w1.ID, ToID: w2.ID, State: domain.ClaimConfirmed, Provenance: "manual",
	}, workRev(w1.ID), relationActor, nil); !errors.Is(err, ErrRelationCycle) {
		t.Fatalf("sequel cycle: err = %v, want ErrRelationCycle", err)
	}
	mustAddRelation(t, db, now.Add(12*time.Second), RelationInput{
		Scope: domain.ScopeWork, Type: domain.RelationSideStoryOf,
		FromID: w3.ID, ToID: w2.ID, State: domain.ClaimConfirmed, Provenance: "manual",
	}, workRev(w3.ID))
	if _, err := db.AddRelation(ctx(t), now, RelationInput{
		Scope: domain.ScopeWork, Type: domain.RelationSideStoryOf,
		FromID: w2.ID, ToID: w3.ID, State: domain.ClaimConfirmed, Provenance: "manual",
	}, workRev(w2.ID), relationActor, nil); !errors.Is(err, ErrRelationCycle) {
		t.Fatalf("side-story cycle: err = %v, want ErrRelationCycle", err)
	}
	// Undirected related cycles are allowed.
	mustAddRelation(t, db, now.Add(13*time.Second), RelationInput{
		Scope: domain.ScopeWork, Type: domain.RelationRelated,
		FromID: w1.ID, ToID: w2.ID, State: domain.ClaimConfirmed, Provenance: "manual",
	}, workRev(canonicalFrom(w1.ID, w2.ID)))
	mustAddRelation(t, db, now.Add(14*time.Second), RelationInput{
		Scope: domain.ScopeWork, Type: domain.RelationRelated,
		FromID: w2.ID, ToID: w3.ID, State: domain.ClaimConfirmed, Provenance: "manual",
	}, workRev(canonicalFrom(w2.ID, w3.ID)))
	res := mustAddRelation(t, db, now.Add(15*time.Second), RelationInput{
		Scope: domain.ScopeWork, Type: domain.RelationRelated,
		FromID: w3.ID, ToID: w1.ID, State: domain.ClaimConfirmed, Provenance: "manual",
	}, workRev(canonicalFrom(w3.ID, w1.ID)))
	if res.Relation.State != domain.ClaimConfirmed {
		t.Fatalf("related cycle = %+v, want confirmed (undirected edges may cycle)", res.Relation)
	}
}

// TestRelationReplacement proves a replacement revokes the named active
// edge and inserts a new one with the replacement link, keeps both rows
// in history, and rejects a missing or already-revoked replacement target
// with a distinguishable conflict and no partial mutation.
func TestRelationReplacement(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Correction")
	eA, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindAnime, "A")
	eB, _ := mustEdition(t, db, now.Add(2*time.Second), w, domain.KindManga, "B")

	editionRev := func(id string) int64 {
		fresh, err := db.GetEdition(ctx(t), id)
		if err != nil {
			t.Fatalf("GetEdition: %v", err)
		}
		return fresh.Revision
	}
	canonicalFrom := func(a, b string) string {
		f, _ := domain.CanonicalEndpoints(a, b)
		return f
	}
	first := mustAddRelation(t, db, now.Add(3*time.Second), editionAdaptation(eA.ID, eB.ID, domain.ClaimProposed), 0)
	// A correction: replace the proposed adaptation with a confirmed one
	// and new provenance.
	second := mustAddRelation(t, db, now.Add(4*time.Second), RelationInput{
		Scope: domain.ScopeEdition, Type: domain.RelationAdaptationOf,
		FromID: eA.ID, ToID: eB.ID, State: domain.ClaimConfirmed, Provenance: "corrected",
		ReplacesID: &first.Relation.ID,
	}, first.Revision)
	if second.Relation.ReplacesID == nil || *second.Relation.ReplacesID != first.Relation.ID {
		t.Fatalf("replacement = %+v, want the replacement link", second.Relation)
	}
	if second.Relation.State != domain.ClaimConfirmed {
		t.Fatalf("replacement = %+v, want confirmed", second.Relation)
	}
	infos, err := db.ListRelations(ctx(t), domain.ScopeEdition, eA.ID)
	if err != nil {
		t.Fatalf("ListRelations: %v", err)
	}
	var revoked, confirmed int
	for _, r := range infos {
		switch r.State {
		case domain.ClaimRevoked:
			revoked++
		case domain.ClaimConfirmed:
			confirmed++
		}
	}
	if revoked != 1 || confirmed != 1 {
		t.Fatalf("history = %d revoked, %d confirmed; want one of each (the corrected edge stays visible)", revoked, confirmed)
	}
	// Replacing an already-revoked edge is a distinguishable conflict.
	if _, err := db.AddRelation(ctx(t), now, RelationInput{
		Scope: domain.ScopeEdition, Type: domain.RelationAdaptationOf,
		FromID: eA.ID, ToID: eB.ID, State: domain.ClaimConfirmed, Provenance: "again",
		ReplacesID: &first.Relation.ID,
	}, second.Revision, relationActor, nil); !errors.Is(err, ErrRelationConflict) {
		t.Fatalf("replacing a revoked edge: err = %v, want ErrRelationConflict", err)
	}
	// Replacing a missing edge is a distinguishable conflict.
	if _, err := db.AddRelation(ctx(t), now, RelationInput{
		Scope: domain.ScopeEdition, Type: domain.RelationAdaptationOf,
		FromID: eA.ID, ToID: eB.ID, State: domain.ClaimConfirmed, Provenance: "ghost",
		ReplacesID: strPtr("00000000-0000-4000-8000-000000000000"),
	}, second.Revision, relationActor, nil); !errors.Is(err, ErrRelationConflict) {
		t.Fatalf("replacing a missing edge: err = %v, want ErrRelationConflict", err)
	}
	// A replacement whose target is a different edge cannot create a
	// parallel active edge of the same shape.
	third := mustAddRelation(t, db, now.Add(5*time.Second), RelationInput{
		Scope: domain.ScopeEdition, Type: domain.RelationRelated,
		FromID: eA.ID, ToID: eB.ID, State: domain.ClaimConfirmed, Provenance: "manual",
	}, editionRev(canonicalFrom(eA.ID, eB.ID)))
	if _, err := db.AddRelation(ctx(t), now, RelationInput{
		Scope: domain.ScopeEdition, Type: domain.RelationAdaptationOf,
		FromID: eA.ID, ToID: eB.ID, State: domain.ClaimConfirmed, Provenance: "parallel",
		ReplacesID: &third.Relation.ID,
	}, editionRev(eA.ID), relationActor, nil); !errors.Is(err, ErrRelationConflict) {
		t.Fatalf("parallel active edge: err = %v, want ErrRelationConflict", err)
	}
}

// TestRelationRevisionGuard proves a stale expected revision on the
// canonical from entity is a distinguishable conflict with no partial
// mutation.
func TestRelationRevisionGuard(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Stale")
	eA, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindAnime, "A")
	eB, _ := mustEdition(t, db, now.Add(2*time.Second), w, domain.KindManga, "B")

	if _, err := db.AddRelation(ctx(t), now, editionAdaptation(eA.ID, eB.ID, domain.ClaimProposed), 99, relationActor, nil); !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("stale revision: err = %v, want ErrRevisionMismatch", err)
	}
	var rows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM media_relations`).Scan(&rows); err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if rows != 0 {
		t.Fatal("the edge was written despite the stale revision")
	}
}

// TestRelationRelatedTraversal proves the related-media read traverses
// both directions with a bounded depth and a visited set, shows proposals
// with their state, and terminates on undirected cycles.
func TestRelationRelatedTraversal(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Traversal")
	eA, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindAnime, "A")
	eB, w := mustEdition(t, db, now.Add(2*time.Second), w, domain.KindManga, "B")
	eC, w := mustEdition(t, db, now.Add(3*time.Second), w, domain.KindBook, "C")

	editionRev := func(id string) int64 {
		fresh, err := db.GetEdition(ctx(t), id)
		if err != nil {
			t.Fatalf("GetEdition: %v", err)
		}
		return fresh.Revision
	}
	canonicalFrom := func(a, b string) string {
		f, _ := domain.CanonicalEndpoints(a, b)
		return f
	}
	mustAddRelation(t, db, now.Add(4*time.Second), RelationInput{
		Scope: domain.ScopeEdition, Type: domain.RelationRelated,
		FromID: eA.ID, ToID: eB.ID, State: domain.ClaimConfirmed, Provenance: "manual",
	}, editionRev(canonicalFrom(eA.ID, eB.ID)))
	mustAddRelation(t, db, now.Add(5*time.Second), RelationInput{
		Scope: domain.ScopeEdition, Type: domain.RelationRelated,
		FromID: eB.ID, ToID: eC.ID, State: domain.ClaimProposed, Provenance: "manual",
	}, editionRev(canonicalFrom(eB.ID, eC.ID)))
	// The undirected cycle back to A must terminate.
	mustAddRelation(t, db, now.Add(6*time.Second), RelationInput{
		Scope: domain.ScopeEdition, Type: domain.RelationRelated,
		FromID: eC.ID, ToID: eA.ID, State: domain.ClaimConfirmed, Provenance: "manual",
	}, editionRev(canonicalFrom(eC.ID, eA.ID)))

	edges, err := db.RelatedMedia(ctx(t), domain.ScopeEdition, eA.ID, 1)
	if err != nil {
		t.Fatalf("RelatedMedia: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("depth-1 edges = %+v, want the two direct neighbours", edges)
	}
	for _, e := range edges {
		if e.Depth != 1 {
			t.Fatalf("edge %+v: depth = %d, want 1", e, e.Depth)
		}
	}
	// Both directions: from B the traversal reaches A and C.
	edges, err = db.RelatedMedia(ctx(t), domain.ScopeEdition, eB.ID, 2)
	if err != nil {
		t.Fatalf("RelatedMedia: %v", err)
	}
	seen := map[string]bool{}
	var proposed int
	for _, e := range edges {
		seen[e.RelationID] = true
		if e.State == domain.ClaimProposed {
			proposed++
		}
	}
	if len(seen) != 3 {
		t.Fatalf("depth-2 edges = %+v, want all three (the cycle terminates)", edges)
	}
	if proposed != 1 {
		t.Fatalf("proposed edges = %d, want the proposals shown with their state", proposed)
	}
	// A revoked edge is absent from the traversal.
	first := edges[0]
	rev, err := db.RevokeRelation(ctx(t), now.Add(7*time.Second), first.RelationID, editionRev(first.FromID), relationActor, nil)
	if err != nil {
		t.Fatalf("RevokeRelation: %v", err)
	}
	edges, err = db.RelatedMedia(ctx(t), domain.ScopeEdition, eB.ID, 2)
	if err != nil {
		t.Fatalf("RelatedMedia: %v", err)
	}
	for _, e := range edges {
		if e.RelationID == rev.Relation.ID {
			t.Fatalf("revoked edge %s is still traversed", rev.Relation.ID)
		}
	}
}

// TestOriginRecordConfirmRevoke proves the origin claim lifecycle: an
// explicit assertion stays proposed until an explicit confirmation, an
// exact re-assertion is a no-op, a different assertion of the same kind
// conflicts, a replacement revokes the named claim and keeps both in
// history, and confirming or replacing a revoked claim is a
// distinguishable conflict.
func TestOriginRecordConfirmRevoke(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Origin Story")
	e, _ := mustEdition(t, db, now.Add(time.Second), w, domain.KindWebNovel, "The WN")

	in := OriginInput{EditionID: e.ID, Kind: domain.OriginAnimeOriginal, State: domain.ClaimProposed, Provenance: "manual"}
	res, err := db.RecordOrigin(ctx(t), now.Add(2*time.Second), in, e.Revision, relationActor, nil)
	if err != nil {
		t.Fatalf("RecordOrigin: %v", err)
	}
	if res.Claim.State != domain.ClaimProposed || res.Claim.ConfirmedBy != nil {
		t.Fatalf("claim = %+v, want proposed without confirmation", res.Claim)
	}
	if res.Revision != e.Revision+1 {
		t.Fatalf("revision = %d, want %d (a claim bumps the edition)", res.Revision, e.Revision+1)
	}
	// An exact re-assertion is a no-op without a revision bump.
	noOp, err := db.RecordOrigin(ctx(t), now.Add(3*time.Second), in, res.Revision, relationActor, nil)
	if err != nil {
		t.Fatalf("re-assertion: %v", err)
	}
	if noOp.Claim.ID != res.Claim.ID || noOp.Revision != res.Revision {
		t.Fatalf("re-assertion = (%s, %d), want the existing claim at the same revision", noOp.Claim.ID, noOp.Revision)
	}
	// A different assertion of the same kind conflicts.
	different := in
	different.Provenance = "another source"
	if _, err := db.RecordOrigin(ctx(t), now, different, res.Revision, relationActor, nil); !errors.Is(err, ErrOriginConflict) {
		t.Fatalf("different provenance: err = %v, want ErrOriginConflict", err)
	}
	// The explicit confirmation records the confirmer and bumps again.
	confirmed, err := db.ConfirmOrigin(ctx(t), now.Add(4*time.Second), e.ID, res.Claim.ID, res.Revision, relationActor, nil)
	if err != nil {
		t.Fatalf("ConfirmOrigin: %v", err)
	}
	if confirmed.Claim.State != domain.ClaimConfirmed || confirmed.Claim.ConfirmedBy == nil || *confirmed.Claim.ConfirmedBy != "local" || confirmed.Claim.ConfirmedAt == nil {
		t.Fatalf("claim = %+v, want confirmed with the confirmer", confirmed.Claim)
	}
	if confirmed.Revision != res.Revision+1 {
		t.Fatalf("revision = %d, want %d (confirmation bumps the edition)", confirmed.Revision, res.Revision+1)
	}
	// Re-confirming is a no-op without a revision bump.
	noOp, err = db.ConfirmOrigin(ctx(t), now.Add(5*time.Second), e.ID, res.Claim.ID, confirmed.Revision, relationActor, nil)
	if err != nil {
		t.Fatalf("re-confirm: %v", err)
	}
	if noOp.Revision != confirmed.Revision {
		t.Fatalf("revision = %d, want %d (re-confirmation is a no-op)", noOp.Revision, confirmed.Revision)
	}
	// The correction: a replacement revokes the confirmed claim and keeps
	// both rows in history with the replacement link.
	replacement := OriginInput{EditionID: e.ID, Kind: domain.OriginOriginal, State: domain.ClaimConfirmed, Provenance: "corrected", ReplacesID: &res.Claim.ID}
	fresh, err := db.RecordOrigin(ctx(t), now.Add(6*time.Second), replacement, confirmed.Revision, relationActor, nil)
	if err != nil {
		t.Fatalf("replacement: %v", err)
	}
	if fresh.Claim.ReplacesID == nil || *fresh.Claim.ReplacesID != res.Claim.ID {
		t.Fatalf("replacement = %+v, want the replacement link", fresh.Claim)
	}
	claims, err := db.ListOrigins(ctx(t), e.ID)
	if err != nil {
		t.Fatalf("ListOrigins: %v", err)
	}
	var revoked, active int
	for _, c := range claims {
		switch c.State {
		case domain.ClaimRevoked:
			revoked++
		default:
			active++
		}
	}
	if revoked != 1 || active != 1 {
		t.Fatalf("history = %d revoked, %d active; want one of each (the corrected origin stays visible)", revoked, active)
	}
	// Replacing an already-revoked claim is a distinguishable conflict.
	if _, err := db.RecordOrigin(ctx(t), now, OriginInput{EditionID: e.ID, Kind: domain.OriginUnknown, State: domain.ClaimProposed, Provenance: "x", ReplacesID: &res.Claim.ID}, fresh.Revision, relationActor, nil); !errors.Is(err, ErrOriginConflict) {
		t.Fatalf("replacing a revoked claim: err = %v, want ErrOriginConflict", err)
	}
	// Revoking the active claim keeps its row; re-revoking is a no-op.
	revokedRes, err := db.RevokeOrigin(ctx(t), now.Add(7*time.Second), e.ID, fresh.Claim.ID, fresh.Revision, relationActor, nil)
	if err != nil {
		t.Fatalf("RevokeOrigin: %v", err)
	}
	if revokedRes.Claim.State != domain.ClaimRevoked {
		t.Fatalf("claim = %+v, want revoked", revokedRes.Claim)
	}
	noOp, err = db.RevokeOrigin(ctx(t), now.Add(8*time.Second), e.ID, fresh.Claim.ID, revokedRes.Revision, relationActor, nil)
	if err != nil {
		t.Fatalf("re-revoke: %v", err)
	}
	if noOp.Revision != revokedRes.Revision {
		t.Fatalf("revision = %d, want %d (re-revocation is a no-op)", noOp.Revision, revokedRes.Revision)
	}
	// Confirming a revoked claim is a distinguishable conflict.
	if _, err := db.ConfirmOrigin(ctx(t), now.Add(9*time.Second), e.ID, fresh.Claim.ID, noOp.Revision, relationActor, nil); !errors.Is(err, ErrOriginConflict) {
		t.Fatalf("confirming a revoked claim: err = %v, want ErrOriginConflict", err)
	}
	// A claim of another edition is not found on this one.
	w2 := mustWork(t, db, now.Add(10*time.Second), "Other")
	e2, _ := mustEdition(t, db, now.Add(11*time.Second), w2, domain.KindAnime, "Other Anime")
	if _, err := db.ConfirmOrigin(ctx(t), now, e2.ID, fresh.Claim.ID, e2.Revision, relationActor, nil); !errors.Is(err, ErrOriginNotFound) {
		t.Fatalf("foreign-edition claim: err = %v, want ErrOriginNotFound", err)
	}
}

// TestOriginNoInference proves no originality is ever inferred: a media
// kind alone creates no origin claim, and an unknown origin is the
// absence of a confirmed claim.
func TestOriginNoInference(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "No Inference")
	e, _ := mustEdition(t, db, now.Add(time.Second), w, domain.KindWebNovel, "Plain WN")
	claims, err := db.ListOrigins(ctx(t), e.ID)
	if err != nil {
		t.Fatalf("ListOrigins: %v", err)
	}
	if len(claims) != 0 {
		t.Fatalf("claims = %+v, want none (a kind never implies an origin)", claims)
	}
}
