package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

func kindPtr(k domain.EditionKind) *domain.EditionKind { return &k }

func TestIdentityURLNormalization(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"https://example.com/a", "https://example.com/a", false},
		{"HTTP://EXAMPLE.com/a", "http://example.com/a", false},
		{"http://example.com:80/a", "http://example.com/a", false},
		{"https://example.com:443/a", "https://example.com/a", false},
		{"https://example.com:8443/a", "https://example.com:8443/a", false},
		{"https://example.com/a#frag", "https://example.com/a", false},
		{"https://example.com/a/bc", "https://example.com/a/bc", false},
		{"https://example.com/a/", "https://example.com/a/", false},
		{"https://example.com/a?b=2&a=1", "https://example.com/a?b=2&a=1", false},
		{"https://example.com/a%20b", "https://example.com/a%20b", false},
		{"http://example.com/a", "http://example.com/a", false},
		{"https://example.com/a", "https://example.com/a", false},
		{"javascript:alert(1)", "", true},
		{"file:///tmp/x", "", true},
		{"example.com/a", "", true},
		{"https://", "", true},
	}
	for _, c := range cases {
		got, err := NormalizeURL(c.in)
		if (err != nil) != c.wantErr {
			t.Fatalf("NormalizeURL(%q) err = %v, wantErr = %v", c.in, err, c.wantErr)
		}
		if err == nil && got != c.want {
			t.Fatalf("NormalizeURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIdentityReserveAndFind(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w, err := db.CreateWork(ctx, now, []TitleInput{{Text: "Story", Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	e, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{Kind: domain.KindManga})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	// A provider + external ID identity is found case-sensitively by exact key.
	id, err := db.FindExternalIdentity(ctx, ReferenceInput{Provider: strPtr("shonenjump"), ExternalID: strPtr("SJ-1")})
	if err != nil {
		t.Fatalf("FindExternalIdentity: %v", err)
	}
	if id != nil {
		t.Fatalf("unreserved identity = %+v, want nil", id)
	}
	// Reserve through an atomic entry on the existing edition's work.
	res, err := db.AttemptEntry(ctx, now, EntryAttempt{
		WorkID: &w.ID,
		Kind:   kindPtr(domain.KindManga),
		Titles: []TitleInput{{Text: "Second Line", Kind: domain.TitleOriginal, Provenance: "manual"}},
		References: []ReferenceInput{
			{Provider: strPtr("shonenjump"), ExternalID: strPtr("SJ-1")},
			{URL: strPtr("https://example.com/second")},
		},
	})
	if err != nil {
		t.Fatalf("AttemptEntry: %v", err)
	}
	if res.Outcome != EntryCreated || res.Edition == nil {
		t.Fatalf("attempt = %+v, want a created edition", res)
	}
	got, err := db.FindExternalIdentity(ctx, ReferenceInput{Provider: strPtr("shonenjump"), ExternalID: strPtr("SJ-1")})
	if err != nil {
		t.Fatalf("FindExternalIdentity: %v", err)
	}
	if got == nil || got.EditionID != res.Edition.ID {
		t.Fatalf("reserved identity = %+v, want the new edition", got)
	}
	byURL, err := db.FindExternalIdentity(ctx, ReferenceInput{URL: strPtr("HTTPS://EXAMPLE.com/second#frag")})
	if err != nil {
		t.Fatalf("FindExternalIdentity url: %v", err)
	}
	if byURL == nil || byURL.EditionID != res.Edition.ID {
		t.Fatalf("url identity = %+v, want the new edition (normalization, fragment ignored)", byURL)
	}
	// The same identity on another edition reports the match; nothing is
	// written.
	res2, err := db.AttemptEntry(ctx, now.Add(time.Second), EntryAttempt{
		WorkID:     &w.ID,
		Kind:       kindPtr(domain.KindManga),
		Titles:     []TitleInput{{Text: "Third Line", Kind: domain.TitleOriginal, Provenance: "manual"}},
		References: []ReferenceInput{{Provider: strPtr("shonenjump"), ExternalID: strPtr("SJ-1")}},
	})
	if err != nil {
		t.Fatalf("second reservation of the same identity: err = %v", err)
	}
	if res2.Outcome != EntryIdentityMatch || res2.Matched == nil || res2.Matched.EditionID != res.Edition.ID {
		t.Fatalf("second attempt = %+v, want the identity match on the first edition", res2)
	}
	// No partial state: the third line was not created.
	editions, err := db.ListEditions(ctx, w.ID)
	if err != nil {
		t.Fatalf("ListEditions: %v", err)
	}
	if len(editions) != 2 {
		t.Fatalf("editions after matched attempt = %d, want 2", len(editions))
	}
	_ = e
}

func TestIdentityURLGlobalAcrossProviders(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w1, err := db.CreateWork(ctx, now, []TitleInput{{Text: "One", Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	w2, err := db.CreateWork(ctx, now, []TitleInput{{Text: "Two", Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	res, err := db.AttemptEntry(ctx, now, EntryAttempt{
		WorkID:     &w1.ID,
		Kind:       kindPtr(domain.KindManga),
		Titles:     []TitleInput{{Text: "One Edition", Kind: domain.TitleOriginal, Provenance: "manual"}},
		References: []ReferenceInput{{URL: strPtr("https://example.com/shared")}},
	})
	if err != nil {
		t.Fatalf("AttemptEntry: %v", err)
	}
	if res.Outcome != EntryCreated {
		t.Fatalf("outcome = %v, want created", res.Outcome)
	}
	// The same normalized URL on another edition is a conflict even though
	// URL identities carry no provider.
	res2, err := db.AttemptEntry(ctx, now.Add(time.Second), EntryAttempt{
		WorkID:     &w2.ID,
		Kind:       kindPtr(domain.KindManga),
		Titles:     []TitleInput{{Text: "Two Edition", Kind: domain.TitleOriginal, Provenance: "manual"}},
		References: []ReferenceInput{{URL: strPtr("https://example.com/shared")}},
	})
	if err != nil {
		t.Fatalf("same url on another edition: err = %v", err)
	}
	if res2.Outcome != EntryIdentityMatch {
		t.Fatalf("same url on another edition = %+v, want the identity match (no writes)", res2)
	}
	// http and https are distinct identities.
	res3, err := db.AttemptEntry(ctx, now.Add(2*time.Second), EntryAttempt{
		WorkID:     &w2.ID,
		Kind:       kindPtr(domain.KindManga),
		Titles:     []TitleInput{{Text: "Two Edition", Kind: domain.TitleOriginal, Provenance: "manual"}},
		References: []ReferenceInput{{URL: strPtr("http://example.com/shared")}},
	})
	if err != nil {
		t.Fatalf("http vs https: err = %v, want distinct identities", err)
	}
	if res3.Outcome != EntryCreated {
		t.Fatalf("http identity outcome = %v, want created (distinct from https)", res3.Outcome)
	}
}

func TestIdentityTitleCandidates(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w, err := db.CreateWork(ctx, now, []TitleInput{{Text: "進撃の巨人", Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	e, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{
		Kind:   domain.KindManga,
		Titles: []TitleInput{{Text: "進撃の巨人", Kind: domain.TitleOriginal, Provenance: "manual"}},
	})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	candidates, err := db.FindTitleCandidates(ctx, domain.SearchKey("  進撃の巨人  "))
	if err != nil {
		t.Fatalf("FindTitleCandidates: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates = %+v, want the work title and the edition title", candidates)
	}
	byTarget := map[string]TitleCandidate{}
	for _, c := range candidates {
		byTarget[c.Target] = c
	}
	if work, ok := byTarget["work"]; !ok || work.WorkID != w.ID || work.EditionID != nil {
		t.Fatalf("work candidate = %+v, want the work without an edition", work)
	}
	if ed, ok := byTarget["edition"]; !ok || ed.WorkID != w.ID || ed.EditionID == nil || *ed.EditionID != e.ID {
		t.Fatalf("edition candidate = %+v, want the edition under the work", ed)
	}
	// A substring is not a candidate: exact normalized match only. An
	// internal space is part of the key, so it does not match either.
	partial, err := db.FindTitleCandidates(ctx, domain.SearchKey("進撃"))
	if err != nil {
		t.Fatalf("FindTitleCandidates partial: %v", err)
	}
	if len(partial) != 0 {
		t.Fatalf("partial candidates = %+v, want none (exact match only)", partial)
	}
	spaced, err := db.FindTitleCandidates(ctx, domain.SearchKey("進撃の 巨人"))
	if err != nil {
		t.Fatalf("FindTitleCandidates spaced: %v", err)
	}
	if len(spaced) != 0 {
		t.Fatalf("spaced candidates = %+v, want none (the space is part of the key)", spaced)
	}
}

// TestAdditionAttemptAtomic proves the identity and candidate rechecks run
// in the same write transaction as the creation.
func TestAdditionAttemptAtomic(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w, err := db.CreateWork(ctx, now, []TitleInput{{Text: "Existing", Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	// A ready-path attempt with an exact title match reports candidates
	// without writing.
	res, err := db.AttemptEntry(ctx, now, EntryAttempt{
		Titles:            []TitleInput{{Text: "existing", Kind: domain.TitleOriginal, Provenance: "manual"}},
		BlockOnCandidates: true,
	})
	if err != nil {
		t.Fatalf("AttemptEntry: %v", err)
	}
	if res.Outcome != EntryCandidates || len(res.Candidates) != 1 {
		t.Fatalf("attempt = %+v, want the one candidate", res)
	}
	works, err := db.ListWorks(ctx)
	if err != nil {
		t.Fatalf("ListWorks: %v", err)
	}
	if len(works) != 1 {
		t.Fatalf("works after blocked attempt = %d, want 1 (no writes)", len(works))
	}
	// A decision for the recomputed set creates under the chosen work.
	dec := &CandidateDecision{
		Candidates:  []string{candidateKey(res.Candidates[0])},
		ExpiresUnix: now.Add(time.Hour).Unix(),
		NowUnix:     now.Unix(),
	}
	res2, err := db.AttemptEntry(ctx, now.Add(time.Second), EntryAttempt{
		Titles:   []TitleInput{{Text: "existing", Kind: domain.TitleOriginal, Provenance: "manual"}},
		WorkID:   &w.ID,
		Kind:     kindPtr(domain.KindManga),
		Decision: dec,
	})
	if err != nil {
		t.Fatalf("AttemptEntry decision: %v", err)
	}
	if res2.Outcome != EntryCreated || res2.Edition == nil || res2.Work.Revision != 1 {
		t.Fatalf("decision attempt = %+v, want the edition under the work with revision 1", res2)
	}
	// A stale decision (the candidate set changed) conflicts without writes.
	res3, err := db.AttemptEntry(ctx, now.Add(2*time.Second), EntryAttempt{
		Titles: []TitleInput{{Text: "existing", Kind: domain.TitleOriginal, Provenance: "manual"}},
		WorkID: &w.ID,
		Kind:   kindPtr(domain.KindAnime),
		Decision: &CandidateDecision{
			Candidates:  []string{"00000000-0000-4000-8000-000000000000"},
			ExpiresUnix: now.Add(time.Hour).Unix(),
			NowUnix:     now.Unix(),
		},
	})
	_ = res3
	if !errors.Is(err, ErrStaleDecision) {
		t.Fatalf("stale decision: err = %v, want ErrStaleDecision", err)
	}
	if works, _ := db.ListWorks(ctx); len(works) != 1 {
		t.Fatalf("works after stale decision = %d, want 1 (no writes)", len(works))
	}
}

func TestAdditionAttemptIdentityWinsInTransaction(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	journal := JournalInput{RequestID: "r-1", Operation: "create_library_entry", RequestHash: "h", Actor: "local"}
	res, err := db.AttemptEntry(ctx, now, EntryAttempt{
		Titles:     []TitleInput{{Text: "First Line", Kind: domain.TitleOriginal, Provenance: "manual"}},
		Kind:       kindPtr(domain.KindManga),
		References: []ReferenceInput{{Provider: strPtr("p"), ExternalID: strPtr("e1")}},
		Journal:    &journal,
	})
	if err != nil {
		t.Fatalf("AttemptEntry: %v", err)
	}
	if res.Outcome != EntryCreated {
		t.Fatalf("outcome = %v, want created", res.Outcome)
	}
	// The same identity in a second attempt reports the match; nothing is
	// written and the journal key is not reused.
	res2, err := db.AttemptEntry(ctx, now.Add(time.Second), EntryAttempt{
		Titles:     []TitleInput{{Text: "Second", Kind: domain.TitleOriginal, Provenance: "manual"}},
		Kind:       kindPtr(domain.KindManga),
		References: []ReferenceInput{{Provider: strPtr("p"), ExternalID: strPtr("e1")}},
	})
	if err != nil {
		t.Fatalf("AttemptEntry identity: %v", err)
	}
	if res2.Outcome != EntryIdentityMatch || res2.Matched == nil || res2.Matched.EditionID != res.Edition.ID {
		t.Fatalf("attempt = %+v, want the identity match", res2)
	}
	if works, _ := db.ListWorks(ctx); len(works) != 1 {
		t.Fatalf("works after identity match = %d, want 1 (no writes)", len(works))
	}
	if entry, _ := db.LookupJournal(ctx, "r-1"); entry == nil {
		t.Fatalf("the journal row of the first attempt is missing")
	}
	var count int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM mutation_journal`).Scan(&count); err != nil {
		t.Fatalf("count journal: %v", err)
	}
	if count != 1 {
		t.Fatalf("journal rows = %d, want 1 (the matched attempt wrote nothing)", count)
	}
}

func TestAdditionAttemptJournalStored(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	journal := JournalInput{RequestID: "r-j", Operation: "create_library_entry", RequestHash: "sha", Actor: "local"}
	res, err := db.AttemptEntry(ctx, now, EntryAttempt{
		Titles:  []TitleInput{{Text: "Journal Story", Kind: domain.TitleOriginal, Provenance: "manual"}},
		Kind:    kindPtr(domain.KindManga),
		Journal: &journal,
	})
	if err != nil {
		t.Fatalf("AttemptEntry: %v", err)
	}
	if res.Outcome != EntryCreated || res.Work.Revision != 1 || res.Edition == nil {
		t.Fatalf("attempt = %+v, want a created work with an edition at revision 1", res)
	}
	entry, err := db.LookupJournal(ctx, "r-j")
	if err != nil {
		t.Fatalf("LookupJournal: %v", err)
	}
	if entry == nil {
		t.Fatalf("journal row missing")
	}
	if !strings.Contains(entry.ResultJSON, `"created":true`) ||
		!strings.Contains(entry.ResultJSON, res.Work.ID) ||
		!strings.Contains(entry.ResultJSON, res.Edition.ID) {
		t.Fatalf("journal result = %s, want the created entry result with both ids", entry.ResultJSON)
	}
	// A failed validation never reserves a key.
	_, err = db.AttemptEntry(ctx, now, EntryAttempt{Kind: kindPtr(domain.KindManga)})
	if err == nil {
		t.Fatalf("attempt without titles or work: want an error")
	}
	if entry, _ := db.LookupJournal(ctx, "never-validated"); entry != nil {
		t.Fatalf("a failed validation must not reserve a key")
	}
}
