package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func newTestService(t *testing.T) *Service {
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
	return NewService(db)
}

func testNow() time.Time { return time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC) }

func appStr(s string) *string { return &s }

func appKind(k domain.EditionKind) *domain.EditionKind { return &k }

func appChoice(c AdditionChoice) *AdditionChoice { return &c }

// baseReq builds a valid addition request with manual titles.
func baseReq(t *testing.T, requestID string, titles ...string) AdditionRequest {
	t.Helper()
	req := AdditionRequest{
		MutationBase: MutationBase{
			RequestID: requestID,
			Actor:     Actor{Principal: "local", Origin: domain.OriginUser},
		},
	}
	for _, text := range titles {
		req.Titles = append(req.Titles, store.TitleInput{Text: text, Kind: domain.TitleOriginal, Provenance: "manual"})
	}
	return req
}

func countWorks(t *testing.T, s *Service) int {
	t.Helper()
	works, err := s.db.ListWorks(context.Background())
	if err != nil {
		t.Fatalf("ListWorks: %v", err)
	}
	return len(works)
}

func appError(t *testing.T, err error) *AppError {
	t.Helper()
	var ae *AppError
	if !errors.As(err, &ae) {
		t.Fatalf("error = %v, want *AppError", err)
	}
	return ae
}

func additionResultsEqual(a, b AdditionResult) bool {
	if a.Created != b.Created || a.WorkID != b.WorkID || a.WorkRevision != b.WorkRevision {
		return false
	}
	if (a.Reason == nil) != (b.Reason == nil) || (a.Reason != nil && *a.Reason != *b.Reason) {
		return false
	}
	if (a.EditionID == nil) != (b.EditionID == nil) {
		return false
	}
	if a.EditionID != nil && *a.EditionID != *b.EditionID {
		return false
	}
	return true
}

// TestAdditionTitleOnlyReadyAndCreated proves title-only manual creation
// works offline: resolve reports ready and create makes a work with no
// edition and no identity.
func TestAdditionTitleOnlyReadyAndCreated(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	req := baseReq(t, "r-ready", "Offline Story")
	res, err := s.ResolveAddition(ctx, req)
	if err != nil {
		t.Fatalf("ResolveAddition: %v", err)
	}
	if res.Outcome != OutcomeReady || res.CandidateToken != nil || res.RequestFingerprint == nil {
		t.Fatalf("resolve = %+v, want ready with a fingerprint and no token", res)
	}
	created, err := s.CreateLibraryEntry(ctx, req)
	if err != nil {
		t.Fatalf("CreateLibraryEntry: %v", err)
	}
	if !created.Created || created.EditionID != nil || created.WorkRevision != 0 {
		t.Fatalf("created = %+v, want a work without an edition at revision 0", created)
	}
	w, err := s.db.GetWork(ctx, created.WorkID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if len(w.Titles) != 1 || w.Titles[0].Text != "Offline Story" || !w.Titles[0].Preferred {
		t.Fatalf("work titles = %+v, want the one preferred title", w.Titles)
	}
}

func TestAdditionTitleOnlyWithEdition(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	req := baseReq(t, "r-edition", "Edition Story")
	req.Kind = appKind(domain.KindManga)
	req.Language = appStr("ru")
	created, err := s.CreateLibraryEntry(ctx, req)
	if err != nil {
		t.Fatalf("CreateLibraryEntry: %v", err)
	}
	if !created.Created || created.EditionID == nil || created.WorkRevision != 1 {
		t.Fatalf("created = %+v, want the edition and work revision 1", created)
	}
	e, err := s.db.GetEdition(ctx, *created.EditionID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if e.Kind != domain.KindManga || e.Language == nil || *e.Language != "ru" || len(e.Titles) != 1 {
		t.Fatalf("edition = %+v, want the requested attributes and the title", e)
	}
}

// TestAdditionURLWithoutKnownIdentityRequestsTitle proves a URL-only
// addition without a matching identity is rejected and asks for a title; it
// reserves nothing.
func TestAdditionURLWithoutKnownIdentityRequestsTitle(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	req := AdditionRequest{
		MutationBase: MutationBase{
			RequestID: "r-url",
			Actor:     Actor{Principal: "local", Origin: domain.OriginUser},
		},
		References: []store.ReferenceInput{{URL: appStr("https://example.com/unknown")}},
	}
	if _, err := s.ResolveAddition(ctx, req); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("resolve url-only: want invalid_argument")
	}
	if _, err := s.CreateLibraryEntry(ctx, req); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("create url-only: want invalid_argument")
	}
	if countWorks(t, s) != 0 {
		t.Fatalf("works = %d, want 0 (no title, no known identity)", countWorks(t, s))
	}
}

// TestAdditionExistingIdentityCreatedFalse proves an exact identity match
// returns created false without writes.
func TestAdditionExistingIdentityCreatedFalse(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	first := baseReq(t, "r-existing-1", "Known Story")
	first.Kind = appKind(domain.KindManga)
	first.References = []store.ReferenceInput{{Provider: appStr("myanimelist"), ExternalID: appStr("anime/1")}}
	created, err := s.CreateLibraryEntry(ctx, first)
	if err != nil {
		t.Fatalf("CreateLibraryEntry: %v", err)
	}
	if !created.Created {
		t.Fatalf("first = %+v, want created", created)
	}
	again := baseReq(t, "r-existing-2", "Known Story")
	again.References = []store.ReferenceInput{{Provider: appStr("myanimelist"), ExternalID: appStr("anime/1")}}
	res, err := s.ResolveAddition(ctx, again)
	if err != nil {
		t.Fatalf("ResolveAddition: %v", err)
	}
	if res.Outcome != OutcomeExisting || res.WorkID == nil || *res.WorkID != created.WorkID {
		t.Fatalf("resolve = %+v, want existing with the first work", res)
	}
	result, err := s.CreateLibraryEntry(ctx, again)
	if err != nil {
		t.Fatalf("CreateLibraryEntry replay-of-identity: %v", err)
	}
	if result.Created || result.Reason == nil || *result.Reason != "already_exists" ||
		result.WorkID != created.WorkID || result.EditionID == nil || *result.EditionID != *created.EditionID {
		t.Fatalf("result = %+v, want created:false already_exists for the same work and edition", result)
	}
	if countWorks(t, s) != 1 {
		t.Fatalf("works = %d, want 1 (the identity match writes nothing)", countWorks(t, s))
	}
}

// TestAdditionContradictoryIdentityFailsWithoutWrites proves contradictory
// explicit work/kind/language against a matched identity is an
// identity_conflict with no writes.
func TestAdditionContradictoryIdentityFailsWithoutWrites(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	first := baseReq(t, "r-contradict-1", "Contradicted")
	first.Kind = appKind(domain.KindManga)
	first.Language = appStr("ru")
	first.References = []store.ReferenceInput{{Provider: appStr("p"), ExternalID: appStr("e")}}
	created, err := s.CreateLibraryEntry(ctx, first)
	if err != nil {
		t.Fatalf("CreateLibraryEntry: %v", err)
	}
	mismatchedKind := baseReq(t, "r-contradict-2", "Contradicted")
	mismatchedKind.Kind = appKind(domain.KindAnime)
	mismatchedKind.References = first.References
	if _, err := s.ResolveAddition(ctx, mismatchedKind); appError(t, err).Code != ErrIdentityConflict {
		t.Fatalf("kind contradiction: want identity_conflict")
	}
	mismatchedLang := baseReq(t, "r-contradict-3", "Contradicted")
	mismatchedLang.Language = appStr("en")
	mismatchedLang.References = first.References
	if _, err := s.CreateLibraryEntry(ctx, mismatchedLang); appError(t, err).Code != ErrIdentityConflict {
		t.Fatalf("language contradiction: want identity_conflict")
	}
	// A work contradiction surfaces on the add_edition decision when an
	// identity is reserved concurrently between resolve and create: a valid
	// token (title + kind + reference), an explicit different work, and a
	// reference whose identity now belongs to another work's edition.
	base := baseReq(t, "r-contradict-base", "Contradicted Two")
	baseWork, err := s.CreateLibraryEntry(ctx, base)
	if err != nil {
		t.Fatalf("CreateLibraryEntry base: %v", err)
	}
	other := baseReq(t, "r-contradict-other", "Unrelated Work")
	otherWork, err := s.CreateLibraryEntry(ctx, other)
	if err != nil {
		t.Fatalf("CreateLibraryEntry other work: %v", err)
	}
	refs := []store.ReferenceInput{{Provider: appStr("p2"), ExternalID: appStr("e2")}}
	probe := baseReq(t, "r-contradict-probe", "Contradicted Two")
	probe.Kind = appKind(domain.KindManga)
	probe.References = refs
	res, err := s.ResolveAddition(ctx, probe)
	if err != nil {
		t.Fatalf("ResolveAddition probe: %v", err)
	}
	if res.Outcome != OutcomeCandidates {
		t.Fatalf("probe = %+v, want candidates while the identity is unreserved", res)
	}
	// The concurrent reservation lands while the decision is in flight.
	if _, err := s.db.AttemptEntry(ctx, testNow(), store.EntryAttempt{
		Titles:     []store.TitleInput{{Text: "Contradicted Line", Kind: domain.TitleOriginal, Provenance: "manual"}},
		Kind:       appKind(domain.KindManga),
		WorkID:     &baseWork.WorkID,
		References: refs,
	}); err != nil {
		t.Fatalf("concurrent reservation: %v", err)
	}
	mismatchedWork := baseReq(t, "r-contradict-4", "Contradicted Two")
	mismatchedWork.WorkID = &otherWork.WorkID
	mismatchedWork.Kind = appKind(domain.KindManga)
	mismatchedWork.References = refs
	mismatchedWork.Choice = appChoice(ChoiceAddEdition)
	mismatchedWork.CandidateToken = res.CandidateToken
	if _, err := s.CreateLibraryEntry(ctx, mismatchedWork); appError(t, err).Code != ErrIdentityConflict {
		t.Fatalf("work contradiction: want identity_conflict")
	}
	if countWorks(t, s) != 3 {
		t.Fatalf("works = %d, want 3 (contradictions write nothing new)", countWorks(t, s))
	}
	if _, err := s.db.GetWork(ctx, created.WorkID); err != nil {
		t.Fatalf("the original work must survive: %v", err)
	}
}

// TestAdditionSameTitleCandidatesThenChoices proves exact title matches are
// candidates, and each explicit choice behaves: add_edition adds a distinct
// edition under the work, create_distinct makes a new work, use_existing
// returns the work.
func TestAdditionSameTitleCandidatesThenChoices(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	seed := baseReq(t, "r-seed", "Same Title")
	seeded, err := s.CreateLibraryEntry(ctx, seed)
	if err != nil {
		t.Fatalf("CreateLibraryEntry seed: %v", err)
	}

	// use_existing on the seed work: resolve first (no choice), then decide.
	probe := baseReq(t, "r-use-probe", "Same Title")
	res, err := s.ResolveAddition(ctx, probe)
	if err != nil {
		t.Fatalf("ResolveAddition: %v", err)
	}
	if res.Outcome != OutcomeCandidates || len(res.Candidates) != 1 || res.CandidateToken == nil {
		t.Fatalf("resolve = %+v, want one candidate with a token", res)
	}
	use := baseReq(t, "r-use", "Same Title")
	use.Choice = appChoice(ChoiceUseExisting)
	use.CandidateToken = res.CandidateToken
	used, err := s.CreateLibraryEntry(ctx, use)
	if err != nil {
		t.Fatalf("CreateLibraryEntry use_existing: %v", err)
	}
	if used.Created || used.WorkID != seeded.WorkID || used.EditionID != nil {
		t.Fatalf("use_existing = %+v, want the seed work, no edition, created false", used)
	}

	// add_edition: a distinct manga line under the same work. The probe
	// carries the edition attributes so the token fingerprint matches.
	addProbe := baseReq(t, "r-add-probe", "Same Title")
	addProbe.Kind = appKind(domain.KindManga)
	resAdd, err := s.ResolveAddition(ctx, addProbe)
	if err != nil {
		t.Fatalf("ResolveAddition add probe: %v", err)
	}
	add := baseReq(t, "r-add-ed", "Same Title")
	add.WorkID = &seeded.WorkID
	add.Kind = appKind(domain.KindManga)
	add.Choice = appChoice(ChoiceAddEdition)
	add.CandidateToken = resAdd.CandidateToken
	added, err := s.CreateLibraryEntry(ctx, add)
	if err != nil {
		t.Fatalf("CreateLibraryEntry add_edition: %v", err)
	}
	if !added.Created || added.WorkID != seeded.WorkID || added.EditionID == nil || added.WorkRevision != 1 {
		t.Fatalf("add_edition = %+v, want the new edition under the seed work at revision 1", added)
	}

	// create_distinct: the anime line becomes a separate work. A fresh
	// resolve now sees the work and the edition as candidates.
	distinct := baseReq(t, "r-distinct", "Same Title")
	distinct.Kind = appKind(domain.KindAnime)
	res, err = s.ResolveAddition(ctx, distinct)
	if err != nil {
		t.Fatalf("ResolveAddition distinct: %v", err)
	}
	if len(res.Candidates) < 2 {
		t.Fatalf("resolve = %+v, want the work and the edition as candidates", res)
	}
	distinct.Choice = appChoice(ChoiceCreateDistinct)
	distinct.CandidateToken = res.CandidateToken
	made, err := s.CreateLibraryEntry(ctx, distinct)
	if err != nil {
		t.Fatalf("CreateLibraryEntry create_distinct: %v", err)
	}
	if !made.Created || made.WorkID == seeded.WorkID || made.EditionID == nil {
		t.Fatalf("create_distinct = %+v, want a new work with an edition", made)
	}
	if countWorks(t, s) != 2 {
		t.Fatalf("works = %d, want 2", countWorks(t, s))
	}
	// A create without a choice while candidates exist is a duplicate
	// candidate, never a silent merge.
	same := baseReq(t, "r-same", "same title")
	if _, err := s.CreateLibraryEntry(ctx, same); appError(t, err).Code != ErrDuplicateCandidate {
		t.Fatalf("create without decision: want duplicate_candidate")
	}
	if countWorks(t, s) != 2 {
		t.Fatalf("works = %d, want 2 (no silent title merge)", countWorks(t, s))
	}
}

// TestAdditionStaleDecisionFailsWithoutWrites proves stale candidate
// decisions conflict: a changed candidate set and a changed request both
// fail without writes.
func TestAdditionStaleDecisionFailsWithoutWrites(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	seed := baseReq(t, "r-stale-seed", "Stale Title")
	seeded, err := s.CreateLibraryEntry(ctx, seed)
	if err != nil {
		t.Fatalf("CreateLibraryEntry seed: %v", err)
	}
	req := baseReq(t, "r-stale", "Stale Title")
	req.Kind = appKind(domain.KindManga)
	res, err := s.ResolveAddition(ctx, req)
	if err != nil {
		t.Fatalf("ResolveAddition: %v", err)
	}
	if res.Outcome != OutcomeCandidates {
		t.Fatalf("resolve = %+v, want candidates", res)
	}
	// A concurrent creation changes the candidate set.
	other := baseReq(t, "r-stale-other", "Stale Title")
	other.Kind = appKind(domain.KindAnime)
	resOther, err := s.ResolveAddition(ctx, other)
	if err != nil {
		t.Fatalf("ResolveAddition other: %v", err)
	}
	other.Choice = appChoice(ChoiceCreateDistinct)
	other.CandidateToken = resOther.CandidateToken
	if _, err := s.CreateLibraryEntry(ctx, other); err != nil {
		t.Fatalf("CreateLibraryEntry other: %v", err)
	}
	req.CandidateToken = res.CandidateToken
	req.Choice = appChoice(ChoiceCreateDistinct)
	if _, err := s.CreateLibraryEntry(ctx, req); appError(t, err).Code != ErrConflict {
		t.Fatalf("changed candidate set: want conflict")
	}
	// A changed request fingerprint conflicts too.
	changed := baseReq(t, "r-stale-changed", "Different Title")
	changed.Kind = appKind(domain.KindManga)
	changed.Choice = appChoice(ChoiceCreateDistinct)
	changed.CandidateToken = res.CandidateToken
	if _, err := s.CreateLibraryEntry(ctx, changed); appError(t, err).Code != ErrConflict {
		t.Fatalf("changed request: want conflict")
	}
	if countWorks(t, s) != 2 {
		t.Fatalf("works = %d, want 2 (stale decisions write nothing)", countWorks(t, s))
	}
	_ = seeded
}

// TestAdditionExpiredDecisionConflicts proves a candidate decision outside
// its short window conflicts.
func TestAdditionExpiredDecisionConflicts(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	seed := baseReq(t, "r-exp-seed", "Expired Title")
	if _, err := s.CreateLibraryEntry(ctx, seed); err != nil {
		t.Fatalf("CreateLibraryEntry seed: %v", err)
	}
	s.now = func() time.Time { return testNow() }
	req := baseReq(t, "r-exp", "Expired Title")
	req.Kind = appKind(domain.KindManga)
	res, err := s.ResolveAddition(ctx, req)
	if err != nil {
		t.Fatalf("ResolveAddition: %v", err)
	}
	req.CandidateToken = res.CandidateToken
	req.Choice = appChoice(ChoiceCreateDistinct)
	s.now = func() time.Time { return testNow().Add(CandidateTTL + time.Minute) }
	if _, err := s.CreateLibraryEntry(ctx, req); appError(t, err).Code != ErrConflict {
		t.Fatalf("expired decision: want conflict")
	}
}

// TestMutationIdenticalRetryReturnsStoredResult proves an exact retry with
// the same idempotency key and payload returns the original result and
// creates nothing new.
func TestMutationIdenticalRetryReturnsStoredResult(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	req := baseReq(t, "key-1", "Retry Story")
	req.Kind = appKind(domain.KindManga)
	first, err := s.CreateLibraryEntry(ctx, req)
	if err != nil {
		t.Fatalf("CreateLibraryEntry: %v", err)
	}
	second, err := s.CreateLibraryEntry(ctx, req)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !additionResultsEqual(second, first) {
		t.Fatalf("retry = %+v, want the stored original %+v", second, first)
	}
	if countWorks(t, s) != 1 {
		t.Fatalf("works = %d, want 1 (the retry writes nothing)", countWorks(t, s))
	}
}

// TestMutationKeyCollisionConflicts proves the same idempotency key with a
// different payload is a conflict without writes.
func TestMutationKeyCollisionConflicts(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	first := baseReq(t, "key-2", "Collision A")
	if _, err := s.CreateLibraryEntry(ctx, first); err != nil {
		t.Fatalf("CreateLibraryEntry: %v", err)
	}
	other := baseReq(t, "key-2", "Collision B")
	if _, err := s.CreateLibraryEntry(ctx, other); appError(t, err).Code != ErrConflict {
		t.Fatalf("key collision: want conflict")
	}
	// A different choice is a different payload as well.
	same := baseReq(t, "key-2", "Collision A")
	same.Choice = appChoice(ChoiceCreateDistinct)
	same.CandidateToken = appStr("token")
	if _, err := s.CreateLibraryEntry(ctx, same); appError(t, err).Code != ErrConflict {
		t.Fatalf("key with different choice: want conflict")
	}
	if countWorks(t, s) != 1 {
		t.Fatalf("works = %d, want 1", countWorks(t, s))
	}
}

// TestMutationFailedValidationDoesNotReserveKey proves a failed validation
// leaves the key free for a corrected request.
func TestMutationFailedValidationDoesNotReserveKey(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	empty := baseReq(t, "key-3")
	if _, err := s.CreateLibraryEntry(ctx, empty); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("empty request: want invalid_argument")
	}
	valid := baseReq(t, "key-3", "Valid After Failure")
	created, err := s.CreateLibraryEntry(ctx, valid)
	if err != nil {
		t.Fatalf("CreateLibraryEntry: %v, a failed validation must not reserve the key", err)
	}
	if !created.Created {
		t.Fatalf("created = %+v, want created", created)
	}
}

// TestAdditionResponseLoss proves a lost response is recovered: the retry
// returns the identical original result and the journal holds one row.
func TestAdditionResponseLoss(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	req := baseReq(t, "key-loss", "Lost Response")
	req.Kind = appKind(domain.KindManga)
	original, err := s.CreateLibraryEntry(ctx, req)
	if err != nil {
		t.Fatalf("CreateLibraryEntry: %v", err)
	}
	// The response is lost: the client never saw it. Retry the same request.
	recovered, err := s.CreateLibraryEntry(ctx, req)
	if err != nil {
		t.Fatalf("recovery retry: %v", err)
	}
	if !additionResultsEqual(recovered, original) {
		t.Fatalf("recovered = %+v, want the original result %+v", recovered, original)
	}
	entry, err := s.db.LookupJournal(ctx, "key-loss")
	if err != nil {
		t.Fatalf("LookupJournal: %v", err)
	}
	if entry == nil {
		t.Fatalf("journal row missing for the mutation")
	}
	if !strings.Contains(entry.ResultJSON, original.WorkID) || !strings.Contains(entry.ResultJSON, *original.EditionID) {
		t.Fatalf("journal result = %s, want the created ids", entry.ResultJSON)
	}
	var count int
	if err := s.db.DB().QueryRow(`SELECT COUNT(*) FROM mutation_journal`).Scan(&count); err != nil {
		t.Fatalf("count journal: %v", err)
	}
	if count != 1 {
		t.Fatalf("journal rows = %d, want 1", count)
	}
	if countWorks(t, s) != 1 {
		t.Fatalf("works = %d, want 1", countWorks(t, s))
	}
}

// TestAdditionIdentityMatchAfterConcurrentCommitReplays proves the
// post-commit identity-match path consults the journal: when the in-
// transaction identity recheck meets a reservation committed by a
// concurrent identical create (after this call's pre-check lookup), the
// stored created result is replayed, and a different payload on the same
// key conflicts instead of surfacing as existing or a storage error.
func TestAdditionIdentityMatchAfterConcurrentCommitReplays(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	original := baseReq(t, "r-race", "Raced Story")
	original.Kind = appKind(domain.KindManga)
	original.References = []store.ReferenceInput{{Provider: appStr("p"), ExternalID: appStr("e1")}}
	created, err := s.CreateLibraryEntry(ctx, original)
	if err != nil {
		t.Fatalf("CreateLibraryEntry: %v", err)
	}
	if !created.Created {
		t.Fatalf("created = %+v, want created", created)
	}
	// Directly exercise the identity-match path, bypassing the pre-check
	// replay: the same key and payload must return the stored result.
	got, err := s.createEntry(ctx, original, nil, nil)
	if err != nil {
		t.Fatalf("createEntry identical: %v", err)
	}
	if !additionResultsEqual(got, created) {
		t.Fatalf("identical retry = %+v, want the stored result %+v", got, created)
	}
	// A different payload on the same key is a key-collision conflict.
	mismatch := original
	mismatch.Titles = []store.TitleInput{{Text: "Other Title", Kind: domain.TitleOriginal, Provenance: "manual"}}
	if _, err := s.createEntry(ctx, mismatch, nil, nil); err == nil {
		t.Fatal("createEntry different payload: want a conflict")
	} else if ae := appError(t, err); ae.Code != ErrConflict {
		t.Fatalf("error = %+v, want conflict", ae)
	}
}

// TestAdditionTitleOnlyConcurrentCommitReplays proves the plain title-only
// create path: when the in-transaction candidate recheck meets the winner's
// committed title, the stored created result is replayed instead of
// surfacing as a candidate match, and a different payload on the same key
// conflicts.
func TestAdditionTitleOnlyConcurrentCommitReplays(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	original := baseReq(t, "t-race", "Title Race")
	created, err := s.CreateLibraryEntry(ctx, original)
	if err != nil {
		t.Fatalf("CreateLibraryEntry: %v", err)
	}
	if !created.Created {
		t.Fatalf("created = %+v, want created", created)
	}
	// Directly exercise the candidate path, bypassing the pre-check
	// replay: the same key and payload must return the stored result.
	got, err := s.createEntry(ctx, original, nil, nil)
	if err != nil {
		t.Fatalf("createEntry identical: %v", err)
	}
	if !additionResultsEqual(got, created) {
		t.Fatalf("identical retry = %+v, want the stored result %+v", got, created)
	}
	mismatch := baseReq(t, "t-race", "Different Title")
	if _, err := s.createEntry(ctx, mismatch, nil, nil); err == nil {
		t.Fatal("createEntry different payload: want a conflict")
	} else if ae := appError(t, err); ae.Code != ErrConflict {
		t.Fatalf("error = %+v, want conflict", ae)
	}
}

// TestAdditionUseExistingDisambiguatesMultipleWorks proves use_existing can
// complete when the candidates span multiple distinct works — the state the
// supported create_distinct flow itself produces.
func TestAdditionUseExistingDisambiguatesMultipleWorks(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	first := baseReq(t, "twin-1", "Twin Story")
	created, err := s.CreateLibraryEntry(ctx, first)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	// The supported create_distinct flow creates a second same-title work.
	res, err := s.ResolveAddition(ctx, baseReq(t, "twin-2", "Twin Story"))
	if err != nil {
		t.Fatalf("ResolveAddition: %v", err)
	}
	if res.Outcome != OutcomeCandidates || res.CandidateToken == nil {
		t.Fatalf("resolve = %+v, want candidates with a token", res)
	}
	distinct := baseReq(t, "twin-2", "Twin Story")
	distinct.Choice = appChoice(ChoiceCreateDistinct)
	distinct.CandidateToken = res.CandidateToken
	second, err := s.CreateLibraryEntry(ctx, distinct)
	if err != nil {
		t.Fatalf("create_distinct: %v", err)
	}
	if !second.Created || second.WorkID == created.WorkID {
		t.Fatalf("create_distinct = %+v, want a new distinct work", second)
	}
	// Two same-title works: use_existing must accept an explicit work_id.
	again, err := s.ResolveAddition(ctx, baseReq(t, "twin-3", "Twin Story"))
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if again.Outcome != OutcomeCandidates || len(again.Candidates) != 2 || again.CandidateToken == nil {
		t.Fatalf("second resolve = %+v, want two work candidates", again)
	}
	pick := baseReq(t, "twin-3", "Twin Story")
	pick.WorkID = &second.WorkID
	pick.Choice = appChoice(ChoiceUseExisting)
	pick.CandidateToken = again.CandidateToken
	got, err := s.CreateLibraryEntry(ctx, pick)
	if err != nil {
		t.Fatalf("use_existing: %v", err)
	}
	if got.Created || got.WorkID != second.WorkID {
		t.Fatalf("use_existing = %+v, want the picked work as existing", got)
	}
	// A work_id outside the candidate set conflicts without writes.
	absent, err := domain.NewUUID()
	if err != nil {
		t.Fatalf("NewUUID: %v", err)
	}
	stale := baseReq(t, "twin-4", "Twin Story")
	stale.WorkID = &absent
	stale.Choice = appChoice(ChoiceUseExisting)
	stale.CandidateToken = again.CandidateToken
	if _, err := s.CreateLibraryEntry(ctx, stale); err == nil {
		t.Fatal("use_existing with a non-candidate work: want a conflict")
	} else if ae := appError(t, err); ae.Code != ErrConflict {
		t.Fatalf("error = %+v, want conflict", ae)
	}
	if countWorks(t, s) != 2 {
		t.Fatalf("works = %d, want 2 (no writes from the rejected pick)", countWorks(t, s))
	}
}

// TestAdditionUseExistingReservesKey proves the no-write outcome reserves
// the request key: an exact retry replays the stored result and a
// different payload on the same key conflicts.
func TestAdditionUseExistingReservesKey(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	if _, err := s.CreateLibraryEntry(ctx, baseReq(t, "u-1", "Reserve Story")); err != nil {
		t.Fatalf("first create: %v", err)
	}
	res, err := s.ResolveAddition(ctx, baseReq(t, "u-2", "Reserve Story"))
	if err != nil || res.Outcome != OutcomeCandidates || res.CandidateToken == nil {
		t.Fatalf("resolve = %+v err = %v, want candidates with a token", res, err)
	}
	pick := baseReq(t, "u-2", "Reserve Story")
	pick.Choice = appChoice(ChoiceUseExisting)
	pick.CandidateToken = res.CandidateToken
	got, err := s.CreateLibraryEntry(ctx, pick)
	if err != nil {
		t.Fatalf("use_existing: %v", err)
	}
	if got.Created || got.Reason == nil || *got.Reason != "already_exists" {
		t.Fatalf("use_existing = %+v, want the existing result", got)
	}
	// The key is reserved: an exact retry replays the stored result.
	retry, err := s.CreateLibraryEntry(ctx, pick)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !additionResultsEqual(retry, got) {
		t.Fatalf("retry = %+v, want the stored existing result %+v", retry, got)
	}
	// A different payload on the reserved key conflicts.
	other := baseReq(t, "u-2", "Reserve Story")
	other.Choice = appChoice(ChoiceCreateDistinct)
	other.CandidateToken = res.CandidateToken
	if _, err := s.CreateLibraryEntry(ctx, other); err == nil {
		t.Fatal("different payload on the reserved key: want a conflict")
	} else if ae := appError(t, err); ae.Code != ErrConflict {
		t.Fatalf("error = %+v, want conflict", ae)
	}
	if countWorks(t, s) != 1 {
		t.Fatalf("works = %d, want 1 (nothing new was created)", countWorks(t, s))
	}
}

// TestAdditionAddEditionWithoutTitlePassesValidation proves an add_edition
// request under an explicit work no longer fails the title-or-identity
// check; the missing candidate token is the next reported problem.
func TestAdditionAddEditionWithoutTitlePassesValidation(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	work := baseReq(t, "work-only", "Host Story")
	created, err := s.CreateLibraryEntry(ctx, work)
	if err != nil {
		t.Fatalf("CreateLibraryEntry: %v", err)
	}
	req := AdditionRequest{
		MutationBase: MutationBase{
			RequestID: "work-only-2",
			Actor:     Actor{Principal: "local", Origin: domain.OriginUser},
		},
		WorkID: &created.WorkID,
		Kind:   appKind(domain.KindManga),
		Choice: appChoice(ChoiceAddEdition),
	}
	if _, err := s.CreateLibraryEntry(ctx, req); err == nil {
		t.Fatal("want an error")
	} else if ae := appError(t, err); ae.Code != ErrInvalidArgument || !strings.Contains(ae.Message, "token") {
		t.Fatalf("error = %+v, want invalid_argument about the candidate token", ae)
	}
}
