package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/importer"
	"github.com/HaidaDaniel/media-core/internal/runtime"
)

// appImportNamespace is the stable Release Watcher producer namespace.
const appImportNamespace = importer.DefaultNamespace

func init() {
	runtime.RegisterCommand("import", runImport)
	runtime.RegisterCommand("pending-imports", runPendingImports)
}

const importUsage = "usage: media-core import releases --from <file|-> [--max-bytes N] [--max-events N]"

const pendingImportsUsage = "usage: media-core pending-imports [--limit N] [--cursor C]"

// importArgs is one Release Watcher import request.
type importArgs struct {
	libCommand
	from      string
	maxBytes  int64
	maxEvents int
}

func parseImportArgs(args []string) (*importArgs, error) {
	a := &importArgs{}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	if len(rest) < 1 || rest[0] != "releases" {
		return nil, fmt.Errorf("media-core import: %s", importUsage)
	}
	for i := 1; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core import: %s requires a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		switch rest[i] {
		case "--from":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.from = v
		case "--max-bytes":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			n, perr := parsePositiveInt(v)
			if perr != nil {
				return nil, fmt.Errorf("media-core import: --max-bytes: %v", perr)
			}
			a.maxBytes = int64(n)
		case "--max-events":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			n, perr := parsePositiveInt(v)
			if perr != nil {
				return nil, fmt.Errorf("media-core import: --max-events: %v", perr)
			}
			a.maxEvents = n
		default:
			return nil, fmt.Errorf("media-core import: unknown argument %q (%s)", rest[i], importUsage)
		}
	}
	if a.from == "" {
		return nil, fmt.Errorf("media-core import: --from is required; %s", importUsage)
	}
	return a, nil
}

// runImport stages one Release Watcher version 1.0 envelope (file or stdin)
// as immutable pending records and prints the per-item result envelope. A
// decode failure prints a failure envelope; a partial import (one or more
// identity conflicts) prints the result and exits 3.
func runImport(args []string, out io.Writer) error {
	a, err := parseImportArgs(args)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	raw, err := readImportSource(a.from)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	svc, db, err := a.service()
	if err != nil {
		return libExitError(err)
	}
	defer db.Close()
	req := app.ImportRequest{
		RequestID: "import-releases",
		Namespace: appImportNamespace,
		Actor:     app.Actor{Principal: "local", Origin: domain.OriginUser},
		Raw:       raw,
		MaxBytes:  a.maxBytes,
		MaxEvents: a.maxEvents,
	}
	payload, err := svc.ImportEvents(context.Background(), req)
	if err != nil {
		return writeEnvelope(out, nil, libTyped(err))
	}
	if err := writeEnvelope(out, payload, nil); err != nil {
		return err
	}
	if payload.Conflicts > 0 {
		return &runtime.ExitCodeError{Exit: ExitPartialImport, Err: fmt.Errorf("partial import: %d event(s) conflicted", payload.Conflicts)}
	}
	return nil
}

// readImportSource reads the import envelope from stdin ("-" ) or a file.
func readImportSource(from string) ([]byte, error) {
	if from == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("media-core import: read stdin: %w", err)
		}
		return b, nil
	}
	b, err := os.ReadFile(from)
	if err != nil {
		return nil, fmt.Errorf("media-core import: read %q: %w", from, err)
	}
	return b, nil
}

// parsePositiveInt parses a positive integer flag value.
func parsePositiveInt(v string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not a positive integer", v)
	}
	return n, nil
}

// pendingArgs is one pending-imports listing request.
type pendingArgs struct {
	libCommand
	limit  int
	cursor *string
}

func parsePendingArgs(args []string) (*pendingArgs, error) {
	a := &pendingArgs{}
	rest, err := a.parseCommon(args)
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(rest); i++ {
		needValue := func() (string, error) {
			if i+1 >= len(rest) {
				return "", fmt.Errorf("media-core pending-imports: %s requires a value", rest[i])
			}
			i++
			return rest[i], nil
		}
		switch rest[i] {
		case "--limit":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			n, perr := parsePositiveInt(v)
			if perr != nil {
				return nil, fmt.Errorf("media-core pending-imports: --limit: %v", perr)
			}
			a.limit = n
		case "--cursor":
			v, err := needValue()
			if err != nil {
				return nil, err
			}
			a.cursor = &v
		default:
			return nil, fmt.Errorf("media-core pending-imports: unknown argument %q (%s)", rest[i], pendingImportsUsage)
		}
	}
	return a, nil
}

// runPendingImports lists the pending import queue and prints the result
// envelope.
func runPendingImports(args []string, out io.Writer) error {
	a, err := parsePendingArgs(args)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	svc, db, err := a.service()
	if err != nil {
		return libExitError(err)
	}
	defer db.Close()
	payload, err := svc.PendingImports(context.Background(), app.PendingImportQuery{Limit: a.limit, Cursor: a.cursor})
	if err != nil {
		return writeEnvelope(out, nil, libTyped(err))
	}
	return writeEnvelope(out, payload, nil)
}
