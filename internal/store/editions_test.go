package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

func addTestWork(t *testing.T, db *Database) Work {
	t.Helper()
	w, err := db.CreateWork(context.Background(), workTestNow(), []TitleInput{
		{Text: "Story", Kind: domain.TitleOriginal, Provenance: "manual"},
	})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	return w
}

// TestEditionAddRetainsMedium proves a translation keeps its medium and one
// work may carry an anime and a manga edition side by side.
func TestEditionAddRetainsMedium(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w := addTestWork(t, db)

	manga, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{
		Kind:     domain.KindManga,
		Language: strPtr("ja"),
		Titles:   []TitleInput{{Text: "進撃の巨人", Kind: domain.TitleOriginal, Provenance: "manual"}},
	})
	if err != nil {
		t.Fatalf("AddEdition manga: %v", err)
	}
	w, err = db.GetWork(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if w.Revision != 1 {
		t.Fatalf("work revision = %d, want 1 after the first edition", w.Revision)
	}
	translation, err := db.AddEdition(ctx, now.Add(time.Second), w.ID, w.Revision, EditionInput{
		Kind:     domain.KindManga,
		Language: strPtr("ru"),
		Titles:   []TitleInput{{Text: "Атака Титанов", Language: strPtr("ru"), Kind: domain.TitleLocalized, Provenance: "manual"}},
	})
	if err != nil {
		t.Fatalf("AddEdition translation: %v, a Russian manga translation must stay manga", err)
	}
	if translation.Kind != domain.KindManga {
		t.Fatalf("translation kind = %q, want manga (medium is retained)", translation.Kind)
	}
	w, err = db.GetWork(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if w.Revision != 2 {
		t.Fatalf("work revision = %d, want 2 after two editions", w.Revision)
	}
	anime, err := db.AddEdition(ctx, now.Add(2*time.Second), w.ID, w.Revision, EditionInput{
		Kind:     domain.KindAnime,
		Language: strPtr("ja"),
		Titles:   []TitleInput{{Text: "進撃の巨人 (anime)", Kind: domain.TitleOriginal, Provenance: "manual"}},
	})
	if err != nil {
		t.Fatalf("AddEdition anime: %v", err)
	}
	if translation.WorkID != manga.WorkID || manga.WorkID != anime.WorkID {
		t.Fatalf("editions span multiple works: %s, %s, %s", manga.WorkID, translation.WorkID, anime.WorkID)
	}
	editions, err := db.ListEditions(ctx, w.ID)
	if err != nil {
		t.Fatalf("ListEditions: %v", err)
	}
	if len(editions) != 3 {
		t.Fatalf("editions = %d, want 3", len(editions))
	}
	if manga.Revision != 0 {
		t.Fatalf("new edition revision = %d, want 0", manga.Revision)
	}
	if !manga.CreatedAt.Equal(now) {
		t.Fatalf("edition created_at = %s, want %s", manga.CreatedAt, now)
	}
}

// TestEditionOtherLabelRules pins the kind-other coupling: the label is
// required for other, forbidden elsewhere, and bounded.
func TestEditionOtherLabelRules(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w := addTestWork(t, db)
	if _, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{Kind: domain.KindOther}); !errors.Is(err, ErrInvalidEdition) {
		t.Fatalf("other without label: err = %v, want ErrInvalidEdition", err)
	}
	if _, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{Kind: domain.KindManga, OtherLabel: strPtr("novel")}); !errors.Is(err, ErrInvalidEdition) {
		t.Fatalf("manga with label: err = %v, want ErrInvalidEdition", err)
	}
	if _, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{Kind: domain.KindOther, OtherLabel: strPtr(strings.Repeat("a", 65))}); !errors.Is(err, ErrInvalidEdition) {
		t.Fatalf("oversized label: err = %v, want ErrInvalidEdition", err)
	}
	e, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{Kind: domain.KindOther, OtherLabel: strPtr("radio play")})
	if err != nil {
		t.Fatalf("other with label: %v", err)
	}
	if e.OtherLabel == nil || *e.OtherLabel != "radio play" {
		t.Fatalf("other label = %v, want radio play", e.OtherLabel)
	}
	if e.Language != nil {
		t.Fatalf("unknown language = %v, want null", *e.Language)
	}
	if editions, _ := db.ListEditions(ctx, w.ID); len(editions) != 1 {
		t.Fatalf("editions after rejected adds = %d, want 1", len(editions))
	}
}

func TestEditionInvalidReferences(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	if _, err := db.AddEdition(ctx, now, "00000000-0000-4000-8000-000000000000", 0, EditionInput{Kind: domain.KindManga}); !errors.Is(err, ErrWorkNotFound) {
		t.Fatalf("AddEdition(unknown work): err = %v, want ErrWorkNotFound", err)
	}
	w := addTestWork(t, db)
	if _, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{Kind: domain.KindManga}); err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	if _, err := db.AddEditionTitle(ctx, now, "00000000-0000-4000-8000-000000000000", 0, TitleInput{Text: "T", Kind: domain.TitleOriginal, Provenance: "manual"}); !errors.Is(err, ErrEditionNotFound) {
		t.Fatalf("AddEditionTitle(unknown edition): err = %v, want ErrEditionNotFound", err)
	}
	if _, err := db.RecordPublication(ctx, now, "00000000-0000-4000-8000-000000000000", 0, PublicationInput{Value: domain.PublicationOngoing, Source: strPtr("manual"), Provenance: "manual"}); !errors.Is(err, ErrEditionNotFound) {
		t.Fatalf("RecordPublication(unknown edition): err = %v, want ErrEditionNotFound", err)
	}
	if _, err := db.GetEdition(ctx, "00000000-0000-4000-8000-000000000000"); !errors.Is(err, ErrEditionNotFound) {
		t.Fatalf("GetEdition(unknown): err = %v, want ErrEditionNotFound", err)
	}
}

// TestEditionTitlesWithWorkFallback proves an edition may start without
// titles, may retire all of them (display falls back to the work title),
// and retired rows stay as history.
func TestEditionTitlesWithWorkFallback(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w := addTestWork(t, db)
	e, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{Kind: domain.KindManga})
	if err != nil {
		t.Fatalf("AddEdition without titles: %v, an edition may fall back to its work title", err)
	}
	if e.Titles == nil || len(e.Titles) != 0 {
		t.Fatalf("titles = %v, want an empty non-nil slice", e.Titles)
	}
	e, err = db.AddEditionTitle(ctx, now.Add(time.Second), e.ID, e.Revision, TitleInput{
		Text: "First Name", Kind: domain.TitleOriginal, Provenance: "manual",
	})
	if err != nil {
		t.Fatalf("AddEditionTitle: %v", err)
	}
	if len(e.Titles) != 1 || !e.Titles[0].Preferred {
		t.Fatalf("titles = %+v, want one preferred title", e.Titles)
	}
	e, err = db.RetireEditionTitle(ctx, now.Add(2*time.Second), e.ID, e.Revision, e.Titles[0].ID)
	if err != nil {
		t.Fatalf("RetireEditionTitle (last title): %v, editions may retire all titles", err)
	}
	if len(e.Titles) != 0 {
		t.Fatalf("active titles after retiring all = %d, want 0 (work title fallback)", len(e.Titles))
	}
	var retiredAt string
	if err := db.DB().QueryRow(`SELECT retired_at FROM edition_titles WHERE edition_id = ?`, e.ID).Scan(&retiredAt); err != nil {
		t.Fatalf("retired history row missing: %v", err)
	}
}

func TestEditionTitleRetirementPromotesOldest(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w := addTestWork(t, db)
	e, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{
		Kind: domain.KindManga,
		Titles: []TitleInput{
			{Text: "Wrong", Kind: domain.TitleOriginal, Provenance: "manual", Preferred: true},
			{Text: "Right", Kind: domain.TitleOriginal, Provenance: "manual"},
		},
	})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	retiredID := e.Titles[0].ID
	e, err = db.RetireEditionTitle(ctx, now.Add(time.Second), e.ID, e.Revision, retiredID)
	if err != nil {
		t.Fatalf("RetireEditionTitle: %v", err)
	}
	if len(e.Titles) != 1 || !e.Titles[0].Preferred || e.Titles[0].Text != "Right" {
		t.Fatalf("titles = %+v, want only Right and preferred", e.Titles)
	}
	if _, err := db.RetireEditionTitle(ctx, now.Add(2*time.Second), e.ID, e.Revision, retiredID); !errors.Is(err, ErrEditionTitleNotFound) {
		t.Fatalf("re-retire: err = %v, want ErrEditionTitleNotFound", err)
	}
}

func TestEditionRevisionConflict(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w := addTestWork(t, db)
	e, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{Kind: domain.KindManga})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	_, err = db.AddEditionTitle(ctx, now, e.ID, 5, TitleInput{Text: "Extra", Kind: domain.TitleAlias, Provenance: "manual"})
	if !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("stale edition revision: err = %v, want ErrRevisionMismatch", err)
	}
	if _, err := db.AddEdition(ctx, now, w.ID, 9, EditionInput{Kind: domain.KindAnime}); !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("stale work revision: err = %v, want ErrRevisionMismatch", err)
	}
	got, err := db.GetEdition(ctx, e.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if got.Revision != 0 || len(got.Titles) != 0 {
		t.Fatalf("edition after conflict = (revision %d, %d titles), want unchanged", got.Revision, len(got.Titles))
	}
	if editions, _ := db.ListEditions(ctx, w.ID); len(editions) != 1 {
		t.Fatalf("editions after rejected add = %d, want 1", len(editions))
	}
}

// TestEditionWorkRevisionBumped proves edition membership is a work child
// mutation: the work revision advances, the edition revision starts at zero.
func TestEditionWorkRevisionBumped(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w := addTestWork(t, db)
	if _, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{Kind: domain.KindManga}); err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	w, err := db.GetWork(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if w.Revision != 1 {
		t.Fatalf("work revision = %d, want 1 after edition add", w.Revision)
	}
	if !w.UpdatedAt.Equal(now) {
		t.Fatalf("work updated_at = %s, want %s", w.UpdatedAt, now)
	}
}

func TestPublicationCurrentSelection(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w := addTestWork(t, db)
	e, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{Kind: domain.KindManga})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	if e.Publication != nil {
		t.Fatalf("publication without observations = %+v, want nil (explicit unknown is an application presentation)", e.Publication)
	}
	e, err = db.RecordPublication(ctx, now.Add(time.Second), e.ID, e.Revision, PublicationInput{
		Value: domain.PublicationOngoing, Source: strPtr("scanlation tracker"), Provenance: "manual",
	})
	if err != nil {
		t.Fatalf("RecordPublication ongoing: %v", err)
	}
	if e.Publication == nil || e.Publication.Value != domain.PublicationOngoing {
		t.Fatalf("current publication = %+v, want ongoing", e.Publication)
	}
	if e.Publication.Source == nil || *e.Publication.Source != "scanlation tracker" {
		t.Fatalf("current source = %v, want the recorded source", e.Publication.Source)
	}
	e, err = db.RecordPublication(ctx, now.Add(2*time.Second), e.ID, e.Revision, PublicationInput{
		Value: domain.PublicationCompleted, Source: strPtr("author note"), Provenance: "manual",
	})
	if err != nil {
		t.Fatalf("RecordPublication completed: %v", err)
	}
	if e.Publication.Value != domain.PublicationCompleted {
		t.Fatalf("current publication = %q, want completed (latest observation)", e.Publication.Value)
	}
	if e.Publication.SupersedesID != nil {
		t.Fatalf("supersedes_id = %v, want null for a plain new observation", *e.Publication.SupersedesID)
	}
}

func TestPublicationUnknownExplicit(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w := addTestWork(t, db)
	e, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{Kind: domain.KindManga})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	e, err = db.RecordPublication(ctx, now.Add(time.Second), e.ID, e.Revision, PublicationInput{
		Value: domain.PublicationUnknown, Provenance: "manual",
	})
	if err != nil {
		t.Fatalf("RecordPublication unknown: %v, unknown is explicit and needs no source", err)
	}
	if e.Publication == nil || e.Publication.Value != domain.PublicationUnknown {
		t.Fatalf("current publication = %+v, want explicit unknown", e.Publication)
	}
	if e.Publication.Source != nil {
		t.Fatalf("unknown source = %v, want null", *e.Publication.Source)
	}
}

func TestPublicationSourceRequiredForKnown(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w := addTestWork(t, db)
	e, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{Kind: domain.KindManga})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	_, err = db.RecordPublication(ctx, now, e.ID, e.Revision, PublicationInput{
		Value: domain.PublicationHiatus, Provenance: "manual",
	})
	if !errors.Is(err, ErrInvalidPublication) {
		t.Fatalf("known claim without source: err = %v, want ErrInvalidPublication", err)
	}
	if _, err := db.RecordPublication(ctx, now, e.ID, e.Revision, PublicationInput{
		Value: domain.PublicationOngoing, Source: strPtr("  "), Provenance: "manual",
	}); !errors.Is(err, ErrInvalidPublication) {
		t.Fatalf("blank source: err = %v, want ErrInvalidPublication", err)
	}
	if _, err := db.RecordPublication(ctx, now, e.ID, e.Revision, PublicationInput{
		Value: domain.PublicationOngoing, Source: strPtr("manual"),
	}); !errors.Is(err, ErrInvalidPublication) {
		t.Fatalf("missing provenance: err = %v, want ErrInvalidPublication", err)
	}
	got, err := db.GetEdition(ctx, e.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if got.Publication != nil || got.Revision != 0 {
		t.Fatalf("edition after rejected observations = (publication %+v, revision %d), want unchanged", got.Publication, got.Revision)
	}
}

func TestPublicationFutureObservedRejected(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w := addTestWork(t, db)
	e, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{Kind: domain.KindManga})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	_, err = db.RecordPublication(ctx, now, e.ID, e.Revision, PublicationInput{
		Value:      domain.PublicationOngoing,
		Source:     strPtr("manual"),
		Provenance: "manual",
		ObservedAt: now.Add(10 * time.Minute),
	})
	if !errors.Is(err, ErrInvalidPublication) {
		t.Fatalf("future observed_at: err = %v, want ErrInvalidPublication", err)
	}
}

// TestPublicationPastDatedDoesNotOverwrite proves a past-dated observation
// is stored as history but never becomes current over a newer one.
func TestPublicationPastDatedDoesNotOverwrite(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w := addTestWork(t, db)
	e, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{Kind: domain.KindManga})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	e, err = db.RecordPublication(ctx, now, e.ID, e.Revision, PublicationInput{
		Value: domain.PublicationOngoing, Source: strPtr("manual"), Provenance: "manual",
		ObservedAt: now,
	})
	if err != nil {
		t.Fatalf("RecordPublication: %v", err)
	}
	e, err = db.RecordPublication(ctx, now.Add(time.Second), e.ID, e.Revision, PublicationInput{
		Value: domain.PublicationCompleted, Source: strPtr("old source"), Provenance: "manual",
		ObservedAt: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("RecordPublication past-dated: %v", err)
	}
	if e.Publication.Value != domain.PublicationOngoing {
		t.Fatalf("current publication = %q, want ongoing (past-dated row must not overtake)", e.Publication.Value)
	}
	e, err = db.RecordPublication(ctx, now.Add(2*time.Second), e.ID, e.Revision, PublicationInput{
		Value: domain.PublicationHiatus, Source: strPtr("manual"), Provenance: "manual",
		ObservedAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("RecordPublication newer: %v", err)
	}
	if e.Publication.Value != domain.PublicationHiatus {
		t.Fatalf("current publication = %q, want hiatus (newer observation)", e.Publication.Value)
	}
}

// TestPublicationSupersedes proves corrections add a superseding row, keep
// the old row as history, and a row is superseded at most once.
func TestPublicationSupersedes(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w := addTestWork(t, db)
	e, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{Kind: domain.KindManga})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	e, err = db.RecordPublication(ctx, now, e.ID, e.Revision, PublicationInput{
		Value: domain.PublicationOngoing, Source: strPtr("manual"), Provenance: "manual",
	})
	if err != nil {
		t.Fatalf("RecordPublication: %v", err)
	}
	firstID := e.Publication.ID
	e, err = db.RecordPublication(ctx, now.Add(time.Second), e.ID, e.Revision, PublicationInput{
		Value: domain.PublicationCancelled, Source: strPtr("author note"), Provenance: "manual",
		SupersedesID: &firstID,
	})
	if err != nil {
		t.Fatalf("RecordPublication correction: %v", err)
	}
	if e.Publication.Value != domain.PublicationCancelled || e.Publication.SupersedesID == nil || *e.Publication.SupersedesID != firstID {
		t.Fatalf("current publication = %+v, want the cancelling correction", e.Publication)
	}
	var history int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM publication_observations WHERE edition_id = ?`, e.ID).Scan(&history); err != nil {
		t.Fatalf("count history: %v", err)
	}
	if history != 2 {
		t.Fatalf("history rows = %d, want 2 (the old row is kept)", history)
	}
	if _, err := db.RecordPublication(ctx, now.Add(2*time.Second), e.ID, e.Revision, PublicationInput{
		Value: domain.PublicationOngoing, Source: strPtr("manual"), Provenance: "manual",
		SupersedesID: &firstID,
	}); !errors.Is(err, ErrSupersedeConflict) {
		t.Fatalf("double supersede: err = %v, want ErrSupersedeConflict", err)
	}
	missing := "00000000-0000-4000-8000-000000000000"
	if _, err := db.RecordPublication(ctx, now.Add(3*time.Second), e.ID, e.Revision, PublicationInput{
		Value: domain.PublicationOngoing, Source: strPtr("manual"), Provenance: "manual",
		SupersedesID: &missing,
	}); !errors.Is(err, ErrSupersedeConflict) {
		t.Fatalf("supersede unknown row: err = %v, want ErrSupersedeConflict", err)
	}
}

// TestPublicationIndependentOfPersonalState proves publication state never
// touches personal dimensions: the work and the edition's other fields stay
// untouched and no consumption coupling exists in the store.
func TestPublicationIndependentOfPersonalState(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w := addTestWork(t, db)
	e, err := db.AddEdition(ctx, now, w.ID, w.Revision, EditionInput{Kind: domain.KindManga, Language: strPtr("ja")})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	e, err = db.RecordPublication(ctx, now.Add(time.Second), e.ID, e.Revision, PublicationInput{
		Value: domain.PublicationCompleted, Source: strPtr("author note"), Provenance: "manual",
	})
	if err != nil {
		t.Fatalf("RecordPublication: %v", err)
	}
	work, err := db.GetWork(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if work.Revision != 1 {
		t.Fatalf("work revision = %d, want 1 (publication is edition-scoped, not a work mutation)", work.Revision)
	}
	if e.Revision != 1 {
		t.Fatalf("edition revision = %d, want 1 after the observation", e.Revision)
	}
	if e.Kind != domain.KindManga || e.Language == nil {
		t.Fatalf("edition identity changed: %+v", e)
	}
	// A publication completed must not be readable as a personal state:
	// the store exposes no consumption data at all on the edition.
	var consumptionTables int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name LIKE '%consumption%'`).Scan(&consumptionTables); err != nil {
		t.Fatalf("inspect schema: %v", err)
	}
	if consumptionTables != 0 {
		t.Fatalf("consumption tables exist in the schema: %d, publication state must not imply personal state", consumptionTables)
	}
}
