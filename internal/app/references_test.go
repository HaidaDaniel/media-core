package app

import (
	"context"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

func mustAddReferenceView(t *testing.T, s *Service, req AddReferenceRequest) EditionView {
	t.Helper()
	view, err := s.AddReference(context.Background(), req)
	if err != nil {
		t.Fatalf("AddReference(%+v): %v", req, err)
	}
	return view
}

func mustRetireReferenceView(t *testing.T, s *Service, req RetireReferenceRequest) EditionView {
	t.Helper()
	view, err := s.RetireReference(context.Background(), req)
	if err != nil {
		t.Fatalf("RetireReference(%+v): %v", req, err)
	}
	return view
}

// TestReferenceAddRetireApp proves the service reference lifecycle: adding
// renders the frozen reference shape in the edition view, an exact retry
// replays without a second row, the same key with a different payload
// conflicts, and retirement drops the reference from the view while the
// row stays in history.
func TestReferenceAddRetireApp(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	w := mustCreateWork(t, s, "Reference App")
	e := addEdition(t, s, w.ID, domain.KindAnime, "Anime R")

	ev := mustAddReferenceView(t, s, AddReferenceRequest{
		MutationBase:  mustBase(t, "req-ref-1", e.Revision),
		EditionID:     e.ID,
		Provider:      "example",
		URL:           strPtr("https://example.org/anime/r"),
		Role:          domain.RolePublication,
		Authoritative: true,
		Language:      strPtr("ja"),
		Provenance:    "manual",
	})
	if len(ev.References) != 1 {
		t.Fatalf("references = %+v, want 1", ev.References)
	}
	r := ev.References[0]
	if r.Provider != "example" || r.URL == nil || *r.URL != "https://example.org/anime/r" ||
		r.Role != domain.RolePublication || !r.Authoritative || r.Language == nil || *r.Language != "ja" ||
		r.Provenance != "manual" || r.RetiredAt != nil || r.ExternalID != nil {
		t.Fatalf("reference = %+v, want the frozen reference shape", r)
	}

	// The exact retry replays without a second row.
	replayed := mustAddReferenceView(t, s, AddReferenceRequest{
		MutationBase:  mustBase(t, "req-ref-1", e.Revision),
		EditionID:     e.ID,
		Provider:      "example",
		URL:           strPtr("https://example.org/anime/r"),
		Role:          domain.RolePublication,
		Authoritative: true,
		Language:      strPtr("ja"),
		Provenance:    "manual",
	})
	if replayed.Revision != ev.Revision {
		t.Fatalf("replayed revision = %d, want %d (a retry appends nothing)", replayed.Revision, ev.Revision)
	}
	var rows int
	if err := s.db.DB().QueryRow(`SELECT COUNT(*) FROM external_references WHERE edition_id = ?`, e.ID).Scan(&rows); err != nil {
		t.Fatalf("count references: %v", err)
	}
	if rows != 1 {
		t.Fatalf("reference rows = %d, want 1 after the replay", rows)
	}

	// The same key with a different payload conflicts.
	if _, err := s.AddReference(ctx, AddReferenceRequest{
		MutationBase: mustBase(t, "req-ref-1", ev.Revision),
		EditionID:    e.ID,
		Provider:     "example",
		URL:          strPtr("https://example.org/other"),
		Role:         domain.RoleMetadata,
		Provenance:   "manual",
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("same key different payload: want conflict, got %v", err)
	}

	// The retirement drops the reference from the view; the row stays.
	ev = mustRetireReferenceView(t, s, RetireReferenceRequest{
		MutationBase: mustBase(t, "req-ref-retire", ev.Revision),
		EditionID:    e.ID,
		ReferenceID:  r.ID,
		Reason:       strPtr("wrong site"),
	})
	if len(ev.References) != 0 {
		t.Fatalf("references = %+v, want empty after the retirement", ev.References)
	}
	if err := s.db.DB().QueryRow(`SELECT COUNT(*) FROM external_references WHERE id = ? AND retired_at IS NOT NULL`, r.ID).Scan(&rows); err != nil {
		t.Fatalf("count retired: %v", err)
	}
	if rows != 1 {
		t.Fatalf("retired reference rows = %d, want 1 (history is kept)", rows)
	}
}

// TestReferenceAppErrors proves the stable error mapping on the reference
// path: invalid role, provider, provenance or identity shape is an
// invalid argument that reserves no key, a missing edition or reference is
// a not-found, a stale revision conflicts, and a foreign-edition identity
// is an identity conflict that leaves the prior state intact.
func TestReferenceAppErrors(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	w := mustCreateWork(t, s, "Reference Errors")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga E")
	e2 := addEdition(t, s, w.ID, domain.KindManga, "Manga E2")

	base := AddReferenceRequest{
		MutationBase: mustBase(t, "req-ref-bad", e.Revision),
		EditionID:    e.ID,
		Provider:     "example",
		URL:          strPtr("https://example.org/e"),
		Role:         domain.RoleMetadata,
		Provenance:   "manual",
	}
	bad := base
	bad.Role = domain.ReferenceRole("hologram")
	bad.RequestID = "req-ref-bad-role"
	if _, err := s.AddReference(ctx, bad); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("unknown role: want invalid_argument, got %v", err)
	}
	bad = base
	bad.RequestID = "req-ref-bad-prov"
	bad.Provider = "   "
	if _, err := s.AddReference(ctx, bad); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("blank provider: want invalid_argument, got %v", err)
	}
	bad = base
	bad.RequestID = "req-ref-bad-prov2"
	bad.Provider = "example"
	bad.ExternalID = strPtr("1")
	if _, err := s.AddReference(ctx, bad); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("both identity forms: want invalid_argument, got %v", err)
	}
	bad = base
	bad.RequestID = "req-ref-bad-scheme"
	bad.URL = strPtr("javascript:alert(1)")
	if _, err := s.AddReference(ctx, bad); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("dangerous scheme: want invalid_argument, got %v", err)
	}
	bad = base
	bad.RequestID = "req-ref-bad-prov3"
	bad.URL = strPtr("https://example.org/e")
	bad.Provenance = "   "
	if _, err := s.AddReference(ctx, bad); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("blank provenance: want invalid_argument, got %v", err)
	}

	// The failed validations reserved no key: the original key carries the
	// valid reference afterwards.
	ev := mustAddReferenceView(t, s, base)
	if len(ev.References) != 1 {
		t.Fatalf("references = %+v, want the valid reference on the reused key", ev.References)
	}

	if _, err := s.AddReference(ctx, AddReferenceRequest{
		MutationBase: mustBase(t, "req-ref-stale", 99),
		EditionID:    e.ID,
		Provider:     "example",
		URL:          strPtr("https://example.org/e2"),
		Role:         domain.RoleMetadata,
		Provenance:   "manual",
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("stale revision: want conflict, got %v", err)
	}
	if _, err := s.AddReference(ctx, AddReferenceRequest{
		MutationBase: mustBase(t, "req-ref-me", 0),
		EditionID:    "00000000-0000-4000-8000-000000000000",
		Provider:     "example",
		URL:          strPtr("https://example.org/e2"),
		Role:         domain.RoleMetadata,
		Provenance:   "manual",
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("missing edition: want not_found, got %v", err)
	}
	// The same identity on another edition is an identity conflict with no
	// silent reassignment.
	if _, err := s.AddReference(ctx, AddReferenceRequest{
		MutationBase: mustBase(t, "req-ref-other", e2.Revision),
		EditionID:    e2.ID,
		Provider:     "example",
		URL:          strPtr("https://example.org/e"),
		Role:         domain.RoleMetadata,
		Provenance:   "manual",
	}); appError(t, err).Code != ErrIdentityConflict {
		t.Fatalf("foreign-edition identity: want identity_conflict, got %v", err)
	}
	fresh, err := s.GetEdition(ctx, e.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if len(fresh.References) != 1 {
		t.Fatalf("prior edition references = %+v, want intact after the conflict", fresh.References)
	}
	if _, err := s.RetireReference(ctx, RetireReferenceRequest{
		MutationBase: mustBase(t, "req-ref-unknown", ev.Revision),
		EditionID:    e.ID,
		ReferenceID:  "00000000-0000-4000-8000-000000000000",
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("unknown reference: want not_found, got %v", err)
	}
}

// TestReferenceRetireNoopApp proves re-retiring a reference is a no-op
// without a revision bump while the journal settles.
func TestReferenceRetireNoopApp(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Retire Noop")
	e := addEdition(t, s, w.ID, domain.KindAnime, "Anime N")
	ev := mustAddReferenceView(t, s, AddReferenceRequest{
		MutationBase: mustBase(t, "req-rr-1", e.Revision),
		EditionID:    e.ID,
		Provider:     "example",
		URL:          strPtr("https://example.org/n"),
		Role:         domain.RoleContent,
		Provenance:   "manual",
	})
	refID := ev.References[0].ID
	ev = mustRetireReferenceView(t, s, RetireReferenceRequest{
		MutationBase: mustBase(t, "req-rr-2", ev.Revision),
		EditionID:    e.ID,
		ReferenceID:  refID,
	})
	before := ev.Revision
	ev = mustRetireReferenceView(t, s, RetireReferenceRequest{
		MutationBase: mustBase(t, "req-rr-3", before),
		EditionID:    e.ID,
		ReferenceID:  refID,
	})
	if ev.Revision != before {
		t.Fatalf("revision = %d, want %d (re-retiring is a no-op)", ev.Revision, before)
	}
}
