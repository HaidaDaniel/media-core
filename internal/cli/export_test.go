package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type exportDoc struct {
	Format string `json:"format"`
	Schema int    `json:"schema"`
	Tables []struct {
		Name string         `json:"name"`
		Rows []map[string]any `json:"rows"`
	} `json:"tables"`
}

type exportSummary struct {
	Path   string `json:"path"`
	Format string `json:"format"`
	Schema int    `json:"schema"`
	Tables int    `json:"tables"`
	Rows   int64  `json:"rows"`
}

func exportDataFrom(t *testing.T, out string) exportDoc {
	t.Helper()
	env := libEnv(t, out)
	if !env.OK || env.Error != nil {
		t.Fatalf("envelope = %+v, want ok", env)
	}
	b, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var doc exportDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("data is not an export document: %v\n%s", err, out)
	}
	return doc
}

// TestExportCLI exercises the export command surface: without --dest the
// versioned document rides the envelope, with --dest the raw document is
// published to a 0600 file and the envelope carries the summary, and
// argument errors keep the usage exit code.
func TestExportCLI(t *testing.T) {
	dir := initLibState(t)
	addWorkID(t, dir, "Export CLI Story")

	code, out, errw := runCLI(t, "export", "--state-dir", dir)
	if code != ExitOK {
		t.Fatalf("export: exit = %d, stderr = %q", code, errw)
	}
	doc := exportDataFrom(t, out)
	if doc.Format != "media-core-export-v1" {
		t.Fatalf("format = %q", doc.Format)
	}
	if doc.Schema <= 0 || len(doc.Tables) == 0 {
		t.Fatalf("document = %+v, want schema and tables", doc)
	}

	dest := filepath.Join(t.TempDir(), "export.json")
	code, out, errw = runCLI(t, "export", "--state-dir", dir, "--dest", dest)
	if code != ExitOK {
		t.Fatalf("export --dest: exit = %d, stderr = %q", code, errw)
	}
	env := libEnv(t, out)
	if !env.OK {
		t.Fatalf("envelope = %+v, want ok", env)
	}
	b, _ := json.Marshal(env.Data)
	var summary exportSummary
	if err := json.Unmarshal(b, &summary); err != nil {
		t.Fatalf("data is not an export summary: %v", err)
	}
	if summary.Path != dest || summary.Format != "media-core-export-v1" || summary.Rows == 0 {
		t.Fatalf("summary = %+v, want the file, the marker and rows", summary)
	}
	fi, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat dest: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("dest mode = %o, want 0600", fi.Mode().Perm())
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	var fileDoc exportDoc
	if err := json.Unmarshal(raw, &fileDoc); err != nil {
		t.Fatalf("file is not an export document: %v", err)
	}
	if fileDoc.Format != "media-core-export-v1" {
		t.Fatalf("file format = %q", fileDoc.Format)
	}
	found := false
	for _, table := range fileDoc.Tables {
		if table.Name != "work_titles" {
			continue
		}
		for _, row := range table.Rows {
			if row["text"] == "Export CLI Story" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("the seeded title is missing from the exported document")
	}

	code, _, _ = runCLI(t, "export", "--state-dir", dir, "--bogus")
	if code != ExitUsage {
		t.Fatalf("unknown flag: exit = %d, want %d", code, ExitUsage)
	}
	code, _, errw = runCLI(t, "export", "--state-dir", dir, "--dest", "relative.json")
	if code != ExitUsage || !strings.Contains(errw, "absolute path") {
		t.Fatalf("relative dest: exit = %d, stderr = %q, want usage with guidance", code, errw)
	}
}

// TestExportCLIMissingState proves the export fails with the stable schema
// guidance before any data is read, and the error is distinguishable from
// a storage fault.
func TestExportCLIMissingState(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	code, _, errw := runCLI(t, "export", "--state-dir", dir)
	if code != ExitError {
		t.Fatalf("export on missing state: exit = %d, want %d, stderr = %q", code, ExitError, errw)
	}
	if !strings.Contains(errw, "schema is missing") {
		t.Fatalf("stderr = %q, want the schema guidance", errw)
	}
}
