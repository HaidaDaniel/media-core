package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

func workTestNow() time.Time { return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC) }

func strPtr(s string) *string { return &s }

func TestWorkCreateRequiresTitle(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	if _, err := db.CreateWork(context.Background(), workTestNow(), nil); !errors.Is(err, ErrNoTitle) {
		t.Fatalf("CreateWork without titles: err = %v, want ErrNoTitle", err)
	}
}

func TestWorkCreatePreferredRules(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	// No title marked preferred: the first becomes preferred.
	w, err := db.CreateWork(ctx, workTestNow(), []TitleInput{
		{Text: "First", Kind: domain.TitleOriginal, Provenance: "manual"},
		{Text: "Second", Kind: domain.TitleAlias, Provenance: "manual"},
	})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	if !w.Titles[0].Preferred || w.Titles[1].Preferred {
		t.Fatalf("preferred = (%v, %v), want first title preferred", w.Titles[0].Preferred, w.Titles[1].Preferred)
	}
	// Two preferred titles are rejected before anything is written.
	if _, err := db.CreateWork(ctx, workTestNow(), []TitleInput{
		{Text: "A", Kind: domain.TitleOriginal, Provenance: "manual", Preferred: true},
		{Text: "B", Kind: domain.TitleAlias, Provenance: "manual", Preferred: true},
	}); !errors.Is(err, ErrInvalidTitle) {
		t.Fatalf("two preferred titles: err = %v, want ErrInvalidTitle", err)
	}
	// Invalid inputs are rejected.
	invalid := []TitleInput{
		{Text: "   ", Kind: domain.TitleOriginal, Provenance: "manual"},
		{Text: "Ok", Kind: domain.TitleKind("bogus"), Provenance: "manual"},
		{Text: "Ok", Language: strPtr(""), Kind: domain.TitleOriginal, Provenance: "manual"},
		{Text: "Ok", Kind: domain.TitleOriginal, Provenance: "  "},
	}
	for _, in := range invalid {
		if _, err := db.CreateWork(ctx, workTestNow(), []TitleInput{in}); !errors.Is(err, ErrInvalidTitle) {
			t.Errorf("CreateWork(%+v): err = %v, want ErrInvalidTitle", in, err)
		}
	}
	if works, _ := db.ListWorks(ctx); len(works) != 1 {
		t.Fatalf("works after rejected creates = %d, want 1", len(works))
	}
}

// TestWorkCreateRoundTripsIDsAndTimestamps proves IDs are stable UUIDs and
// instants survive the RFC3339 round trip exactly.
func TestWorkCreateRoundTripsIDsAndTimestamps(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := workTestNow()
	w, err := db.CreateWork(context.Background(), now, []TitleInput{
		{Text: "進撃の巨人", Language: strPtr("ja"), Kind: domain.TitleOriginal, Provenance: "manual"},
	})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	if len(w.ID) != 36 || w.ID[14] != '4' {
		t.Fatalf("work ID %q, want a UUID v4", w.ID)
	}
	if w.Revision != 0 {
		t.Fatalf("revision = %d, want 0", w.Revision)
	}
	if !w.CreatedAt.Equal(now) || !w.UpdatedAt.Equal(now) {
		t.Fatalf("timestamps = (%s, %s), want (%s, %s)", w.CreatedAt, w.UpdatedAt, now, now)
	}
	if w.ArchivedAt != nil {
		t.Fatalf("archived_at = %v, want null", w.ArchivedAt)
	}
	title := w.Titles[0]
	if len(title.ID) != 36 || title.WorkID != w.ID {
		t.Fatalf("title = (%q, %q), want UUID bound to the work", title.ID, title.WorkID)
	}
	if title.Text != "進撃の巨人" {
		t.Fatalf("title text = %q, want the original display text preserved", title.Text)
	}
	got, err := db.GetWork(context.Background(), w.ID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if got.ID != w.ID || !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now) {
		t.Fatalf("round-tripped work = %+v, want the same identity and instants", got)
	}
	if got.Titles[0].Text != title.Text || !got.Titles[0].CreatedAt.Equal(now) {
		t.Fatalf("round-tripped title = %+v, want the same text and instants", got.Titles[0])
	}
	if _, err := db.GetWork(context.Background(), "00000000-0000-4000-8000-000000000000"); !errors.Is(err, ErrWorkNotFound) {
		t.Fatalf("GetWork(unknown): err = %v, want ErrWorkNotFound", err)
	}
}

// TestWorkTitlesSurviveReopen proves multiple aliases and languages —
// including tags with more than three subtags — persist across close and
// reopen.
func TestWorkTitlesSurviveReopen(t *testing.T) {
	dir := t.TempDir()
	db := openMigrated(t, dir)
	w, err := db.CreateWork(context.Background(), workTestNow(), []TitleInput{
		{Text: "進撃の巨人", Language: strPtr("ja"), Kind: domain.TitleOriginal, Provenance: "manual"},
		{Text: "Атака Титанов", Language: strPtr("ru"), Kind: domain.TitleLocalized, Provenance: "manual"},
		{Text: "Shingeki no Kyojin", Language: strPtr("ja-Latn"), Kind: domain.TitleRomanized, Provenance: "manual"},
		{Text: "Attack on Titan", Language: strPtr("en-Latn-GB-1901"), Kind: domain.TitleLocalized, Provenance: "manual"},
		{Text: "AoT", Kind: domain.TitleAlias, Provenance: "manual"},
	})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	db2 := openMigrated(t, dir)
	defer db2.Close()
	got, err := db2.GetWork(context.Background(), w.ID)
	if err != nil {
		t.Fatalf("GetWork after reopen: %v", err)
	}
	if len(got.Titles) != 5 {
		t.Fatalf("titles after reopen = %d, want 5", len(got.Titles))
	}
	wantTexts := []string{"進撃の巨人", "Атака Титанов", "Shingeki no Kyojin", "Attack on Titan", "AoT"}
	for i, want := range wantTexts {
		if got.Titles[i].Text != want {
			t.Errorf("title %d = %q, want %q", i, got.Titles[i].Text, want)
		}
	}
	if got.Titles[3].Language == nil || *got.Titles[3].Language != "en-Latn-GB-1901" {
		t.Fatalf("four-subtag language did not round-trip: %v", got.Titles[3].Language)
	}
	if got.Titles[4].Language != nil {
		t.Fatalf("unknown language = %v, want null", *got.Titles[4].Language)
	}
}

// TestWorkTitleNotUniqueIdentity proves an exact normalized title is a
// possible match, not an identity: two distinct works may share it.
func TestWorkTitleNotUniqueIdentity(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	a, err := db.CreateWork(ctx, workTestNow(), []TitleInput{
		{Text: "Attack on Titan", Kind: domain.TitleOriginal, Provenance: "manual"},
	})
	if err != nil {
		t.Fatalf("CreateWork a: %v", err)
	}
	b, err := db.CreateWork(ctx, workTestNow(), []TitleInput{
		{Text: "attack on titan", Kind: domain.TitleOriginal, Provenance: "manual"},
	})
	if err != nil {
		t.Fatalf("CreateWork b: %v, want a distinct work with the same normalized title", err)
	}
	if a.ID == b.ID {
		t.Fatal("identical work IDs for distinct works")
	}
	if works, _ := db.ListWorks(ctx); len(works) != 2 {
		t.Fatalf("works = %d, want 2", len(works))
	}
}

// TestWorkTitleRetirementHistory proves corrections keep the retired row
// with its history, active queries exclude it, the last active title cannot
// be retired, and retiring the preferred title promotes the oldest active.
func TestWorkTitleRetirementHistory(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w, err := db.CreateWork(ctx, now, []TitleInput{
		{Text: "Wrong Title", Kind: domain.TitleOriginal, Provenance: "manual", Preferred: true},
		{Text: "Right Title", Kind: domain.TitleOriginal, Provenance: "manual"},
	})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	retiredID := w.Titles[0].ID
	w, err = db.RetireWorkTitle(ctx, now.Add(time.Second), w.ID, w.Revision, retiredID)
	if err != nil {
		t.Fatalf("RetireWorkTitle: %v", err)
	}
	if len(w.Titles) != 1 || w.Titles[0].Text != "Right Title" {
		t.Fatalf("active titles = %+v, want only Right Title", w.Titles)
	}
	if !w.Titles[0].Preferred {
		t.Fatal("oldest remaining title was not promoted to preferred")
	}
	if w.Revision != 1 {
		t.Fatalf("revision = %d, want 1", w.Revision)
	}
	var retiredAt string
	err = db.DB().QueryRow(`SELECT retired_at FROM work_titles WHERE id = ?`, retiredID).Scan(&retiredAt)
	if err != nil {
		t.Fatalf("retired row missing: %v", err)
	}
	if want := now.Add(time.Second).Format("2006-01-02T15:04:05.000Z"); retiredAt != want {
		t.Fatalf("retired_at = %q, want %q", retiredAt, want)
	}
	// Retiring the already-retired title again is not found.
	if _, err := db.RetireWorkTitle(ctx, now.Add(2*time.Second), w.ID, w.Revision, retiredID); !errors.Is(err, ErrWorkTitleNotFound) {
		t.Fatalf("re-retire: err = %v, want ErrWorkTitleNotFound", err)
	}
	// The last active title cannot be retired.
	solo, err := db.CreateWork(ctx, now, []TitleInput{
		{Text: "Only", Kind: domain.TitleOriginal, Provenance: "manual"},
	})
	if err != nil {
		t.Fatalf("CreateWork solo: %v", err)
	}
	if _, err := db.RetireWorkTitle(ctx, now, solo.ID, solo.Revision, solo.Titles[0].ID); !errors.Is(err, ErrInvalidTitle) {
		t.Fatalf("retire last title: err = %v, want ErrInvalidTitle", err)
	}
}

func TestWorkTitleSetPreferredDemotes(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w, err := db.CreateWork(ctx, now, []TitleInput{
		{Text: "Original", Kind: domain.TitleOriginal, Provenance: "manual"},
		{Text: "Alias", Kind: domain.TitleAlias, Provenance: "manual"},
	})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	w, err = db.SetPreferredWorkTitle(ctx, now.Add(time.Second), w.ID, w.Revision, w.Titles[1].ID)
	if err != nil {
		t.Fatalf("SetPreferredWorkTitle: %v", err)
	}
	if !w.Titles[1].Preferred || w.Titles[0].Preferred {
		t.Fatalf("preferred = (%v, %v), want the alias preferred and the original demoted, not retired", w.Titles[0].Preferred, w.Titles[1].Preferred)
	}
	if len(w.Titles) != 2 {
		t.Fatalf("demotion retired a title: %d active", len(w.Titles))
	}
	if _, err := db.SetPreferredWorkTitle(ctx, now, w.ID, w.Revision, "missing"); !errors.Is(err, ErrWorkTitleNotFound) {
		t.Fatalf("SetPreferredWorkTitle(unknown): err = %v, want ErrWorkTitleNotFound", err)
	}
}

func TestWorkTitleAddPreferredDemotes(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	w, err := db.CreateWork(ctx, now, []TitleInput{
		{Text: "Old Name", Kind: domain.TitleOriginal, Provenance: "manual"},
	})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	w, err = db.AddWorkTitle(ctx, now.Add(time.Second), w.ID, w.Revision, TitleInput{
		Text: "New Name", Kind: domain.TitleOriginal, Provenance: "manual", Preferred: true,
	})
	if err != nil {
		t.Fatalf("AddWorkTitle: %v", err)
	}
	if len(w.Titles) != 2 {
		t.Fatalf("active titles = %d, want 2", len(w.Titles))
	}
	preferred := 0
	for _, title := range w.Titles {
		if title.Preferred {
			preferred++
			if title.Text != "New Name" {
				t.Fatalf("preferred title = %q, want the newly added one", title.Text)
			}
		}
	}
	if preferred != 1 {
		t.Fatalf("preferred count = %d, want 1", preferred)
	}
	if w.Revision != 1 {
		t.Fatalf("revision = %d, want 1", w.Revision)
	}
}

// TestWorkRevisionConflict proves a stale expected revision is rejected
// with a distinguishable error and changes nothing.
func TestWorkRevisionConflict(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	w, err := db.CreateWork(ctx, workTestNow(), []TitleInput{
		{Text: "Title", Kind: domain.TitleOriginal, Provenance: "manual"},
	})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	_, err = db.AddWorkTitle(ctx, workTestNow(), w.ID, 5, TitleInput{
		Text: "Extra", Kind: domain.TitleAlias, Provenance: "manual",
	})
	if !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("stale revision: err = %v, want ErrRevisionMismatch", err)
	}
	got, err := db.GetWork(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if got.Revision != 0 || len(got.Titles) != 1 {
		t.Fatalf("state after conflict = (revision %d, %d titles), want unchanged", got.Revision, len(got.Titles))
	}
	if _, err := db.RetireWorkTitle(ctx, workTestNow(), "missing", 0, "t"); !errors.Is(err, ErrWorkNotFound) {
		t.Fatalf("RetireWorkTitle(unknown work): err = %v, want ErrWorkNotFound", err)
	}
}

// TestWorkSearchLiteralWildcards proves LIKE metacharacters in titles and
// queries match literally, and matching runs on the normalized key.
func TestWorkSearchLiteralWildcards(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	wild, err := db.CreateWork(ctx, now, []TitleInput{
		{Text: "100%_off sale", Kind: domain.TitleAlias, Provenance: "manual"},
	})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	plain, err := db.CreateWork(ctx, now, []TitleInput{
		{Text: "plain title", Kind: domain.TitleOriginal, Provenance: "manual"},
	})
	if err != nil {
		t.Fatalf("CreateWork plain: %v", err)
	}
	check := func(query string, wantIDs []string) {
		t.Helper()
		works, err := db.SearchWorks(ctx, query)
		if err != nil {
			t.Fatalf("SearchWorks(%q): %v", query, err)
		}
		if len(works) != len(wantIDs) {
			t.Fatalf("SearchWorks(%q) = %d works, want %d", query, len(works), len(wantIDs))
		}
		for i, want := range wantIDs {
			if works[i].ID != want {
				t.Fatalf("SearchWorks(%q)[%d] = %s, want %s", query, i, works[i].ID, want)
			}
		}
	}
	check("100%_off", []string{wild.ID})
	check("100%_OFF SALE", []string{wild.ID})
	check("100% off", nil)        // literal underscore required: no match
	check("%", []string{wild.ID}) // a bare % is literal, not a wildcard
	check("100%_off%", nil)       // trailing literal % is not a wildcard
	check("plain", []string{plain.ID})
}

// workTxRole switches the test binary into the write-transaction holder.
const workTxRole = "MEDIA_CORE_TEST_WORK_TX_ROLE"

// TestWorkWriteTransactionIsImmediate proves the store DSN makes write
// transactions begin IMMEDIATE: a second process that only opened a
// transaction and read (no write statement) still holds the write lock, so
// a concurrent write times out busy instead of succeeding.
func TestWorkWriteTransactionIsImmediate(t *testing.T) {
	if os.Getenv(workTxRole) == "hold" {
		workTxChild(os.Getenv(subprocessDir))
		return
	}
	dir := filepath.Join(t.TempDir(), "state")
	if err := migrateState(dir); err != nil {
		t.Fatalf("migrateState: %v", err)
	}
	bin, err := os.Executable()
	if err != nil {
		t.Fatalf("test binary path: %v", err)
	}
	cmd := exec.Command(bin, "-test.run=TestWorkWriteTransactionIsImmediate")
	cmd.Env = append(os.Environ(), workTxRole+"=hold", subprocessDir+"="+dir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start holder: %v", err)
	}
	defer killAndWait(t, cmd)
	waitForHoldMarker(t, dir)
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open while holder keeps a write transaction: %v", err)
	}
	defer db.Close()
	start := time.Now()
	_, err = db.DB().Exec("CREATE TABLE immediate_probe (x INTEGER)")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("write succeeded while the holder's transaction is open: write transactions are not BEGIN IMMEDIATE")
	}
	if !containsLocked(err.Error()) {
		t.Fatalf("write error = %v, want a busy/locked error", err)
	}
	if elapsed < 4*time.Second || elapsed > 7*time.Second {
		t.Errorf("write gave up after %v, want the ~5s busy timeout", elapsed)
	}
}

// workTxChild holds an open read-only write transaction until killed; the
// marker signals the transaction is open.
func workTxChild(stateDir string) {
	db, err := Open(stateDir)
	if err == nil {
		tx, err := db.DB().BeginTx(context.Background(), &sql.TxOptions{})
		if err == nil {
			var one int
			if err := tx.QueryRow("SELECT 1").Scan(&one); err == nil {
				writeMarker(stateDir)
				<-make(chan struct{})
			}
			tx.Rollback()
		}
		db.Close()
	}
	os.Exit(1)
}

func TestWorkListOrderedByPreferredKey(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	ctx := context.Background()
	now := workTestNow()
	_, err := db.CreateWork(ctx, now, []TitleInput{{Text: "Zeta", Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork Zeta: %v", err)
	}
	alpha, err := db.CreateWork(ctx, now, []TitleInput{{Text: "alpha", Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork alpha: %v", err)
	}
	_, err = db.CreateWork(ctx, now, []TitleInput{{Text: "Beta", Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork Beta: %v", err)
	}
	works, err := db.ListWorks(ctx)
	if err != nil {
		t.Fatalf("ListWorks: %v", err)
	}
	if len(works) != 3 {
		t.Fatalf("works = %d, want 3", len(works))
	}
	if works[0].ID != alpha.ID {
		t.Fatalf("first work = %q (title %q), want alpha", works[0].ID, works[0].Titles[0].Text)
	}
	if got := works[0].Titles[0].SearchKey; got != "alpha" {
		t.Fatalf("stored search key = %q, want alpha", got)
	}
}
