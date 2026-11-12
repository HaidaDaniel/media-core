package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/config"
	"github.com/HaidaDaniel/media-core/internal/runtime"
)

func init() {
	runtime.RegisterCommand("restore", runRestore)
}

const restoreUsage = "usage: media-core restore <archive> [--state-dir <dir>] [--force]"

// restoreArgs is one archive restore request. The state dir flag selects
// the destination state directory, not a source.
type restoreArgs struct {
	stateDir string
	archive  string
	force    bool
}

func parseRestoreArgs(args []string) (*restoreArgs, error) {
	a := &restoreArgs{}
	var rest []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("media-core restore: --state-dir requires a value")
			}
			i++
			a.stateDir = args[i]
		case "--force":
			a.force = true
		default:
			rest = append(rest, args[i])
		}
	}
	if len(rest) != 1 {
		return nil, fmt.Errorf("media-core restore: %s", restoreUsage)
	}
	a.archive = rest[0]
	return a, nil
}

// runRestore restores one validated archive into the destination state
// directory and prints the restore envelope. It never opens the
// destination database first: the restore takes the exclusive lifetime
// maintenance lock itself, which is the replacement gate.
func runRestore(args []string, out io.Writer) error {
	a, err := parseRestoreArgs(args)
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitUsage, Err: err}
	}
	cfg, err := config.Resolve(config.Options{StateDir: a.stateDir})
	if err != nil {
		return &runtime.ExitCodeError{Exit: ExitError, Err: err}
	}
	payload, err := app.Restore(context.Background(), app.RestoreRequest{
		Archive:  a.archive,
		StateDir: cfg.StateDir,
		Force:    a.force,
	})
	if err != nil {
		return writeEnvelope(out, nil, libTyped(err))
	}
	return writeEnvelope(out, payload, nil)
}
