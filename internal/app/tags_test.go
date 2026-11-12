package app

import (
	"context"
	"strings"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

func mustSetWorkTag(t *testing.T, s *Service, req TagRequest) WorkView {
	t.Helper()
	view, err := s.SetWorkTag(context.Background(), req)
	if err != nil {
		t.Fatalf("SetWorkTag(%s %s): %v", req.TargetID, req.Tag, err)
	}
	return view
}

func mustSetEditionTag(t *testing.T, s *Service, req TagRequest) EditionView {
	t.Helper()
	view, err := s.SetEditionTag(context.Background(), req)
	if err != nil {
		t.Fatalf("SetEditionTag(%s %s): %v", req.TargetID, req.Tag, err)
	}
	return view
}

// TestTagSetWorkAndEdition proves the service tag path: attach and detach
// on both targets, normalization to one unique name, idempotent repeated
// operations, and the edition view carrying the active tag names.
func TestTagSetWorkAndEdition(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Tag Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga T")
	w, err := s.db.GetWork(context.Background(), w.ID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}

	// A work tag bumps the work revision.
	workRev := w.Revision
	wv := mustSetWorkTag(t, s, TagRequest{
		MutationBase: mustBase(t, "req-tag-w1", workRev),
		TargetID:     w.ID,
		Tag:          "  Favorite ",
	})
	if wv.Revision != workRev+1 {
		t.Fatalf("work view revision = %d, want %d", wv.Revision, workRev+1)
	}

	// An edition tag bumps the edition revision and the edition view lists
	// the normalized name.
	edRev := e.Revision
	ev := mustSetEditionTag(t, s, TagRequest{
		MutationBase: mustBase(t, "req-tag-e1", edRev),
		TargetID:     e.ID,
		Tag:          "FAVORITE",
	})
	if ev.Revision != edRev+1 {
		t.Fatalf("edition view revision = %d, want %d", ev.Revision, edRev+1)
	}
	if len(ev.Tags) != 1 || ev.Tags[0] != "favorite" {
		t.Fatalf("edition tags = %+v, want [favorite]", ev.Tags)
	}

	// The same normalized name re-attached with different spelling is a
	// no-op: no second assignment, no revision bump.
	ev = mustSetEditionTag(t, s, TagRequest{
		MutationBase: mustBase(t, "req-tag-e2", ev.Revision),
		TargetID:     e.ID,
		Tag:          "favorite",
	})
	if ev.Revision != edRev+1 {
		t.Fatalf("edition view revision = %d, want %d after the no-op re-attach", ev.Revision, edRev+1)
	}

	// A detach clears the active name but keeps the audit row.
	ev = mustSetEditionTag(t, s, TagRequest{
		MutationBase: mustBase(t, "req-tag-e3", ev.Revision),
		TargetID:     e.ID,
		Tag:          "favorite",
		Detach:       true,
	})
	if ev.Revision != edRev+2 {
		t.Fatalf("edition view revision = %d, want %d after the detach", ev.Revision, edRev+2)
	}
	if len(ev.Tags) != 0 {
		t.Fatalf("edition tags = %+v, want empty after the detach", ev.Tags)
	}
	var rows int
	if err := s.db.DB().QueryRow(`SELECT COUNT(*) FROM tag_assignments WHERE edition_id = ?`, e.ID).Scan(&rows); err != nil {
		t.Fatalf("count assignments: %v", err)
	}
	if rows != 1 {
		t.Fatalf("assignment rows = %d, want 1 (a detach retains the audit row)", rows)
	}

	// A repeated detach is a no-op.
	ev = mustSetEditionTag(t, s, TagRequest{
		MutationBase: mustBase(t, "req-tag-e4", ev.Revision),
		TargetID:     e.ID,
		Tag:          "favorite",
		Detach:       true,
	})
	if ev.Revision != edRev+2 {
		t.Fatalf("edition view revision = %d, want %d after the no-op detach", ev.Revision, edRev+2)
	}
}

// TestTagReplayAndErrors proves the stable error mapping on the tag path:
// an exact retry replays, a different payload on the same key conflicts,
// a stale revision conflicts, a missing target is a not-found, and a blank
// or overlong tag is an invalid argument.
func TestTagReplayAndErrors(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Error Tags")
	e := addEdition(t, s, w.ID, domain.KindManga, "Manga E")

	// An exact retry of the first attach replays without a second row.
	rev := e.Revision
	view := mustSetEditionTag(t, s, TagRequest{
		MutationBase: mustBase(t, "req-tag-r1", rev),
		TargetID:     e.ID,
		Tag:          "favorite",
	})
	replayed := mustSetEditionTag(t, s, TagRequest{
		MutationBase: mustBase(t, "req-tag-r1", rev),
		TargetID:     e.ID,
		Tag:          "favorite",
	})
	if replayed.Revision != view.Revision {
		t.Fatalf("replayed revision = %d, want %d (a retry appends nothing)", replayed.Revision, view.Revision)
	}

	if _, err := s.SetEditionTag(context.Background(), TagRequest{
		MutationBase: mustBase(t, "req-tag-r1", view.Revision),
		TargetID:     e.ID,
		Tag:          "other",
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("same key different payload: want conflict, got %v", err)
	}
	if _, err := s.SetEditionTag(context.Background(), TagRequest{
		MutationBase: mustBase(t, "req-tag-stale", 99),
		TargetID:     e.ID,
		Tag:          "other",
	}); appError(t, err).Code != ErrConflict {
		t.Fatalf("stale revision: want conflict, got %v", err)
	}
	if _, err := s.SetWorkTag(context.Background(), TagRequest{
		MutationBase: mustBase(t, "req-tag-mw", 0),
		TargetID:     "00000000-0000-4000-8000-000000000000",
		Tag:          "favorite",
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("missing work: want not_found, got %v", err)
	}
	if _, err := s.SetEditionTag(context.Background(), TagRequest{
		MutationBase: mustBase(t, "req-tag-me", 0),
		TargetID:     "00000000-0000-4000-8000-000000000000",
		Tag:          "favorite",
	}); appError(t, err).Code != ErrNotFound {
		t.Fatalf("missing edition: want not_found, got %v", err)
	}
	if _, err := s.SetEditionTag(context.Background(), TagRequest{
		MutationBase: mustBase(t, "req-tag-blank", e.Revision),
		TargetID:     e.ID,
		Tag:          "   ",
	}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("blank tag: want invalid_argument, got %v", err)
	}
	if _, err := s.SetEditionTag(context.Background(), TagRequest{
		MutationBase: mustBase(t, "req-tag-long", e.Revision),
		TargetID:     e.ID,
		Tag:          strings.Repeat("a", 65),
	}); appError(t, err).Code != ErrInvalidArgument {
		t.Fatalf("overlong tag: want invalid_argument, got %v", err)
	}
}

// TestIntentionTagFilterQueries proves the shared library filters through
// the service: saved and planned select by intention only (a read-later
// user tag never infers saved), none selects editions without a recorded
// intention, and the tag predicate is satisfied by a work tag or the
// qualifying edition's own tag.
func TestIntentionTagFilterQueries(t *testing.T) {
	s := newTestService(t)

	// The acceptance scenario: a manga is saved while the anime is
	// completed, and the manga carries favorite and a translation
	// candidate tag.
	wm := mustCreateWork(t, s, "Cross Media")
	em := addEdition(t, s, wm.ID, domain.KindManga, "Manga A")
	ea := addEdition(t, s, wm.ID, domain.KindAnime, "Anime A")
	mustSetIntentionView(t, s, SetIntentionRequest{
		MutationBase: mustBase(t, "req-f-int", em.Revision),
		EditionID:    em.ID,
		Value:        string(domain.IntentionSaved),
	})
	mustSetConsumptionView(t, s, SetConsumptionRequest{
		MutationBase: mustBase(t, "req-f-cons", ea.Revision),
		EditionID:    ea.ID,
		Value:        string(domain.ConsumptionCompleted),
	})
	mustSetEditionTag(t, s, TagRequest{
		MutationBase: mustBase(t, "req-f-fav", em.Revision+1),
		TargetID:     em.ID,
		Tag:          "favorite",
	})
	mustSetEditionTag(t, s, TagRequest{
		MutationBase: mustBase(t, "req-f-tran", em.Revision+2),
		TargetID:     em.ID,
		Tag:          "MANGA-Translation-Candidate",
	})

	// A second work keeps the read-later user tag on its edition: the
	// saved filter must not pick it up through tag inference.
	wr := mustCreateWork(t, s, "Read Later Story")
	er := addEdition(t, s, wr.ID, domain.KindManga, "Manga R")
	mustSetEditionTag(t, s, TagRequest{
		MutationBase: mustBase(t, "req-f-rl", er.Revision),
		TargetID:     er.ID,
		Tag:          "read-later",
	})

	workIDs := func(t *testing.T, f LibraryFilter) map[string]bool {
		t.Helper()
		cards := listCards(t, s, f)
		out := map[string]bool{}
		for _, c := range cards {
			out[c.WorkID] = true
		}
		return out
	}

	// Saved picks the manga through its intention; the completed anime is
	// not a saved match and the read-later tag alone is not saved.
	saved := workIDs(t, LibraryFilter{Intentions: []domain.Intention{domain.IntentionSaved}, Limit: 10})
	if len(saved) != 1 || !saved[wm.ID] {
		t.Fatalf("saved filter = %+v, want only the cross-media work", saved)
	}

	// Planned picks nothing: no edition is planned.
	planned := workIDs(t, LibraryFilter{Intentions: []domain.Intention{domain.IntentionPlanned}, Limit: 10})
	if len(planned) != 0 {
		t.Fatalf("planned filter = %+v, want empty", planned)
	}

	// None picks the cross-media work through its intention-less anime
	// edition and the read-later work through its edition.
	none := workIDs(t, LibraryFilter{Intentions: []domain.Intention{domain.IntentionNone}, Limit: 10})
	if len(none) != 2 || !none[wm.ID] || !none[wr.ID] {
		t.Fatalf("none filter = %+v, want the cross-media and read-later works", none)
	}

	// The edition tag predicate: favorite and the normalized translation
	// candidate both pick the manga work.
	fav := workIDs(t, LibraryFilter{Tags: []string{"favorite"}, Limit: 10})
	if len(fav) != 1 || !fav[wm.ID] {
		t.Fatalf("favorite filter = %+v, want only the cross-media work", fav)
	}
	tran := workIDs(t, LibraryFilter{Tags: []string{"manga-translation-candidate"}, Limit: 10})
	if len(tran) != 1 || !tran[wm.ID] {
		t.Fatalf("translation candidate filter = %+v, want only the cross-media work", tran)
	}

	// The work tag predicate: a work-scoped tag satisfies the filter for
	// the whole work.
	wrFresh, err := s.db.GetWork(context.Background(), wr.ID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	mustSetWorkTag(t, s, TagRequest{
		MutationBase: mustBase(t, "req-f-wt", wrFresh.Revision),
		TargetID:     wr.ID,
		Tag:          "read-later",
	})
	workTag := workIDs(t, LibraryFilter{Tags: []string{"read-later"}, Limit: 10})
	if len(workTag) != 1 || !workTag[wr.ID] {
		t.Fatalf("read-later filter = %+v, want only the read-later work", workTag)
	}

	// The dimensions AND on the same edition: the saved work carries the
	// favorite tag, the read-later work does not.
	both := workIDs(t, LibraryFilter{
		Intentions: []domain.Intention{domain.IntentionSaved},
		Tags:       []string{"favorite"},
		Limit:      10,
	})
	if len(both) != 1 || !both[wm.ID] {
		t.Fatalf("saved+favorite filter = %+v, want only the cross-media work", both)
	}

	// Consumption and intention stay independent: the completed anime and
	// the saved manga live on the same work, but no single edition is both
	// completed and saved.
	completedAndSaved := workIDs(t, LibraryFilter{
		Consumptions: []domain.Consumption{domain.ConsumptionCompleted},
		Intentions:   []domain.Intention{domain.IntentionSaved},
		Limit:        10,
	})
	if len(completedAndSaved) != 0 {
		t.Fatalf("completed+saved filter = %+v, want empty (no single edition is both)", completedAndSaved)
	}
}
