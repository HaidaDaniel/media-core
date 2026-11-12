package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

func progressActor() ProgressActor {
	return ProgressActor{Principal: "local", Origin: "user"}
}

func mustRecordProgress(t *testing.T, db *Database, now time.Time, e Edition, expectedRevision int64, in ProgressInput) Edition {
	t.Helper()
	fresh, err := db.RecordProgress(ctx(t), now, e.ID, expectedRevision, in, progressActor(), nil)
	if err != nil {
		t.Fatalf("RecordProgress(%s %s): %v", in.Unit, in.Position, err)
	}
	return fresh
}

func countProgressRows(t *testing.T, db *Database, editionID string) int {
	t.Helper()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM progress_observations WHERE edition_id = ?`, editionID).Scan(&n); err != nil {
		t.Fatalf("count progress rows: %v", err)
	}
	return n
}

func progressRows(t *testing.T, db *Database, editionID string) []ProgressObservation {
	t.Helper()
	rows, err := db.DB().Query(`SELECT id, unit, position, total, label, reason, observed_at, recorded_at, actor, supersedes_id
		FROM progress_observations WHERE edition_id = ? ORDER BY observed_at, recorded_at, id`, editionID)
	if err != nil {
		t.Fatalf("query progress rows: %v", err)
	}
	defer rows.Close()
	out := []ProgressObservation{}
	for rows.Next() {
		var (
			o          ProgressObservation
			position   *string
			total      *string
			label      *string
			reason     *string
			supersedes *string
			observedAt string
			recordedAt string
			actor      string
		)
		if err := rows.Scan(&o.ID, &o.Unit, &position, &total, &label, &reason, &observedAt, &recordedAt, &actor, &supersedes); err != nil {
			t.Fatalf("scan progress row: %v", err)
		}
		o.Position = position
		o.Total = total
		o.Label = label
		o.Reason = reason
		o.SupersedesID = supersedes
		if err := json.Unmarshal([]byte(actor), &o.Actor); err != nil {
			t.Fatalf("parse actor: %v", err)
		}
		if o.ObservedAt, err = parseInstant(observedAt); err != nil {
			t.Fatalf("parse observed_at: %v", err)
		}
		if o.RecordedAt, err = parseInstant(recordedAt); err != nil {
			t.Fatalf("parse recorded_at: %v", err)
		}
		out = append(out, o)
	}
	return out
}

// TestProgressRecordsHistoryAndCurrent proves transitions append history
// rows, bump the edition revision and update the current position,
// preserving every prior observation.
func TestProgressRecordsHistoryAndCurrent(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story A")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga Title")
	if e.Revision != 0 {
		t.Fatalf("fresh edition revision = %d, want 0", e.Revision)
	}

	e = mustRecordProgress(t, db, now.Add(2*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "42"})
	if e.Revision != 1 {
		t.Fatalf("edition revision = %d, want 1 after the first observation", e.Revision)
	}
	if e.Progress == nil || len(e.Progress) != 1 {
		t.Fatalf("edition progress = %+v, want one live unit", e.Progress)
	}
	if e.Progress[0].Unit != domain.UnitChapter || *e.Progress[0].Position != "42" {
		t.Fatalf("current progress = %+v, want chapter 42", e.Progress[0])
	}
	if e.Progress[0].Actor.Principal != "local" || e.Progress[0].Actor.Origin != "user" {
		t.Fatalf("progress actor = %+v, want principal local origin user", e.Progress[0].Actor)
	}

	e = mustRecordProgress(t, db, now.Add(3*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "48"})
	if e.Revision != 2 {
		t.Fatalf("edition revision = %d, want 2 after the second observation", e.Revision)
	}
	if e.Progress == nil || len(e.Progress) != 1 || *e.Progress[0].Position != "48" {
		t.Fatalf("current progress = %+v, want chapter 48", e.Progress)
	}
	if got := countProgressRows(t, db, e.ID); got != 2 {
		t.Fatalf("progress rows = %d, want 2 (history is preserved)", got)
	}
}

// TestProgressReopenPersistsCurrent proves the latest position and both
// historical rows with their timestamps survive a process reopen.
func TestProgressReopenPersistsCurrent(t *testing.T) {
	dir := t.TempDir()
	db := openMigrated(t, dir)
	now := libraryNow()
	w := mustWork(t, db, now, "Reopen Story")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Reopen Manga")
	e = mustRecordProgress(t, db, now.Add(2*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "42"})
	e = mustRecordProgress(t, db, now.Add(3*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "48"})
	db.Close()

	db = openMigrated(t, dir)
	defer db.Close()
	fresh, err := db.GetEdition(ctx(t), e.ID)
	if err != nil {
		t.Fatalf("GetEdition after reopen: %v", err)
	}
	if fresh.Progress == nil || len(fresh.Progress) != 1 || *fresh.Progress[0].Position != "48" {
		t.Fatalf("current progress after reopen = %+v, want chapter 48", fresh.Progress)
	}
	rows := progressRows(t, db, e.ID)
	if len(rows) != 2 {
		t.Fatalf("progress rows after reopen = %d, want 2", len(rows))
	}
	if *rows[0].Position != "42" || *rows[1].Position != "48" {
		t.Fatalf("historical positions = [%s, %s], want [42, 48]", *rows[0].Position, *rows[1].Position)
	}
	if !rows[0].ObservedAt.Equal(now.Add(2*time.Second)) || !rows[1].ObservedAt.Equal(now.Add(3*time.Second)) {
		t.Fatalf("timestamps after reopen = [%s, %s], want the recorded instants", rows[0].ObservedAt, rows[1].ObservedAt)
	}
}

// TestProgressSameValueNoOp proves a same position/total/label request is a
// no-op: no new row, no revision bump, but the journal still settles so an
// exact retry replays.
func TestProgressSameValueNoOp(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story B")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga B")

	e = mustRecordProgress(t, db, now.Add(2*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "42", Total: strPtr("120")})
	total := "120"
	req := &JournalInput{RequestID: "req-progress-same", Operation: "record_progress", RequestHash: "h", Actor: "local"}
	e, err := db.RecordProgress(ctx(t), now.Add(3*time.Second), e.ID, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "42", Total: &total}, progressActor(), req)
	if err != nil {
		t.Fatalf("RecordProgress(same value): %v", err)
	}
	if e.Revision != 1 {
		t.Fatalf("edition revision = %d, want 1 (same-value no-op must not bump)", e.Revision)
	}
	if got := countProgressRows(t, db, e.ID); got != 1 {
		t.Fatalf("progress rows = %d, want 1 (no duplicate observation)", got)
	}
	var journal int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM mutation_journal WHERE request_id = ?`, "req-progress-same").Scan(&journal); err != nil {
		t.Fatalf("count journal: %v", err)
	}
	if journal != 1 {
		t.Fatalf("journal rows = %d, want 1 (the no-op settles the request key)", journal)
	}
}

// TestProgressInvalidValues proves negative, fractional-page, oversized,
// above-total and malformed values fail atomically without touching the
// state.
func TestProgressInvalidValues(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story C")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga C")

	cases := []ProgressInput{
		{Unit: domain.UnitChapter, Position: "-1"},
		{Unit: domain.UnitChapter, Position: "1.2345"},
		{Unit: domain.UnitChapter, Position: "042"},
		{Unit: domain.UnitChapter, Position: "1000000000.001"},
		{Unit: domain.UnitPage, Position: "3.5"},
		{Unit: domain.UnitPage, Position: "-2"},
		{Unit: domain.UnitPercent, Position: "100.001"},
		{Unit: domain.UnitPercent, Position: "50", Total: strPtr("100")},
		{Unit: domain.UnitChapter, Position: "42", Total: strPtr("0")},
		{Unit: domain.UnitChapter, Position: "42", Total: strPtr("41")},
		{Unit: domain.UnitChapter, Position: "42", Total: strPtr("1.2345")},
		{Unit: "scene", Position: "1"},
		{Unit: domain.UnitChapter, Position: ""},
		{Unit: domain.UnitChapter, Position: "1.5.5"},
	}
	for _, in := range cases {
		_, err := db.RecordProgress(ctx(t), now.Add(2*time.Second), e.ID, e.Revision, in, progressActor(), nil)
		if !errors.Is(err, ErrInvalidProgress) {
			t.Fatalf("RecordProgress(%s %q): err = %v, want ErrInvalidProgress", in.Unit, in.Position, err)
		}
	}
	if e.Revision != 0 || countProgressRows(t, db, e.ID) != 0 {
		t.Fatalf("state after invalid values: revision %d rows %d, want untouched", e.Revision, countProgressRows(t, db, e.ID))
	}

	// The boundary values themselves are valid.
	e = mustRecordProgress(t, db, now.Add(3*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitPercent, Position: "100"})
	if e.Progress == nil || len(e.Progress) != 1 || *e.Progress[0].Position != "100" {
		t.Fatalf("percent 100 = %+v, want accepted", e.Progress)
	}
	e = mustRecordProgress(t, db, now.Add(4*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitPage, Position: "0"})
	e = mustRecordProgress(t, db, now.Add(5*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "1000000000"})
	got := map[domain.ProgressUnit]string{}
	for _, p := range e.Progress {
		got[p.Unit] = *p.Position
	}
	if len(got) != 3 || got[domain.UnitPercent] != "100" || got[domain.UnitPage] != "0" || got[domain.UnitChapter] != "1000000000" {
		t.Fatalf("boundary units = %v, want percent 100, page 0 and chapter 1000000000 live", got)
	}
}

// TestProgressStaleRevision proves a lost-update guard: a wrong expected
// revision is rejected and the prior state stays intact.
func TestProgressStaleRevision(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story D")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga D")

	e = mustRecordProgress(t, db, now.Add(2*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "42"})
	_, err := db.RecordProgress(ctx(t), now.Add(3*time.Second), e.ID, 0, ProgressInput{Unit: domain.UnitChapter, Position: "48"}, progressActor(), nil)
	if !errors.Is(err, ErrRevisionMismatch) {
		t.Fatalf("stale revision: err = %v, want ErrRevisionMismatch", err)
	}
	fresh, err := db.GetEdition(ctx(t), e.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	if fresh.Progress == nil || len(fresh.Progress) != 1 || *fresh.Progress[0].Position != "42" || fresh.Revision != 1 {
		t.Fatalf("state after stale attempt = %+v rev %d, want chapter 42 at rev 1 intact", fresh.Progress, fresh.Revision)
	}
	if got := countProgressRows(t, db, e.ID); got != 1 {
		t.Fatalf("progress rows after stale attempt = %d, want 1", got)
	}
}

// TestProgressSupersedeCorrection proves a correction supersedes an
// existing row of the same unit (kept as history), a row is superseded at
// most once, and a missing, foreign or cross-unit target is distinct.
func TestProgressSupersedeCorrection(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story E")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga E")
	other, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindAnime, "Anime E")

	e = mustRecordProgress(t, db, now.Add(2*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "42"})
	firstID := e.Progress[0].ID
	e = mustRecordProgress(t, db, now.Add(3*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "48"})

	e = mustRecordProgress(t, db, now.Add(4*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "42.5", SupersedesID: &firstID})
	if e.Progress == nil || len(e.Progress) != 1 || *e.Progress[0].Position != "42.5" {
		t.Fatalf("corrected current = %+v, want chapter 42.5", e.Progress)
	}
	if e.Progress[0].SupersedesID == nil || *e.Progress[0].SupersedesID != firstID {
		t.Fatalf("corrected supersedes = %+v, want the first row", e.Progress[0].SupersedesID)
	}
	if got := countProgressRows(t, db, e.ID); got != 3 {
		t.Fatalf("progress rows = %d, want 3 (the corrected row stays in history)", got)
	}

	_, err := db.RecordProgress(ctx(t), now.Add(5*time.Second), e.ID, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "43", SupersedesID: &firstID}, progressActor(), nil)
	if !errors.Is(err, ErrSupersedeConflict) {
		t.Fatalf("second supersede of the same row: err = %v, want ErrSupersedeConflict", err)
	}
	missing := "00000000-0000-4000-8000-000000000000"
	_, err = db.RecordProgress(ctx(t), now.Add(6*time.Second), e.ID, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "43", SupersedesID: &missing}, progressActor(), nil)
	if !errors.Is(err, ErrProgressNotFound) {
		t.Fatalf("unknown supersede target: err = %v, want ErrProgressNotFound", err)
	}
	_, err = db.RecordProgress(ctx(t), now.Add(7*time.Second), other.ID, other.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "1", SupersedesID: &firstID}, progressActor(), nil)
	if !errors.Is(err, ErrProgressNotFound) {
		t.Fatalf("foreign supersede target: err = %v, want ErrProgressNotFound", err)
	}
	// A cross-unit target is invalid even though the id exists on the
	// same edition.
	other = mustRecordProgress(t, db, now.Add(8*time.Second), other, other.Revision, ProgressInput{Unit: domain.UnitVolume, Position: "3"})
	volID := other.Progress[0].ID
	_, err = db.RecordProgress(ctx(t), now.Add(9*time.Second), other.ID, other.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "1", SupersedesID: &volID}, progressActor(), nil)
	if !errors.Is(err, ErrInvalidProgress) {
		t.Fatalf("cross-unit supersede target: err = %v, want ErrInvalidProgress", err)
	}
}

// TestProgressConcurrentSameRowCorrection proves two concurrent corrections
// of the same row: one commits, the other fails with a distinguishable
// error, and the history keeps exactly one correction of the row.
func TestProgressConcurrentSameRowCorrection(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story F")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga F")

	e = mustRecordProgress(t, db, now.Add(2*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "42"})
	firstID := e.Progress[0].ID
	baseRev := e.Revision

	errCh := make(chan error, 2)
	for _, pos := range []string{"41", "43"} {
		go func(pos string) {
			_, err := db.RecordProgress(context.Background(), now.Add(3*time.Second), e.ID, baseRev, ProgressInput{Unit: domain.UnitChapter, Position: pos, SupersedesID: &firstID}, progressActor(), nil)
			errCh <- err
		}(pos)
	}
	var ok, failed int
	var lastErr error
	for range 2 {
		lastErr = <-errCh
		if lastErr == nil {
			ok++
			continue
		}
		if errors.Is(lastErr, ErrRevisionMismatch) || errors.Is(lastErr, ErrSupersedeConflict) {
			failed++
			continue
		}
		t.Fatalf("unexpected error: %v", lastErr)
	}
	if ok != 1 || failed != 1 {
		t.Fatalf("concurrent corrections: %d ok, %d failed (last %v), want one of each", ok, failed, lastErr)
	}
	fresh, err := db.GetEdition(ctx(t), e.ID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	corrections := 0
	for _, r := range progressRows(t, db, e.ID) {
		if r.SupersedesID != nil && *r.SupersedesID == firstID {
			corrections++
		}
	}
	if corrections != 1 {
		t.Fatalf("corrections of the first row = %d, want 1", corrections)
	}
	if fresh.Revision < baseRev+1 {
		t.Fatalf("revision = %d, want at least %d", fresh.Revision, baseRev+1)
	}
}

// TestProgressRetraction proves an explicit retraction keeps the retracted
// row in history superseded, falls the current position back to the next
// live observation or to absence, and rejects missing targets, double
// retractions, retractions of markers and missing reasons.
func TestProgressRetraction(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story G")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga G")

	e = mustRecordProgress(t, db, now.Add(2*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "42"})
	e = mustRecordProgress(t, db, now.Add(3*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "48"})
	currentID := e.Progress[0].ID

	mustRetract := func(recordID, reason string, expectedRevision int64) (Edition, error) {
		return db.RetractProgress(ctx(t), now.Add(4*time.Second), e.ID, expectedRevision, recordID, reason, progressActor(), nil)
	}

	e, err := mustRetract(currentID, "misread", e.Revision)
	if err != nil {
		t.Fatalf("RetractProgress: %v", err)
	}
	if e.Progress == nil || len(e.Progress) != 1 || *e.Progress[0].Position != "42" {
		t.Fatalf("current progress after retraction = %+v, want the older chapter 42", e.Progress)
	}
	rows := progressRows(t, db, e.ID)
	if len(rows) != 3 {
		t.Fatalf("progress rows = %d, want 3 (retraction stays in history)", len(rows))
	}
	if rows[2].Position != nil {
		t.Fatalf("retraction row position = %v, want NULL", rows[2].Position)
	}
	if rows[2].Reason == nil || *rows[2].Reason != "misread" {
		t.Fatalf("retraction reason = %v, want the supplied reason", rows[2].Reason)
	}
	if rows[2].SupersedesID == nil || *rows[2].SupersedesID != currentID {
		t.Fatalf("retraction supersedes = %v, want the retracted row", rows[2].SupersedesID)
	}

	markerID := rows[2].ID
	if _, err := mustRetract(markerID, "again", e.Revision); !errors.Is(err, ErrInvalidProgress) {
		t.Fatalf("retract a retraction marker: err = %v, want ErrInvalidProgress", err)
	}
	if _, err := mustRetract(currentID, "twice", e.Revision); !errors.Is(err, ErrSupersedeConflict) {
		t.Fatalf("second retraction of the same row: err = %v, want ErrSupersedeConflict", err)
	}
	missing := "00000000-0000-4000-8000-000000000000"
	if _, err := mustRetract(missing, "gone", e.Revision); !errors.Is(err, ErrProgressNotFound) {
		t.Fatalf("unknown retraction target: err = %v, want ErrProgressNotFound", err)
	}
	if _, err := mustRetract(currentID, " ", e.Revision); !errors.Is(err, ErrInvalidProgress) {
		t.Fatalf("blank retraction reason: err = %v, want ErrInvalidProgress", err)
	}

	// Reading again after a retraction records a fresh observation.
	e = mustRecordProgress(t, db, now.Add(5*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "10"})
	if e.Progress == nil || len(e.Progress) != 1 || *e.Progress[0].Position != "10" {
		t.Fatalf("current progress after reread = %+v, want chapter 10", e.Progress)
	}
}

// TestProgressRetractionFallsBackToOlder proves retracting the current row
// falls back to the next live observation, and retracting that too leaves
// the unit absent while all rows stay.
func TestProgressRetractionFallsBackToOlder(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story H")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga H")

	e = mustRecordProgress(t, db, now.Add(2*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "42"})
	olderID := e.Progress[0].ID
	e = mustRecordProgress(t, db, now.Add(3*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "48"})
	currentID := e.Progress[0].ID

	var err error
	e, err = db.RetractProgress(ctx(t), now.Add(4*time.Second), e.ID, e.Revision, currentID, "typo", progressActor(), nil)
	if err != nil {
		t.Fatalf("retract current: %v", err)
	}
	if e.Progress == nil || len(e.Progress) != 1 || *e.Progress[0].Position != "42" {
		t.Fatalf("current after retracting the latest = %+v, want the older chapter 42", e.Progress)
	}
	e, err = db.RetractProgress(ctx(t), now.Add(5*time.Second), e.ID, e.Revision, olderID, "all wrong", progressActor(), nil)
	if err != nil {
		t.Fatalf("retract older: %v", err)
	}
	if e.Progress != nil && len(e.Progress) != 0 {
		t.Fatalf("current after retracting everything = %+v, want absent", e.Progress)
	}
	if got := countProgressRows(t, db, e.ID); got != 4 {
		t.Fatalf("progress rows = %d, want 4 (two observations, two retractions)", got)
	}
}

// TestProgressLowerRereadAllowed proves a lower position is a valid new
// observation without requiring a reason, and history keeps both.
func TestProgressLowerRereadAllowed(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story I")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga I")

	e = mustRecordProgress(t, db, now.Add(2*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "48"})
	e = mustRecordProgress(t, db, now.Add(3*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "10"})
	if e.Progress == nil || len(e.Progress) != 1 || *e.Progress[0].Position != "10" {
		t.Fatalf("current after lower reread = %+v, want chapter 10", e.Progress)
	}
	if got := countProgressRows(t, db, e.ID); got != 2 {
		t.Fatalf("progress rows = %d, want 2 (the reread does not delete history)", got)
	}
}

// TestProgressUnknownTotalNoCarryForward proves a total is never carried
// forward: an observation without a total renders an unknown total even
// when an older row had one.
func TestProgressUnknownTotalNoCarryForward(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story J")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga J")

	e = mustRecordProgress(t, db, now.Add(2*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "42", Total: strPtr("120")})
	if e.Progress == nil || e.Progress[0].Total == nil || *e.Progress[0].Total != "120" {
		t.Fatalf("current total = %+v, want 120", e.Progress)
	}
	e = mustRecordProgress(t, db, now.Add(3*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "48"})
	if e.Progress == nil || e.Progress[0].Total != nil {
		t.Fatalf("current total after a no-total row = %+v, want unknown (no carry-forward)", e.Progress)
	}
	e = mustRecordProgress(t, db, now.Add(4*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "50", Total: strPtr("120")})
	if e.Progress == nil || e.Progress[0].Total == nil || *e.Progress[0].Total != "120" {
		t.Fatalf("current total after a known-total row = %+v, want 120", e.Progress)
	}
}

// TestProgressPastDatedUpdate proves a past-dated observation is stored
// but never becomes current over a newer one.
func TestProgressPastDatedUpdate(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story K")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga K")

	e = mustRecordProgress(t, db, now.Add(4*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "48"})
	e = mustRecordProgress(t, db, now.Add(5*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "20", ObservedAt: now.Add(2 * time.Second)})
	if e.Progress == nil || len(e.Progress) != 1 || *e.Progress[0].Position != "48" {
		t.Fatalf("current = %+v, want 48 (the past-dated row must not overtake)", e.Progress)
	}
	if got := countProgressRows(t, db, e.ID); got != 2 {
		t.Fatalf("progress rows = %d, want 2 (the past-dated row is stored)", got)
	}
}

// TestProgressIsolatesUnits proves units are independent: chapter and
// volume positions never sum or mix, and each unit keeps its own current.
func TestProgressIsolatesUnits(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story L")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindManga, "Manga L")

	e = mustRecordProgress(t, db, now.Add(2*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitChapter, Position: "42"})
	e = mustRecordProgress(t, db, now.Add(3*time.Second), e, e.Revision, ProgressInput{Unit: domain.UnitVolume, Position: "3"})
	if e.Progress == nil || len(e.Progress) != 2 {
		t.Fatalf("progress = %+v, want two independent units", e.Progress)
	}
	got := map[domain.ProgressUnit]string{}
	for _, p := range e.Progress {
		got[p.Unit] = *p.Position
	}
	if got[domain.UnitChapter] != "42" || got[domain.UnitVolume] != "3" {
		t.Fatalf("unit currents = %v, want chapter 42 and volume 3 exactly (never summed)", got)
	}
}

// TestProgressPublicationDoesNotAdvance proves recorded publication
// observations (release information) never create or change personal
// progress rows.
func TestProgressPublicationDoesNotAdvance(t *testing.T) {
	db := openMigrated(t, t.TempDir())
	defer db.Close()
	now := libraryNow()
	w := mustWork(t, db, now, "Story M")
	e, w := mustEdition(t, db, now.Add(time.Second), w, domain.KindAnime, "Anime M")

	pub, err := db.RecordPublication(ctx(t), now.Add(2*time.Second), e.ID, e.Revision, PublicationInput{
		Value: domain.PublicationOngoing, Source: strPtr("source"), Provenance: "import", ObservedAt: now.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatalf("RecordPublication: %v", err)
	}
	if pub.Progress != nil && len(pub.Progress) != 0 {
		t.Fatalf("progress after publication = %+v, want none (releases never advance reading)", pub.Progress)
	}
	if got := countProgressRows(t, db, e.ID); got != 0 {
		t.Fatalf("progress rows after publication = %d, want 0", got)
	}
}
