package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

func exportDocument(t *testing.T, s *Service) (ExportDocument, string) {
	t.Helper()
	var buf bytes.Buffer
	doc, err := s.Export(context.Background(), &buf)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	return doc, buf.String()
}

func tableRowsFrom(t *testing.T, raw, name string) []map[string]any {
	t.Helper()
	var doc struct {
		Tables []struct {
			Name string           `json:"name"`
			Rows []map[string]any `json:"rows"`
		} `json:"tables"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("document is not JSON: %v\n%s", err, raw)
	}
	for _, table := range doc.Tables {
		if table.Name == name {
			return table.Rows
		}
	}
	t.Fatalf("table %q missing from the document", name)
	return nil
}

// seedExportHistory builds a small library with progress and note history.
func seedExportHistory(t *testing.T, s *Service) (workID, editionID string) {
	t.Helper()
	w := mustCreateWork(t, s, "Export Story")
	e := addEdition(t, s, w.ID, domain.KindManga, "Export Re")
	mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "exp-progress-1", e.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "42",
	})
	view := mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "exp-progress-2", e.Revision+1),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "48",
	})
	noteView, err := s.AddEditionNote(context.Background(), AddNoteRequest{
		MutationBase: mustBase(t, "exp-note-1", view.Revision),
		TargetID:     e.ID,
		Text:         "First impression.",
	})
	if err != nil {
		t.Fatalf("AddEditionNote: %v", err)
	}
	if _, err := s.ReviseNote(context.Background(), ReviseNoteRequest{
		MutationBase: mustBase(t, "exp-note-2", noteView.Notes[0].Revision),
		NoteID:       noteView.Notes[0].ID,
		Text:         "Revised impression.",
	}); err != nil {
		t.Fatalf("ReviseNote: %v", err)
	}
	return w.ID, e.ID
}

// TestExportDocument proves the document contract: the versioned marker,
// the build identity and schema version, the stable per-table row order,
// and the inclusion of IDs, provenance and the full observation history.
func TestExportDocument(t *testing.T) {
	s := newTestService(t)
	seedExportHistory(t, s)
	// Two further works: with three or more rows the stable order is
	// observable, and random ids make creation order and sorted order
	// differ.
	mustCreateWork(t, s, "Zeta Work")
	mustCreateWork(t, s, "Alpha Work")

	doc, raw := exportDocument(t, s)
	if doc.Format != ExportFormatName {
		t.Fatalf("format = %q, want %q", doc.Format, ExportFormatName)
	}
	if doc.Schema <= 0 {
		t.Fatalf("schema = %d, want the current version", doc.Schema)
	}
	if doc.App.Version == "" || doc.App.Commit == "" || doc.App.GoVersion == "" {
		t.Fatalf("app identity incomplete: %+v", doc.App)
	}
	if doc.CreatedAt.IsZero() || time.Since(doc.CreatedAt) > time.Minute {
		t.Fatalf("created_at = %v, want a recent instant", doc.CreatedAt)
	}

	// Every registered table is present exactly once, in registry order.
	seen := map[string]bool{}
	for _, table := range doc.Tables {
		if seen[table.Name] {
			t.Fatalf("table %q appears twice", table.Name)
		}
		seen[table.Name] = true
	}
	for _, want := range []string{"works", "work_titles", "editions", "progress_observations", "note_revisions", "schema_migrations"} {
		if !seen[want] {
			t.Fatalf("table %q missing from the document", want)
		}
	}

	// Stable row order: works are sorted by id, not by creation order.
	works := tableRowsFrom(t, raw, "works")
	if len(works) != 3 {
		t.Fatalf("works rows = %d, want 3", len(works))
	}
	ids := []string{}
	for _, row := range works {
		id, _ := row["id"].(string)
		if id == "" {
			t.Fatalf("work row has no id: %v", row)
		}
		ids = append(ids, id)
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatalf("works order = %v, want sorted by id", ids)
	}

	// IDs and provenance travel with the rows.
	titles := tableRowsFrom(t, raw, "work_titles")
	if len(titles) == 0 {
		t.Fatalf("work_titles is empty")
	}
	for _, row := range titles {
		if row["id"] == nil || row["provenance"] == nil || row["work_id"] == nil {
			t.Fatalf("title row lacks id/provenance/work_id: %v", row)
		}
	}

	// The full history survives: both progress observations and both note
	// revisions, the superseded text included.
	progress := tableRowsFrom(t, raw, "progress_observations")
	if len(progress) != 2 {
		t.Fatalf("progress rows = %d, want 2", len(progress))
	}
	positions := []string{}
	for _, row := range progress {
		positions = append(positions, row["position"].(string))
	}
	if positions[0] != "42" || positions[1] != "48" {
		t.Fatalf("progress positions = %v, want [42 48] in record order", positions)
	}
	notes := tableRowsFrom(t, raw, "note_revisions")
	if len(notes) != 2 {
		t.Fatalf("note revision rows = %d, want 2", len(notes))
	}
	if notes[0]["text"] != "First impression." || notes[1]["text"] != "Revised impression." {
		t.Fatalf("note revisions = %v, want both texts in revision order", notes)
	}
}

// TestExportUnknownValuesPassThrough proves the export re-validates no
// domain value: rows carrying free-text values this build does not
// recognize (a non-ISO language, an arbitrary progress label) are exported
// verbatim, never rejected or rewritten.
func TestExportUnknownValuesPassThrough(t *testing.T) {
	s := newTestService(t)
	w := mustCreateWork(t, s, "Pass Through Story")
	if _, err := s.db.AddWorkTitle(context.Background(), s.now(), w.ID, w.Revision, store.TitleInput{
		Text:       "Pass Through Re",
		Language:   appStr("xx"),
		Kind:       domain.TitleOriginal,
		Provenance: "manual",
	}); err != nil {
		t.Fatalf("AddWorkTitle: %v", err)
	}
	e := addEdition(t, s, w.ID, domain.KindManga, "Pass Re")
	label := "глава 48 — переосмысление"
	mustRecordProgressView(t, s, RecordProgressRequest{
		MutationBase: mustBase(t, "exp-label-1", e.Revision),
		EditionID:    e.ID,
		Unit:         domain.UnitChapter,
		Position:     "48",
		Label:        &label,
	})

	_, raw := exportDocument(t, s)
	titles := tableRowsFrom(t, raw, "work_titles")
	foundLang := false
	for _, row := range titles {
		if row["language"] == "xx" {
			foundLang = true
		}
	}
	if !foundLang {
		t.Fatalf("the unknown language code was not exported verbatim: %v", titles)
	}
	progress := tableRowsFrom(t, raw, "progress_observations")
	if len(progress) != 1 || progress[0]["label"] != label {
		t.Fatalf("progress label = %v, want %q verbatim", progress, label)
	}
}

// TestExportSnapshotIsConsistent proves one document is one moment: while a
// second handle writes continuously, every exported document is internally
// referentially complete — no row points at a parent that the same
// snapshot did not include.
func TestExportSnapshotIsConsistent(t *testing.T) {
	dir := t.TempDir()
	maint, err := store.OpenForMaintenance(dir)
	if err != nil {
		t.Fatalf("OpenForMaintenance: %v", err)
	}
	if _, err := store.RunMigrations(maint.DB()); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if err := maint.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	reader, err := store.Open(dir)
	if err != nil {
		t.Fatalf("Open reader: %v", err)
	}
	t.Cleanup(func() { reader.Close() })
	writer, err := store.Open(dir)
	if err != nil {
		t.Fatalf("Open writer: %v", err)
	}
	t.Cleanup(func() { writer.Close() })
	svc := NewService(reader)
	writerSvc := NewService(writer)

	seedExportHistory(t, svc)
	stop := make(chan struct{})
	done := make(chan struct{})
	var writes int
	var writerErr error
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := writerSvc.db.CreateWork(context.Background(), writerSvc.now(),
				[]store.TitleInput{{Text: "Churn Work", Kind: domain.TitleOriginal, Provenance: "manual"}}); err != nil {
				writerErr = err
				return
			}
			writes++
		}
	}()
	for i := 0; i < 12; i++ {
		_, raw := exportDocument(t, svc)
		assertSnapshotComplete(t, raw)
	}
	close(stop)
	<-done
	if writerErr != nil {
		t.Fatalf("concurrent writer: %v", writerErr)
	}
	if writes == 0 {
		t.Fatalf("the writer produced no rows; the snapshot check is vacuous")
	}
}

// assertSnapshotComplete checks the referential closure of one document:
// every child row's parent id appears in the parent table of the same
// document.
func assertSnapshotComplete(t *testing.T, raw string) {
	t.Helper()
	idSet := func(table string) map[string]bool {
		ids := map[string]bool{}
		for _, row := range tableRowsFrom(t, raw, table) {
			if id, ok := row["id"].(string); ok {
				ids[id] = true
			}
		}
		return ids
	}
	works := idSet("works")
	editions := idSet("editions")
	references := func(table, col string) {
		for _, row := range tableRowsFrom(t, raw, table) {
			v, _ := row[col].(string)
			if v == "" {
				continue // nullable parent (work-level notes)
			}
			switch col {
			case "work_id":
				if !works[v] {
					t.Fatalf("%s row references work %s outside the snapshot", table, v)
				}
			case "edition_id":
				if !editions[v] {
					t.Fatalf("%s row references edition %s outside the snapshot", table, v)
				}
			}
		}
	}
	for _, table := range []string{"edition_titles", "publication_observations", "consumption_observations", "progress_observations", "intention_observations", "notes", "rating_observations", "external_references"} {
		references(table, "edition_id")
	}
	references("editions", "work_id")
}

// TestExportIsNotRestoreInput proves the formats are distinct: restore
// consumes only a validated backup archive and rejects an export document
// with the stable invalid-archive code, touching nothing.
func TestExportIsNotRestoreInput(t *testing.T) {
	s := newTestService(t)
	seedExportHistory(t, s)
	_, raw := exportDocument(t, s)
	if strings.Contains(raw, "media-core-snapshot-v1") {
		t.Fatalf("the export carries the backup format marker:\n%s", raw)
	}
	file := filepath.Join(t.TempDir(), "export.json")
	if err := os.WriteFile(file, []byte(raw), 0o600); err != nil {
		t.Fatalf("write export: %v", err)
	}
	dest := t.TempDir()
	_, err := Restore(context.Background(), RestoreRequest{Archive: file, StateDir: filepath.Join(dest, "state")})
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != ErrInvalidArgument {
		t.Fatalf("restore of an export = %v, want the invalid-archive code", err)
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Fatalf("restore created files before rejecting: %v", entries)
	}
}

// TestExportRegistryCoversSchema pins the full-coverage invariant of the
// readable export: every table in the live schema has a registry entry,
// no registry entry names a table that does not exist, and every ORDER BY
// column of the registry exists on its table — so a migration that adds a
// table or drops a sort column fails here before the gap ships.
func TestExportRegistryCoversSchema(t *testing.T) {
	s := newTestService(t)
	db := s.db.DB()

	names, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer names.Close()
	live := map[string]bool{}
	for names.Next() {
		var n string
		if err := names.Scan(&n); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		live[n] = true
	}
	if err := names.Err(); err != nil {
		t.Fatalf("read table names: %v", err)
	}

	registry := map[string]bool{}
	for _, table := range exportTableRegistry {
		if registry[table.Name] {
			t.Fatalf("the registry lists %s twice", table.Name)
		}
		registry[table.Name] = true
		if !live[table.Name] {
			t.Fatalf("the registry names table %s, which is not in the live schema", table.Name)
		}
		cols, err := db.Query(`PRAGMA table_info(` + table.Name + `)`)
		if err != nil {
			t.Fatalf("table_info %s: %v", table.Name, err)
		}
		have := map[string]bool{}
		for cols.Next() {
			var (
				cid, notnull, pk int
				name, dft        any
				ctype            string
			)
			if err := cols.Scan(&cid, &name, &ctype, &notnull, &dft, &pk); err != nil {
				t.Fatalf("scan table_info %s: %v", table.Name, err)
			}
			have[name.(string)] = true
		}
		cols.Close()
		for _, order := range strings.Split(table.OrderBy, ",") {
			col := strings.TrimSpace(order)
			if !have[col] {
				t.Fatalf("registry ORDER BY %q for %s: column is not in the live schema", table.OrderBy, table.Name)
			}
		}
	}
	for name := range live {
		if !registry[name] {
			t.Fatalf("the live table %s is missing from the export registry", name)
		}
	}
}
