// Process-level counterexamples for the shared SQLite and maintenance
// contract. Writers racing through the same shared service (the
// HTTP-equivalent every adapter reaches) must never create duplicate exact
// identities or half history rows and must never lose a successful commit;
// a blocked maintenance open is bounded and reports a stable locked error;
// an active handle blocks restore/migrate; and a writer killed mid-transaction
// recovers cleanly — its committed writes survive, its uncommitted write is
// discarded, and the database stays consistent.
package concurrency

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// openService migrates a fresh temporary state dir and opens it under the
// shared lifetime lock, returning the service and the store handle.
func openService(t *testing.T) (*app.Service, *store.Database) {
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
	return app.NewService(db), db
}

func countWorks(t *testing.T, db *store.Database) int {
	t.Helper()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM works`).Scan(&n); err != nil {
		t.Fatalf("count works: %v", err)
	}
	return n
}

func appErr(t *testing.T, err error) *app.AppError {
	t.Helper()
	var ae *app.AppError
	if err == nil || !errors.As(err, &ae) {
		t.Fatalf("error = %v, want *app.AppError", err)
	}
	return ae
}

func strPtr(s string) *string { return &s }

// TestConcurrentExactAdds proves racing exact adds never create a duplicate
// identity: an exact idempotent replay (same key, identical payload) yields
// one row and the same result for every caller, and racing adds of the same
// title under distinct keys create exactly one work with the rest a stable
// duplicate_candidate.
func TestConcurrentExactAdds(t *testing.T) {
	t.Run("exact replay same key", func(t *testing.T) {
		svc, db := openService(t)
		ctx := context.Background()
		const n = 8
		results := make([]app.AdditionResult, n)
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				req := app.AdditionRequest{
					MutationBase: app.MutationBase{RequestID: "exact-add", Actor: app.Actor{Principal: "local", Origin: domain.OriginUser}},
					Titles:       []store.TitleInput{{Text: "Raced Story", Kind: domain.TitleOriginal, Provenance: "manual"}},
				}
				results[i], errs[i] = svc.CreateLibraryEntry(ctx, req)
			}(i)
		}
		wg.Wait()
		workID := ""
		for i := 0; i < n; i++ {
			if errs[i] != nil {
				t.Fatalf("goroutine %d: %v", i, errs[i])
			}
			if !results[i].Created {
				t.Fatalf("goroutine %d: Created = false, want true", i)
			}
			if workID == "" {
				workID = results[i].WorkID
			} else if results[i].WorkID != workID {
				t.Fatalf("goroutine %d: WorkID = %s, want %s (one exact identity)", i, results[i].WorkID, workID)
			}
		}
		if got := countWorks(t, db); got != 1 {
			t.Fatalf("works = %d, want 1 (no duplicate exact identity)", got)
		}
	})

	t.Run("same identity different key", func(t *testing.T) {
		svc, db := openService(t)
		ctx := context.Background()
		const n = 6
		errs := make([]error, n)
		var mu sync.Mutex
		created := 0
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				req := app.AdditionRequest{
					MutationBase: app.MutationBase{RequestID: fmt.Sprintf("id-add-%d", i), Actor: app.Actor{Principal: "local", Origin: domain.OriginUser}},
					Titles:       []store.TitleInput{{Text: "Identity Story", Kind: domain.TitleOriginal, Provenance: "manual"}},
				}
				res, err := svc.CreateLibraryEntry(ctx, req)
				if err != nil {
					errs[i] = err
					return
				}
				mu.Lock()
				if res.Created {
					created++
				}
				mu.Unlock()
			}(i)
		}
		wg.Wait()
		conflicts := 0
		for i := 0; i < n; i++ {
			if errs[i] == nil {
				continue
			}
			if code := appErr(t, errs[i]).Code; code != app.ErrDuplicateCandidate {
				t.Fatalf("goroutine %d: code = %s, want duplicate_candidate", i, code)
			}
			conflicts++
		}
		if created != 1 {
			t.Fatalf("created = %d, want exactly 1", created)
		}
		if conflicts != n-1 {
			t.Fatalf("conflicts = %d, want %d", conflicts, n-1)
		}
		if got := countWorks(t, db); got != 1 {
			t.Fatalf("works = %d, want 1 (no duplicate exact identity)", got)
		}
	})
}

// TestConcurrentStaleRevisionUpdates proves racing stale-revision updates
// yields exactly one winner and stable conflict errors for the rest, with the
// revision advanced exactly once (no double apply, no half history row). The
// edition is kind other, whose descriptive label is the metadata field being
// corrected (other kinds forbid a label, so it is the field that is allowed).
func TestConcurrentStaleRevisionUpdates(t *testing.T) {
	svc, db := openService(t)
	ctx := context.Background()
	now := time.Now().UTC()
	w, err := db.CreateWork(ctx, now, []store.TitleInput{{Text: "Revision Story", Kind: domain.TitleOriginal, Provenance: "manual"}})
	if err != nil {
		t.Fatalf("CreateWork: %v", err)
	}
	e, err := db.AddEdition(ctx, now, w.ID, w.Revision, store.EditionInput{Kind: domain.KindOther, OtherLabel: strPtr("initial label")})
	if err != nil {
		t.Fatalf("AddEdition: %v", err)
	}
	editionID := e.ID
	ev, err := svc.GetEdition(ctx, editionID)
	if err != nil {
		t.Fatalf("GetEdition: %v", err)
	}
	stale := ev.Revision
	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			exp := stale
			req := app.UpdateEditionRequest{
				MutationBase: app.MutationBase{RequestID: fmt.Sprintf("rev-upd-%d", i), Actor: app.Actor{Principal: "local", Origin: domain.OriginUser}, ExpectedRevision: &exp},
				EditionID:    editionID,
				OtherLabel:   strPtr(fmt.Sprintf("label-%d", i)),
			}
			_, errs[i] = svc.UpdateEdition(ctx, req)
		}(i)
	}
	wg.Wait()
	succeeded := 0
	for i := 0; i < n; i++ {
		if errs[i] == nil {
			succeeded++
			continue
		}
		if code := appErr(t, errs[i]).Code; code != app.ErrConflict {
			t.Fatalf("goroutine %d: code = %s, want conflict", i, code)
		}
	}
	if succeeded != 1 {
		t.Fatalf("succeeded = %d, want exactly 1 (one winner, the rest stable conflicts)", succeeded)
	}
	after, err := svc.GetEdition(ctx, editionID)
	if err != nil {
		t.Fatalf("GetEdition after: %v", err)
	}
	if after.Revision != stale+1 {
		t.Fatalf("revision = %d, want %d (exactly one update applied)", after.Revision, stale+1)
	}
}

// TestActiveHandleBlocksMaintenance proves two shared handles coexist, an
// active shared handle blocks an exclusive maintenance open (restore/migrate)
// with a stable locked error after the bounded deadline, and the exclusive
// open succeeds once the handles are released.
func TestActiveHandleBlocksMaintenance(t *testing.T) {
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

	dbA, err := store.Open(dir)
	if err != nil {
		t.Fatalf("Open A: %v", err)
	}
	defer dbA.Close()
	dbB, err := store.Open(dir)
	if err != nil {
		t.Fatalf("Open B: shared handles must coexist: %v", err)
	}
	defer dbB.Close()

	start := time.Now()
	_, err = store.OpenForMaintenance(dir)
	elapsed := time.Since(start)
	if !errors.Is(err, store.ErrLocked) {
		t.Fatalf("OpenForMaintenance while held: err = %v, want ErrLocked", err)
	}
	if elapsed < 4*time.Second {
		t.Fatalf("blocked open returned in %v, want the bounded ~5s lock deadline", elapsed)
	}
	if elapsed > 30*time.Second {
		t.Fatalf("blocked open took %v, want bounded by the deadline", elapsed)
	}

	// Once the shared handles are gone the exclusive maintenance open succeeds.
	_ = dbA.Close()
	_ = dbB.Close()
	if _, err := store.OpenForMaintenance(dir); err != nil {
		t.Fatalf("OpenForMaintenance after release: %v, want success", err)
	}
}

const (
	roleEnv = "MEDIA_CORE_TEST_CONC_ROLE"
	dirEnv  = "MEDIA_CORE_TEST_CONC_DIR"
)

// TestKilledWriterRecovery proves a writer killed mid-transaction recovers
// cleanly: its committed write survives the reopen (no lost successful
// commit), its uncommitted write is discarded (no half history row), and the
// database passes an integrity check.
func TestKilledWriterRecovery(t *testing.T) {
	if role := os.Getenv(roleEnv); role != "" {
		childWriterRole(os.Getenv(dirEnv))
		return
	}
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

	bin, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cmd := exec.Command(bin, "-test.run=TestKilledWriterRecovery")
	cmd.Env = append(os.Environ(), roleEnv+"=writer", dirEnv+"="+dir)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	reaped := false
	defer func() {
		if !reaped {
			_ = cmd.Process.Kill()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
			}
		}
	}()

	waitForFile(t, filepath.Join(dir, "held"), 20*time.Second)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill child: %v", err)
	}
	<-done
	reaped = true

	db, err := store.Open(dir)
	if err != nil {
		t.Fatalf("reopen after kill: %v", err)
	}
	defer db.Close()
	var committed, uncommitted int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM works WHERE id = 'work-committed'`).Scan(&committed); err != nil {
		t.Fatalf("read committed: %v", err)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM works WHERE id = 'work-uncommitted'`).Scan(&uncommitted); err != nil {
		t.Fatalf("read uncommitted: %v", err)
	}
	if committed != 1 {
		t.Fatalf("committed work rows = %d, want 1 (no lost successful commit)", committed)
	}
	if uncommitted != 0 {
		t.Fatalf("uncommitted work rows = %d, want 0 (no half history row)", uncommitted)
	}
	var ok string
	if err := db.DB().QueryRow(`PRAGMA integrity_check`).Scan(&ok); err != nil {
		t.Fatalf("integrity_check: %v", err)
	}
	if ok != "ok" {
		t.Fatalf("integrity_check = %q, want ok", ok)
	}
}

// childWriterRole is the child process: open the shared handle, commit one
// write, open an uncommitted write transaction, signal, then block until
// killed.
func childWriterRole(dir string) {
	db, err := store.Open(dir)
	if err != nil {
		os.Exit(1)
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if _, err := db.DB().Exec(`INSERT INTO works (id, revision, created_at, updated_at) VALUES ('work-committed', 0, ?, ?)`, now, now); err != nil {
		os.Exit(1)
	}
	tx, err := db.DB().Begin()
	if err != nil {
		os.Exit(1)
	}
	if _, err := tx.Exec(`INSERT INTO works (id, revision, created_at, updated_at) VALUES ('work-uncommitted', 0, ?, ?)`, now, now); err != nil {
		os.Exit(1)
	}
	// Deliberately do not commit: the transaction stays open and uncommitted.
	if err := os.WriteFile(filepath.Join(dir, "held"), []byte("held"), 0o600); err != nil {
		os.Exit(1)
	}
	select {}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
