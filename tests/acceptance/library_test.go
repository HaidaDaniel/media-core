// Scenario A and C acceptance: a watched anime stays connected to its
// unread saved manga source with note, tag, publication URL, confirmed
// adaptation link and a guessed-then-confirmed origin; self and cyclic
// source links are rejected without partial writes. Scenario C proves the
// exact-identity, candidate-decision, distinct-work, reference-attachment,
// stale-token and concurrent-exact-addition contract through the shared
// service, asserting user-visible outcomes rather than internal steps.
package acceptance

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

var acceptanceActor = app.Actor{Principal: "acceptance", Origin: domain.OriginUser}

func strP(s string) *string { return &s }
func i64P(v int64) *int64   { return &v }
func intP(v int) *int       { return &v }

func errorCode(t *testing.T, err error) app.ErrorCode {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}
	var ae *app.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("expected an app error, got %v", err)
	}
	return ae.Code
}

// TestScenarioA drives the watched-anime/unread-manga workflow through the
// shared service end to end and asserts the Scenario A contract.
func TestScenarioA(t *testing.T) {
	ctx := context.Background()
	svc, _ := openService(t)

	const (
		jpTitle = "星霜の軌跡"
		ruTitle = "След звёзд"
	)
	jp := strP("ja")
	ru := strP("ru")
	jpTitleInput := store.TitleInput{Text: jpTitle, Kind: domain.TitleOriginal, Language: jp, Preferred: true, Provenance: "manual"}

	// Create the anime with its Japanese and Russian aliases.
	anime := domain.KindAnime
	created, err := svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase: app.MutationBase{RequestID: "a-create-anime", Actor: acceptanceActor},
		Titles:       []store.TitleInput{jpTitleInput, {Text: ruTitle, Kind: domain.TitleAlias, Language: ru, Provenance: "manual"}},
		Kind:         &anime,
		Language:     jp,
	})
	if err != nil || !created.Created || created.EditionID == nil {
		t.Fatalf("create anime: created=%+v err=%v", created, err)
	}
	workID := created.WorkID
	animeID := *created.EditionID

	// Explicitly mark it completed and rate it 8/10.
	completed, err := svc.SetConsumption(ctx, app.SetConsumptionRequest{
		MutationBase:  app.MutationBase{RequestID: "a-anime-completed", Actor: acceptanceActor, ExpectedRevision: i64P(0)},
		EditionID:     animeID,
		Value:         string(domain.ConsumptionCompleted),
		Reason:        strP("watched through"),
	})
	if err != nil {
		t.Fatalf("set consumption: %v", err)
	}
	rated, err := svc.SetRating(ctx, app.SetRatingRequest{
		MutationBase:  app.MutationBase{RequestID: "a-anime-rating", Actor: acceptanceActor, ExpectedRevision: i64P(completed.Revision)},
		EditionID:     animeID,
		Value:         intP(8),
		Comment:       strP("steady, no filler"),
	})
	if err != nil {
		t.Fatalf("set rating: %v", err)
	}

	// Add the manga under the same work through the resolve + add_edition
	// decision: the exact title match must produce candidates first.
	manga := domain.KindManga
	resolved, err := svc.ResolveAddition(ctx, app.AdditionRequest{
		MutationBase: app.MutationBase{Actor: acceptanceActor},
		Titles:       []store.TitleInput{{Text: jpTitle, Kind: domain.TitleOriginal, Language: jp, Provenance: "manual"}},
		Kind:         &manga,
		Language:     jp,
	})
	if err != nil || resolved.Outcome != app.OutcomeCandidates || resolved.CandidateToken == nil {
		t.Fatalf("resolve manga: outcome=%v token=%v err=%v", resolved.Outcome, resolved.CandidateToken, err)
	}
	added, err := svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase:   app.MutationBase{RequestID: "a-add-manga", Actor: acceptanceActor},
		Titles:         []store.TitleInput{{Text: jpTitle, Kind: domain.TitleOriginal, Language: jp, Provenance: "manual"}},
		Kind:           &manga,
		Language:       jp,
		WorkID:         &workID,
		Choice:         strChoice(app.ChoiceAddEdition),
		CandidateToken: resolved.CandidateToken,
	})
	if err != nil || !added.Created || added.EditionID == nil || added.WorkID != workID {
		t.Fatalf("add manga edition: created=%+v err=%v", added, err)
	}
	mangaID := *added.EditionID

	// The manga is unread by default; keep it not_started and saved, then
	// attach the deferral note, the candidate tag and the publication URL.
	mangaView, err := svc.GetEdition(ctx, mangaID)
	if err != nil {
		t.Fatalf("get manga: %v", err)
	}
	if mangaView.Consumption == nil || mangaView.Consumption.Value != domain.ConsumptionNotStarted {
		t.Fatalf("manga default consumption = %+v, want not_started", mangaView.Consumption)
	}
	saved, err := svc.SetIntention(ctx, app.SetIntentionRequest{
		MutationBase:  app.MutationBase{RequestID: "a-manga-saved", Actor: acceptanceActor, ExpectedRevision: i64P(mangaView.Revision)},
		EditionID:     mangaID,
		Value:         string(domain.IntentionSaved),
		Reason:        strP("unread, kept for later"),
	})
	if err != nil {
		t.Fatalf("set intention: %v", err)
	}
	tagged, err := svc.SetEditionTag(ctx, app.TagRequest{
		MutationBase: app.MutationBase{RequestID: "a-manga-tag", Actor: acceptanceActor, ExpectedRevision: i64P(saved.Revision)},
		TargetID:     mangaID,
		Tag:          "manga-translation-candidate",
	})
	if err != nil {
		t.Fatalf("set tag: %v", err)
	}
	noted, err := svc.AddEditionNote(ctx, app.AddNoteRequest{
		MutationBase: app.MutationBase{RequestID: "a-manga-note", Actor: acceptanceActor, ExpectedRevision: i64P(tagged.Revision)},
		TargetID:     mangaID,
		Text:         "Defer smartphone reading until a stable reader is chosen.",
	})
	if err != nil {
		t.Fatalf("add note: %v", err)
	}
	pubURL := strP("https://manga.example.org/star-frost")
	linked, err := svc.AddReference(ctx, app.AddReferenceRequest{
		MutationBase:  app.MutationBase{RequestID: "a-manga-url", Actor: acceptanceActor, ExpectedRevision: i64P(noted.Revision)},
		EditionID:     mangaID,
		Provider:      "manga.example.org",
		URL:           pubURL,
		Role:          domain.RolePublication,
		Authoritative: true,
		Provenance:    "manual",
	})
	if err != nil || len(linked.References) != 1 || linked.References[0].URL == nil {
		t.Fatalf("add reference: refs=%v err=%v", linked.References, err)
	}

	// Confirm the anime as an adaptation of the manga with source provenance.
	relView, err := svc.AddRelation(ctx, app.AddRelationRequest{
		MutationBase:   app.MutationBase{RequestID: "a-relation", Actor: acceptanceActor, ExpectedRevision: i64P(rated.Revision)},
		Scope:          domain.ScopeEdition,
		Type:           domain.RelationAdaptationOf,
		FromID:         animeID,
		ToID:           mangaID,
		State:          domain.ClaimConfirmed,
		Provenance:     "user",
	})
	if err != nil || len(relView.Confirmed) != 1 {
		t.Fatalf("add relation: view=%+v err=%v", relView, err)
	}

	// A self source link is rejected and a cyclic one conflicts at
	// confirmation; neither leaves a partial write.
	if _, err := svc.AddRelation(ctx, app.AddRelationRequest{
		MutationBase:   app.MutationBase{RequestID: "a-relation-self", Actor: acceptanceActor, ExpectedRevision: i64P(rated.Revision)},
		Scope:          domain.ScopeEdition,
		Type:           domain.RelationAdaptationOf,
		FromID:         animeID,
		ToID:           animeID,
		State:          domain.ClaimConfirmed,
		Provenance:     "user",
	}); errorCode(t, err) != app.ErrInvalidArgument {
		t.Fatalf("self link: want invalid_argument, got %v", err)
	}
	back, err := svc.AddRelation(ctx, app.AddRelationRequest{
		MutationBase:   app.MutationBase{RequestID: "a-relation-back", Actor: acceptanceActor, ExpectedRevision: i64P(linked.Revision)},
		Scope:          domain.ScopeEdition,
		Type:           domain.RelationAdaptationOf,
		FromID:         mangaID,
		ToID:           animeID,
		State:          domain.ClaimProposed,
		Provenance:     "agent",
	})
	if err != nil || len(back.Proposed) != 1 {
		t.Fatalf("propose back link: view=%+v err=%v", back, err)
	}
	backManga, err := svc.GetEdition(ctx, mangaID)
	if err != nil {
		t.Fatalf("get manga for confirm: %v", err)
	}
	if _, err := svc.ConfirmRelation(ctx, app.ConfirmRelationRequest{
		MutationBase: app.MutationBase{RequestID: "a-relation-back-confirm", Actor: acceptanceActor, ExpectedRevision: i64P(backManga.Revision)},
		RelationID:   back.Proposed[0].ID,
	}); errorCode(t, err) != app.ErrCycleDetected {
		t.Fatalf("cyclic confirm: want cycle_detected, got %v", err)
	}
	after, err := svc.ListRelations(ctx, domain.ScopeEdition, animeID)
	if err != nil {
		t.Fatalf("list relations: %v", err)
	}
	if len(after.Confirmed) != 1 || after.Confirmed[0].FromID != animeID || after.Confirmed[0].ToID != mangaID {
		t.Fatalf("confirmed edges after cycle attempt = %+v, want only the original edge", after.Confirmed)
	}
	if len(after.Proposed) != 1 || after.Proposed[0].FromID != mangaID {
		t.Fatalf("proposed edges after cycle attempt = %+v, want the unconfirmed back edge only", after.Proposed)
	}

	// A guessed origin stays proposed until it is explicitly confirmed.
	origin, err := svc.RecordOrigin(ctx, app.RecordOriginRequest{
		MutationBase:   app.MutationBase{RequestID: "a-origin", Actor: acceptanceActor, ExpectedRevision: i64P(backManga.Revision)},
		EditionID:      mangaID,
		Kind:           domain.OriginOriginal,
		State:          domain.ClaimProposed,
		Provenance:     "guessed",
	})
	if err != nil || len(origin.Claims) != 1 || origin.Claims[0].State != domain.ClaimProposed {
		t.Fatalf("record origin: view=%+v err=%v", origin, err)
	}
	originManga, err := svc.GetEdition(ctx, mangaID)
	if err != nil {
		t.Fatalf("get manga for origin confirm: %v", err)
	}
	confirmedOrigin, err := svc.ConfirmOrigin(ctx, app.ConfirmOriginRequest{
		MutationBase:   app.MutationBase{RequestID: "a-origin-confirm", Actor: acceptanceActor, ExpectedRevision: i64P(originManga.Revision)},
		EditionID:      mangaID,
		ClaimID:        origin.Claims[0].ID,
	})
	if err != nil || confirmedOrigin.Claims[0].State != domain.ClaimConfirmed {
		t.Fatalf("confirm origin: view=%+v err=%v", confirmedOrigin, err)
	}

	// dropped / not_interested must not appear as side effects.
	finalAnime, err := svc.GetEdition(ctx, animeID)
	if err != nil {
		t.Fatalf("get anime: %v", err)
	}
	if finalAnime.Intention != nil {
		t.Fatalf("anime gained an intention: %+v", finalAnime.Intention)
	}
	finalManga, err := svc.GetEdition(ctx, mangaID)
	if err != nil {
		t.Fatalf("get manga: %v", err)
	}
	if finalManga.Consumption == nil || finalManga.Consumption.Value != domain.ConsumptionNotStarted {
		t.Fatalf("manga consumption drifted: %+v", finalManga.Consumption)
	}
	if finalManga.Intention == nil || finalManga.Intention.Value != domain.IntentionSaved {
		t.Fatalf("manga intention drifted: %+v", finalManga.Intention)
	}

	// Search by either title and navigate both directions.
	for _, query := range []string{jpTitle, ruTitle} {
		page, err := svc.SearchLibrary(ctx, app.LibraryFilter{Query: strP(query)})
		if err != nil || page.Total != 1 {
			t.Fatalf("search %q: total=%d err=%v", query, page.Total, err)
		}
		card, ok := page.Items[0].(app.WorkCard)
		if !ok || card.WorkID != workID {
			t.Fatalf("search %q: unexpected card %+v", query, page.Items[0])
		}
	}
	workView, err := svc.GetWork(ctx, workID)
	if err != nil {
		t.Fatalf("get work: %v", err)
	}
	if len(workView.Editions) != 2 {
		t.Fatalf("work editions = %+v, want the anime and the manga", workView.Editions)
	}
	if finalAnime.WorkID != workID || finalAnime.Rating == nil || finalAnime.Rating.Value == nil || *finalAnime.Rating.Value != 8 {
		t.Fatalf("anime view = %+v, want the work link and the 8/10 rating", finalAnime)
	}
	if finalManga.WorkID != workID || len(finalManga.Tags) != 1 || len(finalManga.Notes) != 1 || len(finalManga.References) != 1 {
		t.Fatalf("manga view = %+v, want the work link, tag, note and URL", finalManga)
	}

	// The status history carries both the default and the explicit state.
	history, err := svc.GetHistory(ctx, app.HistoryQuery{EditionID: &animeID, Limit: 10})
	if err != nil || history.Total < 2 {
		t.Fatalf("anime history: total=%d err=%v", history.Total, err)
	}
}

func strChoice(c app.AdditionChoice) *app.AdditionChoice { return &c }

// TestScenarioC drives the existing-work / another-edition workflow through
// the shared service and asserts the Scenario C contract.
func TestScenarioC(t *testing.T) {
	ctx := context.Background()
	svc, _ := openService(t)

	const (
		origTitle  = "Тестовый Роман"
		enTitle    = "Test Novel"
		romajiName = "Testovyi Roman"
		similar    = "Тестовый Роман 2"
	)
	ru := strP("ru")
	en := strP("en")
	novel := domain.KindLightNovel
	provider := strP("novelhub")
	externalID := strP("12345")

	// Original title plus explicit Russian, English and romaji aliases,
	// with a provider identity on the edition.
	created, err := svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase: app.MutationBase{RequestID: "c-create", Actor: acceptanceActor},
		Titles: []store.TitleInput{
			{Text: origTitle, Kind: domain.TitleOriginal, Language: ru, Preferred: true, Provenance: "manual"},
			{Text: enTitle, Kind: domain.TitleLocalized, Language: en, Provenance: "manual"},
			{Text: romajiName, Kind: domain.TitleRomanized, Provenance: "manual"},
		},
		Kind:       &novel,
		References: []store.ReferenceInput{{Provider: provider, ExternalID: externalID}},
	})
	if err != nil || !created.Created || created.EditionID == nil {
		t.Fatalf("create: created=%+v err=%v", created, err)
	}
	workID := created.WorkID
	editionID := *created.EditionID

	// Search by any alias.
	for _, query := range []string{origTitle, enTitle, romajiName} {
		page, err := svc.SearchLibrary(ctx, app.LibraryFilter{Query: strP(query)})
		if err != nil || page.Total != 1 {
			t.Fatalf("search %q: total=%d err=%v", query, page.Total, err)
		}
		card, ok := page.Items[0].(app.WorkCard)
		if !ok || card.WorkID != workID {
			t.Fatalf("search %q: unexpected card %+v", query, page.Items[0])
		}
	}

	// Re-adding the same identity returns the existing record.
	readd, err := svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase: app.MutationBase{RequestID: "c-readd", Actor: acceptanceActor},
		Titles:       []store.TitleInput{{Text: origTitle, Kind: domain.TitleOriginal, Language: ru, Provenance: "manual"}},
		References:   []store.ReferenceInput{{Provider: provider, ExternalID: externalID}},
	})
	if err != nil || readd.Created || readd.WorkID != workID {
		t.Fatalf("re-add identity: created=%+v err=%v", readd, err)
	}
	if readd.Reason == nil || *readd.Reason != "already_exists" {
		t.Fatalf("re-add identity reason = %+v, want already_exists", readd.Reason)
	}

	// The same key with a different payload is a conflict.
	if _, err := svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase: app.MutationBase{RequestID: "c-create", Actor: acceptanceActor},
		Titles:       []store.TitleInput{{Text: "Другой Заголовок", Kind: domain.TitleOriginal, Provenance: "manual"}},
	}); errorCode(t, err) != app.ErrConflict {
		t.Fatalf("same key different payload: want conflict, got %v", err)
	}

	// A title-only duplicate reports candidates before any creation; the
	// explicit choices then decide.
	if _, err := svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase: app.MutationBase{RequestID: "c-dup-block", Actor: acceptanceActor},
		Titles:       []store.TitleInput{{Text: origTitle, Kind: domain.TitleOriginal, Language: ru, Provenance: "manual"}},
	}); errorCode(t, err) != app.ErrDuplicateCandidate {
		t.Fatalf("title-only duplicate: want duplicate_candidate, got %v", err)
	}

	resolveFor := func(requestID string, kind *domain.EditionKind) app.ResolveResult {
		t.Helper()
		req := app.AdditionRequest{
			MutationBase: app.MutationBase{Actor: acceptanceActor},
			Titles:       []store.TitleInput{{Text: origTitle, Kind: domain.TitleOriginal, Language: ru, Provenance: "manual"}},
		}
		if kind != nil {
			req.Kind = kind
		}
		res, err := svc.ResolveAddition(ctx, req)
		if err != nil || res.Outcome != app.OutcomeCandidates || res.CandidateToken == nil {
			t.Fatalf("resolve (%s): outcome=%v err=%v", requestID, res.Outcome, err)
		}
		return res
	}

	// Choosing the existing record creates nothing.
	useExisting := resolveFor("c-use-existing", nil)
	existing, err := svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase:   app.MutationBase{RequestID: "c-use-existing", Actor: acceptanceActor},
		Titles:         []store.TitleInput{{Text: origTitle, Kind: domain.TitleOriginal, Language: ru, Provenance: "manual"}},
		WorkID:         &workID,
		Choice:         strChoice(app.ChoiceUseExisting),
		CandidateToken: useExisting.CandidateToken,
	})
	if err != nil || existing.Created || existing.WorkID != workID {
		t.Fatalf("use existing: created=%+v err=%v", existing, err)
	}

	// Promote the creation-time identity reservation into a rendered
	// reference, then attach another URL to the same edition; neither clones
	// it.
	view, err := svc.GetEdition(ctx, editionID)
	if err != nil {
		t.Fatalf("get edition: %v", err)
	}
	promoted, err := svc.AddReference(ctx, app.AddReferenceRequest{
		MutationBase: app.MutationBase{RequestID: "c-promote", Actor: acceptanceActor, ExpectedRevision: i64P(view.Revision)},
		EditionID:    editionID,
		Provider:     "novelhub",
		ExternalID:   externalID,
		Role:         domain.RoleMetadata,
		Provenance:   "manual",
	})
	if err != nil || len(promoted.References) != 1 {
		t.Fatalf("promote reference: refs=%d err=%v", len(promoted.References), err)
	}
	secondURL := strP("https://read.example.org/test-novel")
	withURL, err := svc.AddReference(ctx, app.AddReferenceRequest{
		MutationBase:  app.MutationBase{RequestID: "c-second-url", Actor: acceptanceActor, ExpectedRevision: i64P(promoted.Revision)},
		EditionID:     editionID,
		Provider:      "read.example.org",
		URL:           secondURL,
		Role:          domain.RoleContent,
		Provenance:    "manual",
	})
	if err != nil || len(withURL.References) != 2 {
		t.Fatalf("second reference: refs=%d err=%v", len(withURL.References), err)
	}
	workView, err := svc.GetWork(ctx, workID)
	if err != nil || len(workView.Editions) != 1 {
		t.Fatalf("work after reference attach: editions=%+v err=%v", workView.Editions, err)
	}

	// A distinct adaptation is added under the explicit work choice.
	adap := domain.KindBook
	addRes := resolveFor("c-add-adaptation", &adap)
	adapAdded, err := svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase:   app.MutationBase{RequestID: "c-add-adaptation", Actor: acceptanceActor},
		Titles:         []store.TitleInput{{Text: origTitle, Kind: domain.TitleOriginal, Language: ru, Provenance: "manual"}},
		Kind:           &adap,
		WorkID:         &workID,
		Choice:         strChoice(app.ChoiceAddEdition),
		CandidateToken: addRes.CandidateToken,
	})
	if err != nil || !adapAdded.Created || adapAdded.WorkID != workID {
		t.Fatalf("add adaptation: created=%+v err=%v", adapAdded, err)
	}

	// A similar (not exact) title is created as a distinct work and is never
	// merged into the original: nothing links the two, and re-resolving the
	// original identity still returns the original work.
	similarCreated, err := svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase: app.MutationBase{RequestID: "c-similar", Actor: acceptanceActor},
		Titles:       []store.TitleInput{{Text: similar, Kind: domain.TitleOriginal, Language: ru, Preferred: true, Provenance: "manual"}},
		Kind:         &novel,
	})
	if err != nil || !similarCreated.Created || similarCreated.WorkID == workID {
		t.Fatalf("similar title: created=%+v err=%v", similarCreated, err)
	}
	related, err := svc.Related(ctx, domain.ScopeWork, workID, 3)
	if err != nil {
		t.Fatalf("related works: %v", err)
	}
	for _, edge := range related.Edges {
		if edge.FromID == similarCreated.WorkID || edge.ToID == similarCreated.WorkID {
			t.Fatalf("similar work was linked to the original: %+v", edge)
		}
	}
	reResolve, err := svc.ResolveAddition(ctx, app.AdditionRequest{
		MutationBase: app.MutationBase{Actor: acceptanceActor},
		Titles:       []store.TitleInput{{Text: origTitle, Kind: domain.TitleOriginal, Language: ru, Provenance: "manual"}},
	})
	if err != nil || reResolve.Outcome != app.OutcomeCandidates {
		t.Fatalf("re-resolve original: outcome=%v err=%v", reResolve.Outcome, err)
	}
	foundOriginal := false
	for _, c := range reResolve.Candidates {
		if c.WorkID == workID {
			foundOriginal = true
		}
	}
	if !foundOriginal {
		t.Fatalf("re-resolve original did not return the original work: %+v", reResolve.Candidates)
	}

	// A stale candidate decision conflicts without writes: the candidate
	// set changed between the resolve and the decision.
	stale := resolveFor("c-stale", nil)
	staleDistinct, err := svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase:   app.MutationBase{RequestID: "c-stale-distinct", Actor: acceptanceActor},
		Titles:         []store.TitleInput{{Text: origTitle, Kind: domain.TitleOriginal, Language: ru, Provenance: "manual"}},
		Choice:         strChoice(app.ChoiceCreateDistinct),
		CandidateToken: stale.CandidateToken,
	})
	if err != nil || !staleDistinct.Created {
		t.Fatalf("stale distinct create: created=%+v err=%v", staleDistinct, err)
	}
	if _, err := svc.CreateLibraryEntry(ctx, app.AdditionRequest{
		MutationBase:   app.MutationBase{RequestID: "c-stale-use", Actor: acceptanceActor},
		Titles:         []store.TitleInput{{Text: origTitle, Kind: domain.TitleOriginal, Language: ru, Provenance: "manual"}},
		WorkID:         &workID,
		Choice:         strChoice(app.ChoiceUseExisting),
		CandidateToken: stale.CandidateToken,
	}); errorCode(t, err) != app.ErrConflict {
		t.Fatalf("stale token: want conflict, got %v", err)
	}

	// Concurrent exact additions settle to one record.
	const workers = 8
	type outcome struct {
		res app.AdditionResult
		err error
	}
	results := make([]outcome, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, err := svc.CreateLibraryEntry(ctx, app.AdditionRequest{
				MutationBase: app.MutationBase{RequestID: "c-concurrent", Actor: acceptanceActor},
				Titles:       []store.TitleInput{{Text: "Параллельный Тест", Kind: domain.TitleOriginal, Language: ru, Preferred: true, Provenance: "manual"}},
				Kind:         &novel,
			})
			results[i] = outcome{res: r, err: err}
		}(i)
	}
	wg.Wait()
	for i, o := range results {
		if o.err != nil || !o.res.Created {
			t.Fatalf("concurrent worker %d: created=%+v err=%v", i, o.res, o.err)
		}
	}
	concurrentPage, err := svc.SearchLibrary(ctx, app.LibraryFilter{Query: strP("Параллельный Тест")})
	if err != nil || concurrentPage.Total != 1 {
		t.Fatalf("concurrent exact adds: total=%d err=%v", concurrentPage.Total, err)
	}
	if got := concurrentPage.Items[0].(app.WorkCard).WorkID; got != results[0].res.WorkID {
		t.Fatalf("concurrent adds split into %q and %q", got, results[0].res.WorkID)
	}
}
