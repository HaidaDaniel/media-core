package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/runtime"
)

func init() {
	runtime.RegisterCommand("export", runExport)
}

const exportUsage = "usage: media-core export [--dest <file>]"

// exportArgs is one readable export request. Without --dest the versioned
// document rides the envelope on stdout; with --dest the raw document is
// written to the file and the envelope carries the summary.
type exportArgs struct {
	libCommand
	dest string
}

func parseExportArgs(args []string) (*exportArgs, error) {
	a := &exportArgs{}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--dest":
			if i+1 >= len(rest) {
				return nil, fmt.Errorf("media-core export: --dest requires a value; %s", exportUsage)
			}
			i++
			a.dest = rest[i]
		default:
			return nil, fmt.Errorf("media-core export: unknown argument %q; %s", rest[i], exportUsage)
		}
	}
	if a.dest != "" && !filepath.IsAbs(a.dest) {
		return nil, fmt.Errorf("media-core export: --dest must be an absolute path; %s", exportUsage)
	}
	return a, nil
}

// ExportSummary is the envelope payload when the document goes to a file.
type ExportSummary struct {
	Path   string `json:"path"`
	Format string `json:"format"`
	Schema int    `json:"schema"`
	Tables int    `json:"tables"`
	Rows   int64  `json:"rows"`
}

// runExport publishes one readable snapshot of the state database. The
// export is a projection: it never writes to the state directory, and the
// document it produces is not a restore input.
func runExport(args []string, out io.Writer) error {
	a, err := parseExportArgs(args)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	svc, db, err := a.service()
	if err != nil {
		return libExitError(err)
	}
	defer db.Close()

	var buf bytes.Buffer
	doc, err := svc.Export(context.Background(), &buf)
	if err != nil {
		return writeEnvelope(out, nil, libTyped(err))
	}
	if a.dest == "" {
		return writeEnvelope(out, doc, nil)
	}
	if err := writeExportFile(a.dest, buf.Bytes()); err != nil {
		return writeEnvelope(out, nil, typedStorageError("export: write file: "+err.Error()))
	}
	return writeEnvelope(out, ExportSummary{
		Path:   a.dest,
		Format: doc.Format,
		Schema: doc.Schema,
		Tables: len(doc.Tables),
		Rows:   doc.Rows(),
	}, nil)
}

// writeExportFile publishes the document to path with a 0600 mode that
// holds even when the file already exists: os.WriteFile would keep the
// old mode on replacement, and the document carries personal notes and
// progress.
func writeExportFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func typedStorageError(msg string) *app.AppError {
	return app.NewAppError(app.ErrStorageError, msg)
}
