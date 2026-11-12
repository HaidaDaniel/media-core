package store

import (
	"errors"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

func tagActor() TagActor {
	return TagActor{Principal: "local", Origin: "user"}
}

func mustAttachWorkTag(t *testing.T, db *Database, now time.Time, w Work, revision int64, name string) Work {
	t.Helper()
	fresh, err := db.SetWorkTag(ctx(t), now, w.ID, revision, name, false, tagActor(), nil)
	if err != nil {
		t.Fatalf("SetWorkTag(attach %s): %v", name, err)
	}
	return fresh
}

func mustDetachWorkTag(t *testing.T, db *Database, now time.Time, w Work, revision int64, name string) Work {
	t.Helper()
	fresh, err := db.SetWorkTag(ctx(t), now, w.ID, revision, name, true, tagActor(), nil)
	if err != nil {
		t.Fatalf("SetWorkTag(detach %s): %v", name, err)
	}
	return fresh
}

func mustAttachEditionTag(t *testing.T, db *Database, now time.Time, e Edition, revision int64, name string) Edition {
	t.Helper()
	fresh, err := db.SetEditionTag(ctx(t), now, e.ID, revision, name, false, tagActor(), nil)
	if err != nil {
		t.Fatalf("SetEditionTag(attach %s): %v", name, err)
	}
	return fresh
}

func mustDetachEditionTag(t *testing.T, db *Database, now time.Time, e Edition, revision int64, name string) Edition {
	t.Helper()
	fresh, err := db.SetEditionTag(ctx(t), now, e.ID, revision, name, true, tagActor(), nil)
	if err != nil {
		t.Fatalf("SetEditionTag(detach %s): %v", name, err)
	}
	return fresh
}

// TestTagNormalization proves the unique name identity: NFKC, case fold and
// whitespace collapse, with the 1-64 character bound on the normalized name.
func TestTagNormalization(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"  Favorite  ", "favorite"},
		{"MANGA-Translation-Candidate", "manga-translation-candidate"},
		{"Fav\tofite", "fav ofite"},
		{"Ｆａｖｏｒｉｔｅ", "favorite"},
		// Full case fold maps the eszett to ss, like the title search key.
		{"ßstraße", "ssstrasse"},
	}
	for _, c := range cases {
		got, err := NormalizeTagName(c.raw)
		if err != nil {
			t.Fatalf("NormalizeTagName(%q): %v", c.raw, err)
		}
		if got != c.want {
			t.Errorf("NormalizeTagName(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
	if _, err := NormalizeTagName("   "); err == nil {
		t.Error("whitespace-only name: want error, got nil")
	}
	if _, err := NormalizeTagName(""); err == nil {
		t.Error("empty name: want error, got nil")
	}
	long := ""
	for i := 0; i < MaxTagNameRunes+1; i++ {
		long += "a"
	}
	if _, err := NormalizeTagName(long); err == nil {
		t.Error("overlong name: want error, got nil")
	}
	if _, err := NormalizeTagName(string([]rune(make([]rune, 0)))); err == nil {
		t.Error("empty rune name: want error, got nil")
	}
	boundary := ""
	for i := 0; i < MaxTagNameRunes; i++ {
		boundary += "é"
	}
	if got, err := NormalizeTagName(boundary); err != nil || utf8.RuneCountInString(got) != MaxTagNameRunes {
		t.Errorf("64-rune name = (%q, %v), want accepted at the bound", got, err)
	}
}

// TestTagAttachDetachWork proves work tag attach/detach: the revision
// bumps, a repeated attach or detach is a no-op, a detach keeps the
// assignment row as audit with its detach actor, and a re-attach starts a
// fresh assignment.
func TestTagAttachDetachWork(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Tag Story")
	base := w.Revision

	w = mustAttachWorkTag(t, db, now.Add(time.Second), w, base, "Favorite")
	if w.Revision != base+1 {
		t.Fatalf("work revision = %d, want %d after attach", w.Revision, base+1)
	}
	if len(w.Tags) != 1 || w.Tags[0] != "favorite" {
		t.Fatalf("work tags = %+v, want [favorite]", w.Tags)
	}

	w = mustAttachWorkTag(t, db, now.Add(2*time.Second), w, w.Revision, "  FAVORITE ")
	if w.Revision != base+1 {
		t.Fatalf("work revision = %d, want %d after the no-op re-attach", w.Revision, base+1)
	}

	w = mustDetachWorkTag(t, db, now.Add(3*time.Second), w, w.Revision, "favorite")
	if w.Revision != base+2 {
		t.Fatalf("work revision = %d, want %d after detach", w.Revision, base+2)
	}
	if len(w.Tags) != 0 {
		t.Fatalf("work tags = %+v, want empty after detach", w.Tags)
	}

	var (
		attachedAt, detachedAt string
		actor, detachActor     string
		rows                   int
	)
	if err := db.DB().QueryRow(
		`SELECT COUNT(*) FROM tag_assignments WHERE work_id = ?`, w.ID).Scan(&rows); err != nil {
		t.Fatalf("count assignments: %v", err)
	}
	if rows != 1 {
		t.Fatalf("assignment rows = %d, want 1 (a detach retains the audit row)", rows)
	}
	if err := db.DB().QueryRow(
		`SELECT attached_at, detached_at, actor, detach_actor FROM tag_assignments WHERE work_id = ?`, w.ID).
		Scan(&attachedAt, &detachedAt, &actor, &detachActor); err != nil {
		t.Fatalf("read assignment: %v", err)
	}
	if detachedAt == "" {
		t.Fatal("detached_at is empty after detach")
	}
	if detachActor == "" {
		t.Fatal("detach_actor is empty after detach")
	}

	// A re-attach after a detach starts a fresh assignment.
	w = mustAttachWorkTag(t, db, now.Add(4*time.Second), w, w.Revision, "favorite")
	if w.Revision != base+3 || len(w.Tags) != 1 || w.Tags[0] != "favorite" {
		t.Fatalf("work = %+v, want favorite re-attached at revision %d", w, base+3)
	}
	if err := db.DB().QueryRow(
		`SELECT COUNT(*) FROM tag_assignments WHERE work_id = ?`, w.ID).Scan(&rows); err != nil {
		t.Fatalf("count assignments: %v", err)
	}
	if rows != 2 {
		t.Fatalf("assignment rows = %d, want 2 after the re-attach", rows)
	}

	// Detaching a tag that is not attached is a no-op.
	w = mustDetachWorkTag(t, db, now.Add(5*time.Second), w, w.Revision, "read-later")
	if w.Revision != base+3 {
		t.Fatalf("work revision = %d, want %d after the no-op detach", w.Revision, base+3)
	}
}

// TestTagAttachDetachEdition proves edition tag attach/detach with the same
// rules, and that work tags and edition tags are independent dimensions of
// the same normalized name.
func TestTagAttachDetachEdition(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Edition Tags")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga T")

	e = mustAttachEditionTag(t, db, now.Add(2*time.Second), e, e.Revision, "favorite")
	if len(e.Tags) != 1 || e.Tags[0] != "favorite" {
		t.Fatalf("edition tags = %+v, want [favorite]", e.Tags)
	}

	// The same normalized name on the work is an independent dimension.
	w = mustAttachWorkTag(t, db, now.Add(3*time.Second), w, w.Revision, "favorite")
	if len(w.Tags) != 1 || w.Tags[0] != "favorite" {
		t.Fatalf("work tags = %+v, want [favorite]", w.Tags)
	}

	// Detaching the edition tag leaves the work tag and keeps the audit row.
	e = mustDetachEditionTag(t, db, now.Add(4*time.Second), e, e.Revision, "favorite")
	if len(e.Tags) != 0 {
		t.Fatalf("edition tags = %+v, want empty after detach", e.Tags)
	}
	var rows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM tag_assignments WHERE edition_id = ?`, e.ID).Scan(&rows); err != nil {
		t.Fatalf("count assignments: %v", err)
	}
	if rows != 1 {
		t.Fatalf("assignment rows = %d, want 1 (a detach retains the audit row)", rows)
	}
	fresh, err := db.GetWork(ctx(t), w.ID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if len(fresh.Tags) != 1 || fresh.Tags[0] != "favorite" {
		t.Fatalf("work tags = %+v, want the work tag to survive the edition detach", fresh.Tags)
	}
}

// TestTagAndIntentionFilters prove the shared library predicates: the
// saved/planned intention filter uses intention only, the tag predicate is
// satisfied by a work tag or the qualifying edition's tag, and the
// dimensions AND together on the same edition.
func TestTagAndIntentionFilters(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()

	// Manga work: the manga edition is saved, the anime edition completed.
	wm := mustWork(t, db, now, "Cross Media")
	em, wm := mustEdition(t, db, now.Add(time.Second), wm, domain.KindManga, "Manga X")
	ea, wm := mustEdition(t, db, now.Add(2*time.Second), wm, domain.KindAnime, "Anime X")
	em = mustSetIntention(t, db, now.Add(3*time.Second), em, em.Revision, IntentionInput{Value: string(domain.IntentionSaved)})
	ea = mustSetConsumption(t, db, now.Add(4*time.Second), ea, ea.Revision, ConsumptionInput{Value: string(domain.ConsumptionCompleted)})

	// A second work with a planned intention and a work-scoped tag.
	wp := mustWork(t, db, now.Add(5*time.Second), "Planned Story")
	ep, wp := mustEdition(t, db, now.Add(6*time.Second), wp, domain.KindManga, "Manga P")
	ep = mustSetIntention(t, db, now.Add(7*time.Second), ep, ep.Revision, IntentionInput{Value: string(domain.IntentionPlanned)})
	wp = mustAttachWorkTag(t, db, now.Add(8*time.Second), wp, wp.Revision, "read-later")

	// An untagged, intention-less work with one unobserved edition.
	wu := mustWork(t, db, now.Add(9*time.Second), "Plain Story")
	_, wu = mustEdition(t, db, now.Add(10*time.Second), wu, domain.KindManga, "Manga U")

	ids := func(t *testing.T, q LibraryQuery) map[string]bool {
		t.Helper()
		cards, total, err := db.SearchLibrary(ctx(t), q)
		if err != nil {
			t.Fatalf("SearchLibrary: %v", err)
		}
		if total != int64(len(cards)) {
			t.Fatalf("total = %d, want %d (single page)", total, len(cards))
		}
		out := map[string]bool{}
		for _, c := range cards {
			out[c.Work.ID] = true
		}
		return out
	}

	// Saved picks only the manga work; planned picks only the planned work;
	// none picks the works whose editions have no recorded intention — the
	// plain work through its edition and the cross-media work through its
	// intention-less anime edition. The completed anime is never a saved
	// match: the filter uses intention, not consumption or tags.
	saved := ids(t, LibraryQuery{Intentions: []domain.Intention{domain.IntentionSaved}, Sort: SortTitle, Limit: 10})
	if len(saved) != 1 || !saved[wm.ID] {
		t.Fatalf("saved filter = %+v, want only the cross-media work", saved)
	}
	planned := ids(t, LibraryQuery{Intentions: []domain.Intention{domain.IntentionPlanned}, Sort: SortTitle, Limit: 10})
	if len(planned) != 1 || !planned[wp.ID] {
		t.Fatalf("planned filter = %+v, want only the planned work", planned)
	}
	none := ids(t, LibraryQuery{Intentions: []domain.Intention{domain.IntentionNone}, Sort: SortTitle, Limit: 10})
	if len(none) != 2 || !none[wu.ID] || !none[wm.ID] {
		t.Fatalf("none filter = %+v, want the plain work and the cross-media work through its anime edition", none)
	}

	// saved OR planned within the dimension.
	either := ids(t, LibraryQuery{Intentions: []domain.Intention{domain.IntentionSaved, domain.IntentionPlanned}, Sort: SortTitle, Limit: 10})
	if len(either) != 2 || !either[wm.ID] || !either[wp.ID] {
		t.Fatalf("saved+planned filter = %+v, want both works", either)
	}

	// A work tag satisfies the tag predicate for the whole work.
	workTag := ids(t, LibraryQuery{Tags: []string{"read-later"}, Sort: SortTitle, Limit: 10})
	if len(workTag) != 1 || !workTag[wp.ID] {
		t.Fatalf("work tag filter = %+v, want only the planned work", workTag)
	}

	// An edition tag satisfies the predicate through its own edition.
	em = mustAttachEditionTag(t, db, now.Add(10*time.Second), em, em.Revision, "favorite")
	fav := ids(t, LibraryQuery{Tags: []string{"favorite"}, Sort: SortTitle, Limit: 10})
	if len(fav) != 1 || !fav[wm.ID] {
		t.Fatalf("edition tag filter = %+v, want only the cross-media work", fav)
	}

	// The dimensions AND on the same edition: the favorite tag work has a
	// saved intention, the planned work a work tag but no saved edition
	// intention.
	both := ids(t, LibraryQuery{Intentions: []domain.Intention{domain.IntentionSaved}, Tags: []string{"favorite"}, Sort: SortTitle, Limit: 10})
	if len(both) != 1 || !both[wm.ID] {
		t.Fatalf("intention+tag filter = %+v, want only the cross-media work", both)
	}
	andNo := ids(t, LibraryQuery{Intentions: []domain.Intention{domain.IntentionSaved}, Tags: []string{"read-later"}, Sort: SortTitle, Limit: 10})
	if len(andNo) != 0 {
		t.Fatalf("saved+read-later filter = %+v, want empty (no saved work carries the work tag)", andNo)
	}

	// Consumption and intention stay independent: the completed anime
	// edition's work has no saved intention and the saved manga edition's
	// work is not completed.
	completed := ids(t, LibraryQuery{Consumptions: []domain.Consumption{domain.ConsumptionCompleted}, Sort: SortTitle, Limit: 10})
	if len(completed) != 1 || !completed[wm.ID] {
		t.Fatalf("completed filter = %+v, want the cross-media work through its anime edition", completed)
	}
	completedAndSaved := ids(t, LibraryQuery{
		Consumptions: []domain.Consumption{domain.ConsumptionCompleted},
		Intentions:   []domain.Intention{domain.IntentionSaved},
		Sort:         SortTitle,
		Limit:        10,
	})
	if len(completedAndSaved) != 0 {
		t.Fatalf("completed+saved filter = %+v, want empty (no single edition is both)", completedAndSaved)
	}
}

// TestTagInvalidAndStale proves invalid names, a stale revision and a
// missing target fail distinctly without partial writes.
func TestTagInvalidAndStale(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Invalid Tags")
	e, _ := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga I")

	if _, err := db.SetWorkTag(ctx(t), now, w.ID, w.Revision, "   ", false, tagActor(), nil); !errors.Is(err, ErrInvalidTag) {
		t.Fatalf("blank name: err = %v, want ErrInvalidTag", err)
	}
	long := ""
	for i := 0; i < MaxTagNameRunes+1; i++ {
		long += "a"
	}
	if _, err := db.SetWorkTag(ctx(t), now, w.ID, w.Revision, long, false, tagActor(), nil); !errors.Is(err, ErrInvalidTag) {
		t.Fatalf("overlong name: err = %v, want ErrInvalidTag", err)
	}
	if _, err := db.SetWorkTag(ctx(t), now, w.ID, 99, "favorite", false, tagActor(), nil); !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("stale work revision: err = %v, want ErrRevisionMismatch", err)
	}
	if _, err := db.SetEditionTag(ctx(t), now, "00000000-0000-4000-8000-000000000000", 0, "favorite", false, tagActor(), nil); !errors.Is(err, ErrEditionNotFound) {
		t.Fatalf("missing edition: err = %v, want ErrEditionNotFound", err)
	}
	if _, err := db.SetWorkTag(ctx(t), now, "00000000-0000-4000-8000-000000000000", 0, "favorite", false, tagActor(), nil); !errors.Is(err, ErrWorkNotFound) {
		t.Fatalf("missing work: err = %v, want ErrWorkNotFound", err)
	}
	if len(w.Tags) != 0 || len(e.Tags) != 0 {
		t.Fatalf("tags = work %v edition %v, want none after failures", w.Tags, e.Tags)
	}
	var tagRows int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM tags`).Scan(&tagRows); err != nil {
		t.Fatalf("count tags: %v", err)
	}
	if tagRows != 0 {
		t.Fatalf("tag rows = %d, want 0 after invalid names", tagRows)
	}
}
