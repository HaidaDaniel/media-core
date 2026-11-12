package store

import (
	"errors"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

var referenceActor = ReferenceActor{Principal: "local", Origin: "user"}

func mustAddReference(t *testing.T, db *Database, now time.Time, e Edition, in ReferenceRequest) Edition {
	t.Helper()
	fresh, err := db.AddReference(ctx(t), now, e.ID, e.Revision, in, referenceActor, nil)
	if err != nil {
		t.Fatalf("AddReference(%+v): %v", in, err)
	}
	return fresh
}

func urlRef(url string) ReferenceRequest {
	return ReferenceRequest{
		Provider:   "example",
		URL:        strPtr(url),
		Role:       domain.RolePublication,
		Provenance: "manual",
	}
}

// TestReferenceAddRetire proves the reference lifecycle on an edition:
// adding renders the reference and reserves its 0012 identity, a second URL
// on the same edition adds another reference without creating an edition,
// and retirement keeps the row and the reservation in history while the
// view drops them and the identity becomes free again.
func TestReferenceAddRetire(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Reference Story")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga R")
	edRev := e.Revision
	workCount := func() int {
		var n int
		if err := db.DB().QueryRow(`SELECT COUNT(*) FROM works`).Scan(&n); err != nil {
			t.Fatalf("count works: %v", err)
		}
		return n
	}

	e = mustAddReference(t, db, now.Add(2*time.Second), e, urlRef("https://example.org/anime/story"))
	if e.Revision != edRev+1 {
		t.Fatalf("edition revision = %d, want %d (a reference bumps the edition)", e.Revision, edRev+1)
	}
	if len(e.References) != 1 {
		t.Fatalf("references = %+v, want 1", e.References)
	}
	r := e.References[0]
	if r.Provider != "example" || r.URL == nil || *r.URL != "https://example.org/anime/story" ||
		r.Role != domain.RolePublication || r.Authoritative || r.Provenance != "manual" || r.RetiredAt != nil {
		t.Fatalf("reference = %+v, want the published reference", r)
	}
	var reserved int
	if err := db.DB().QueryRow(
		`SELECT COUNT(*) FROM external_identities WHERE edition_id = ? AND retired_at IS NULL`, e.ID).Scan(&reserved); err != nil {
		t.Fatalf("count reservations: %v", err)
	}
	if reserved != 1 {
		t.Fatalf("active reservations = %d, want 1 (the 0012 identity reservation)", reserved)
	}

	// A second URL on the same edition adds another reference and never
	// creates another edition.
	e = mustAddReference(t, db, now.Add(3*time.Second), e, urlRef("https://mirror.example.org/story"))
	if len(e.References) != 2 {
		t.Fatalf("references = %+v, want 2 (both URLs on the same edition)", e.References)
	}
	if workCount() != 1 {
		t.Fatalf("works = %d, want 1 (a new URL never creates another edition)", workCount())
	}

	// The retirement keeps the row and the reservation in history and
	// frees the identity.
	firstID := e.References[0].ID
	edBefore := e.Revision
	e, err := db.RetireReference(ctx(t), now.Add(4*time.Second), e.ID, edBefore, firstID, strPtr("wrong site"), referenceActor, nil)
	if err != nil {
		t.Fatalf("RetireReference: %v", err)
	}
	if len(e.References) != 1 {
		t.Fatalf("references = %+v, want the surviving reference only", e.References)
	}
	var (
		rows   int
		ret    bool
		reason string
	)
	if err := db.DB().QueryRow(
		`SELECT COUNT(*), retired_at IS NOT NULL, COALESCE(retired_reason, '') FROM external_references WHERE id = ?`, firstID).Scan(&rows, &ret, &reason); err != nil {
		t.Fatalf("read retired reference: %v", err)
	}
	if rows != 1 || !ret || reason != "wrong site" {
		t.Fatalf("retired reference = (%d, %v, %q), want the kept row with the correction reason", rows, ret, reason)
	}
	var retiredRes int
	if err := db.DB().QueryRow(
		`SELECT COUNT(*) FROM external_identities WHERE normalized_url = ? AND retired_at IS NOT NULL`, "https://example.org/anime/story").Scan(&retiredRes); err != nil {
		t.Fatalf("count retired reservations: %v", err)
	}
	if retiredRes != 1 {
		t.Fatalf("retired reservations = %d, want 1 (the identity is retired, not erased)", retiredRes)
	}

	// The freed identity can be attached again.
	e = mustAddReference(t, db, now.Add(5*time.Second), e, urlRef("https://example.org/anime/story"))
	if len(e.References) != 2 {
		t.Fatalf("references = %+v, want the re-attached reference", e.References)
	}
}

// TestReferenceIdentityConflict proves the 0012 identity reservation
// decides conflicts: the same active identity on another edition is an
// identity conflict with no silent reassignment, the same edition plus the
// same identity with a different assertion conflicts, and the exact same
// reference is a no-op without a revision bump.
func TestReferenceIdentityConflict(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Conflict Story")
	eA, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga A")
	eB, _ := mustEdition(t, db, now.Add(2*time.Second), w, domain.KindManga, "Manga B")

	eA = mustAddReference(t, db, now.Add(3*time.Second), eA, urlRef("https://example.org/story"))

	// The same URL on another edition is an identity conflict and leaves
	// the prior state intact.
	if _, err := db.AddReference(ctx(t), now.Add(4*time.Second), eB.ID, eB.Revision, urlRef("https://example.org/story"), referenceActor, nil); !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("same url on another edition: want ErrIdentityConflict, got %v", err)
	}
	fresh, err := db.GetEdition(ctx(t), eA.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if len(fresh.References) != 1 {
		t.Fatalf("edition A references = %+v, want the prior state intact", fresh.References)
	}

	// The same identity on the same edition with a different role
	// conflicts: one identity, one active reference.
	if _, err := db.AddReference(ctx(t), now.Add(5*time.Second), eA.ID, eA.Revision,
		ReferenceRequest{Provider: "example", URL: strPtr("https://example.org/story"), Role: domain.RoleMetadata, Provenance: "manual"}, referenceActor, nil); !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("same identity different role: want ErrIdentityConflict, got %v", err)
	}

	// The exact same reference is a no-op without a revision bump.
	before := eA
	eA = mustAddReference(t, db, now.Add(6*time.Second), eA, urlRef("https://example.org/story"))
	if eA.Revision != before.Revision {
		t.Fatalf("edition revision = %d, want %d (an exact re-assertion is a no-op)", eA.Revision, before.Revision)
	}
	var rows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM external_references WHERE edition_id = ?`, eA.ID).Scan(&rows); err != nil {
		t.Fatalf("count references: %v", err)
	}
	if rows != 1 {
		t.Fatalf("reference rows = %d, want 1 (a no-op appends nothing)", rows)
	}
}

// TestReferencePromotionAndErrors proves a 0012 add-path reservation on the
// same edition is promoted into a reference without a second reservation,
// and invalid input fails before any state changes.
func TestReferencePromotionAndErrors(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Promotion Story")
	e, _ := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga P")

	// A reservation from the add path (provider-less URL identity).
	kind := domain.KindManga
	res, err := db.AttemptEntry(ctx(t), now.Add(2*time.Second), EntryAttempt{
		Titles:     []TitleInput{{Text: "Promo Story", Kind: domain.TitleOriginal, Provenance: "manual"}},
		Kind:       &kind,
		References: []ReferenceInput{{URL: strPtr("https://example.org/promo")}},
	})
	if err != nil || res.Outcome != EntryCreated || res.Edition == nil {
		t.Fatalf("AttemptEntry = (%+v, %v), want a created entry with an edition", res, err)
	}
	promo := res.Edition

	// The promotion adds the reference and no second reservation.
	fresh := mustAddReference(t, db, now.Add(3*time.Second), *promo, urlRef("https://example.org/promo"))
	promo = &fresh
	if len(promo.References) != 1 {
		t.Fatalf("references = %+v, want the promoted reference", promo.References)
	}
	var reserved int
	if err := db.DB().QueryRow(
		`SELECT COUNT(*) FROM external_identities WHERE normalized_url = ?`, "https://example.org/promo").Scan(&reserved); err != nil {
		t.Fatalf("count reservations: %v", err)
	}
	if reserved != 1 {
		t.Fatalf("reservations = %d, want 1 (no second dedupe policy)", reserved)
	}

	// Invalid input fails before any state changes.
	bad := []struct {
		name string
		in   ReferenceRequest
	}{
		{"missing provider", ReferenceRequest{URL: strPtr("https://example.org/x"), Role: domain.RoleMetadata, Provenance: "manual"}},
		{"both identity forms", ReferenceRequest{Provider: "example", ExternalID: strPtr("1"), URL: strPtr("https://example.org/x"), Role: domain.RoleMetadata, Provenance: "manual"}},
		{"neither identity form", ReferenceRequest{Provider: "example", Role: domain.RoleMetadata, Provenance: "manual"}},
		{"blank external id", ReferenceRequest{Provider: "example", ExternalID: strPtr("  "), Role: domain.RoleMetadata, Provenance: "manual"}},
		{"dangerous scheme", urlRef("javascript:alert(1)")},
		{"file scheme", urlRef("file:///etc/passwd")},
		{"unknown role", ReferenceRequest{Provider: "example", URL: strPtr("https://example.org/x"), Role: domain.ReferenceRole("hologram"), Provenance: "manual"}},
		{"blank provenance", ReferenceRequest{Provider: "example", URL: strPtr("https://example.org/x"), Role: domain.RoleMetadata, Provenance: "   "}},
		{"blank language", ReferenceRequest{Provider: "example", URL: strPtr("https://example.org/x"), Role: domain.RoleMetadata, Provenance: "manual", Language: strPtr(" ")}},
	}
	edBefore := e
	for _, c := range bad {
		if _, err := db.AddReference(ctx(t), now, e.ID, e.Revision, c.in, referenceActor, nil); !errors.Is(err, ErrInvalidReference) && !errors.Is(err, ErrInvalidURL) {
			t.Fatalf("%s: want ErrInvalidReference/ErrInvalidURL, got %v", c.name, err)
		}
	}
	fresh, err = db.GetEdition(ctx(t), e.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if fresh.Revision != edBefore.Revision || len(fresh.References) != 0 {
		t.Fatalf("edition = (%d, %+v), want the prior state intact", fresh.Revision, fresh.References)
	}

	// A missing edition and a stale revision are distinct errors.
	if _, err := db.AddReference(ctx(t), now, "00000000-0000-4000-8000-000000000000", 0, urlRef("https://example.org/x"), referenceActor, nil); !errors.Is(err, ErrEditionNotFound) {
		t.Fatalf("missing edition: want ErrEditionNotFound, got %v", err)
	}
	if _, err := db.AddReference(ctx(t), now, e.ID, e.Revision+5, urlRef("https://example.org/x"), referenceActor, nil); !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("stale revision: want ErrRevisionMismatch, got %v", err)
	}
}

// TestReferenceCanonicalEquivalence proves URL normalization decides
// identity: canonical-equivalent URLs (host case, default port, fragment)
// are one identity, while http and https stay distinct.
func TestReferenceCanonicalEquivalence(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Canonical Story")
	eA, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga C")
	eB, _ := mustEdition(t, db, now.Add(2*time.Second), w, domain.KindManga, "Manga D")

	// https://example.org:443/story#top normalizes to the same identity as
	// https://example.org/story.
	eA = mustAddReference(t, db, now.Add(3*time.Second), eA, urlRef("https://example.org/story"))
	equiv := urlRef("https://Example.ORG:443/story#top")
	if _, err := db.AddReference(ctx(t), now.Add(4*time.Second), eB.ID, eB.Revision, equiv, referenceActor, nil); !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("canonical equivalent on another edition: want ErrIdentityConflict, got %v", err)
	}

	// The canonical equivalent on the same edition is the same reference.
	before := eA
	eA = mustAddReference(t, db, now.Add(5*time.Second), eA, equiv)
	if eA.Revision != before.Revision || len(eA.References) != 1 {
		t.Fatalf("edition = (%d, %+v), want the no-op canonical equivalent", eA.Revision, eA.References)
	}

	// http stays distinct from https: both attach without conflict.
	eA = mustAddReference(t, db, now.Add(6*time.Second), eA, urlRef("http://example.org/story"))
	if len(eA.References) != 2 {
		t.Fatalf("references = %+v, want http and https to stay distinct", eA.References)
	}
}

// TestReferenceRetireNotFoundAndNoop proves retiring an unknown or
// foreign-edition reference is a not-found, a retirement keeps the row
// with the first correction reason, and re-retiring is a no-op without a
// revision bump.
func TestReferenceRetireNotFoundAndNoop(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Retire Story")
	e, _ := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga T")
	e = mustAddReference(t, db, now.Add(2*time.Second), e, urlRef("https://example.org/story"))
	e = mustAddReference(t, db, now.Add(3*time.Second), e, ReferenceRequest{Provider: "example", ExternalID: strPtr("123"), Role: domain.RoleMetadata, Provenance: "manual"})
	var metaID string
	if err := db.DB().QueryRow(`SELECT id FROM external_references WHERE edition_id = ? AND external_id = '123'`, e.ID).Scan(&metaID); err != nil {
		t.Fatalf("read metadata reference: %v", err)
	}

	// An unknown reference id is a not-found and leaves the state intact.
	if _, err := db.RetireReference(ctx(t), now.Add(4*time.Second), e.ID, e.Revision, "00000000-0000-4000-8000-000000000000", nil, referenceActor, nil); !errors.Is(err, ErrReferenceNotFound) {
		t.Fatalf("unknown reference: want ErrReferenceNotFound, got %v", err)
	}
	// A reference of another edition is not found on this one.
	wo := mustWork(t, db, now.Add(5*time.Second), "Foreign Retire")
	eO, _ := mustEdition(t, db, now.Add(6*time.Second), wo, domain.KindManga, "Manga O")
	eO = mustAddReference(t, db, now.Add(7*time.Second), eO, urlRef("https://example.org/foreign"))
	if _, err := db.RetireReference(ctx(t), now.Add(8*time.Second), e.ID, e.Revision, eO.References[0].ID, nil, referenceActor, nil); !errors.Is(err, ErrReferenceNotFound) {
		t.Fatalf("foreign reference: want ErrReferenceNotFound, got %v", err)
	}

	// The retirement bumps the edition and keeps the row with the reason.
	edBefore := e.Revision
	e, err := db.RetireReference(ctx(t), now.Add(9*time.Second), e.ID, edBefore, metaID, strPtr("erroneous"), referenceActor, nil)
	if err != nil {
		t.Fatalf("RetireReference: %v", err)
	}
	if e.Revision != edBefore+1 {
		t.Fatalf("edition revision = %d, want %d (a retirement bumps the edition)", e.Revision, edBefore+1)
	}
	if len(e.References) != 1 || e.References[0].ExternalID != nil || e.References[0].URL == nil {
		t.Fatalf("references = %+v, want the surviving URL reference only", e.References)
	}
	var (
		rows   int
		reason string
	)
	if err := db.DB().QueryRow(`SELECT COUNT(*), COALESCE(retired_reason, '') FROM external_references WHERE id = ?`, metaID).Scan(&rows, &reason); err != nil {
		t.Fatalf("read retired reference: %v", err)
	}
	if rows != 1 || reason != "erroneous" {
		t.Fatalf("retired reference = (%d, %q), want the kept row with the correction reason", rows, reason)
	}

	// Re-retiring is a no-op without a bump and keeps the first reason.
	again := e
	e, err = db.RetireReference(ctx(t), now.Add(10*time.Second), e.ID, again.Revision, metaID, strPtr("again"), referenceActor, nil)
	if err != nil {
		t.Fatalf("re-RetireReference: %v", err)
	}
	if e.Revision != again.Revision {
		t.Fatalf("edition revision = %d, want %d (re-retiring is a no-op)", e.Revision, again.Revision)
	}
	if err := db.DB().QueryRow(`SELECT retired_reason FROM external_references WHERE id = ?`, metaID).Scan(&reason); err != nil {
		t.Fatalf("read reason: %v", err)
	}
	if reason != "erroneous" {
		t.Fatalf("retirement reason = %q, want the first correction reason kept", reason)
	}
}
