package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/HaidaDaniel/media-core/internal/buildinfo"
)

// ExportFormatName is the format marker of the readable v1 export document.
// It is deliberately distinct from the backup archive marker
// ("media-core-snapshot-v1"): an export is a portable, human-readable
// projection of the state and is never a valid restore input.
const ExportFormatName = "media-core-export-v1"

// ExportApp is the build identity that produced the document.
type ExportApp struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

// ExportTable is one exported table in its stable row order. Rows carry
// every column, so IDs, provenance and the full observation history travel
// with the document.
type ExportTable struct {
	Name string           `json:"name"`
	Rows []map[string]any `json:"rows"`
}

// ExportDocument is the versioned readable export. The registry of exported
// tables lives in this package so later capabilities can extend the set
// without touching the writers; the document schema evolves by adding
// tables, never by changing the shape of the document.
type ExportDocument struct {
	Format    string        `json:"format"`
	CreatedAt time.Time     `json:"created_at"`
	App       ExportApp     `json:"app"`
	Schema    int           `json:"schema"`
	Tables    []ExportTable `json:"tables"`
}

// exportTableRegistry is the initial export registry: every current schema
// table with the column list that gives a stable, total row order. Entity
// rows order by their id; observation rows order by the same
// (observed_at, recorded_at, id) tuple the application itself uses to pick
// the current value, so the export reads in record order; note revisions
// order by their per-note revision counter. Task 0047 extends the registry
// for later domain/import tables and checks full coverage.
var exportTableRegistry = []struct {
	Name    string
	OrderBy string
}{
	{"works", "id"},
	{"work_titles", "id"},
	{"editions", "id"},
	{"edition_titles", "id"},
	{"publication_observations", "observed_at, recorded_at, id"},
	{"external_identities", "id"},
	{"mutation_journal", "request_id"},
	{"consumption_observations", "observed_at, recorded_at, id"},
	{"progress_observations", "observed_at, recorded_at, id"},
	{"intention_observations", "observed_at, recorded_at, id"},
	{"tags", "id"},
	{"tag_assignments", "id"},
	{"notes", "id"},
	{"note_revisions", "note_id, revision"},
	{"rating_observations", "observed_at, recorded_at, id"},
	{"external_references", "id"},
	{"media_relations", "id"},
	{"origin_claims", "id"},
	{"import_events", "id"},
	{"import_bindings", "id"},
	{"schema_migrations", "version"},
}

// Rows sums the exported rows across every table.
func (d ExportDocument) Rows() int64 {
	var n int64
	for _, t := range d.Tables {
		n += int64(len(t.Rows))
	}
	return n
}

// Export writes one versioned readable export document to w. Every table is
// read inside a single read-only transaction, which fixes the WAL read
// snapshot: the document reflects one consistent moment and never mixes a
// partially committed write. A read-only BEGIN is deferred, so in WAL mode
// the export never takes the write lock and coexists with a live writer;
// the snapshot is fixed at the first read. Values are passed through
// verbatim — the export re-validates no domain value, so rows written by
// another build stay readable.
func (s *Service) Export(ctx context.Context, w io.Writer) (ExportDocument, error) {
	tx, err := s.db.DB().BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ExportDocument{}, fmt.Errorf("app: begin export snapshot: %w", err)
	}
	doc := ExportDocument{
		Format:    ExportFormatName,
		CreatedAt: time.Now().UTC(),
		App: ExportApp{
			Version:   buildinfo.Get().Version,
			Commit:    buildinfo.Get().Commit,
			BuildTime: buildinfo.Get().BuildTime,
			GoVersion: buildinfo.Get().GoVersion,
		},
		Tables: []ExportTable{},
	}
	for _, table := range exportTableRegistry {
		rows, err := exportTableRows(tx, table.Name, table.OrderBy)
		if err != nil {
			_ = tx.Rollback()
			return ExportDocument{}, fmt.Errorf("app: export table %s: %w", table.Name, err)
		}
		doc.Tables = append(doc.Tables, ExportTable{Name: table.Name, Rows: rows})
	}
	if err := tx.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&doc.Schema); err != nil {
		_ = tx.Rollback()
		return ExportDocument{}, fmt.Errorf("app: export schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ExportDocument{}, fmt.Errorf("app: end export snapshot: %w", err)
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return doc, fmt.Errorf("app: write export document: %w", err)
	}
	return doc, nil
}

// exportTableRows reads one table inside the caller's snapshot transaction
// in the registry's stable order. Column names come from the live schema,
// so a table that gains columns keeps exporting without code changes.
func exportTableRows(tx *sql.Tx, name, orderBy string) ([]map[string]any, error) {
	rows, err := tx.Query("SELECT * FROM " + name + " ORDER BY " + orderBy)
	if err != nil {
		return nil, fmt.Errorf("store: query export table: %w", err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("store: export table columns: %w", err)
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("store: scan export row: %w", err)
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = exportValue(vals[i])
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: read export rows: %w", err)
	}
	return out, nil
}

// exportValue normalizes the driver's scalar set for stable JSON: blobs
// render as their UTF-8 text (the schema is text-only), everything else
// keeps its native JSON shape.
func exportValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}
