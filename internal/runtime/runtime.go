// Package runtime owns process lifecycle and mode dispatch for media-core.
package runtime

import (
	"fmt"
	"io"

	"github.com/HaidaDaniel/media-core/internal/buildinfo"
	"github.com/HaidaDaniel/media-core/internal/config"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// ResolveConfig resolves process configuration for a mode. It is pure: no
// state directory is created and no secret value is rendered. The reserved
// serve and mcp lifecycles will use it when implemented.
func ResolveConfig(opts config.Options) (*config.Config, error) {
	return config.Resolve(opts)
}

// OpenDatabase opens the process database for a resolved configuration
// under the shared lifetime maintenance lock.
func OpenDatabase(cfg *config.Config) (*store.Database, error) {
	return store.Open(cfg.StateDir)
}

// OpenDatabaseForMaintenance opens the database under the exclusive
// maintenance lock for init, migration and restore.
func OpenDatabaseForMaintenance(cfg *config.Config) (*store.Database, error) {
	return store.OpenForMaintenance(cfg.StateDir)
}

// CommandHandler implements one mode command beyond the bootstrap set.
// Capability packages register their commands at initialization time so
// runtime stays free of capability imports.
type CommandHandler func(args []string, out io.Writer) error

var commandHandlers = map[string]CommandHandler{}

// RegisterCommand makes a mode command available through Dispatch.
func RegisterCommand(name string, h CommandHandler) {
	commandHandlers[name] = h
}

// ExitCodeError carries an explicit process exit code for a command
// failure so the CLI adapter maps it to the contract exit codes.
type ExitCodeError struct {
	Exit int
	Err  error
}

func (e *ExitCodeError) Error() string { return e.Err.Error() }
func (e *ExitCodeError) Unwrap() error { return e.Err }

// ErrMissingCommand is returned when no command is supplied.
var ErrMissingCommand = fmt.Errorf("media-core: missing command")

// ErrReserved is returned for commands that exist in the design but are not
// implemented yet.
type ErrReserved struct {
	Command string
}

func (e ErrReserved) Error() string {
	return fmt.Sprintf("media-core: command %q is reserved and not implemented yet", e.Command)
}

// ErrUnsupported is returned for commands outside the known set.
type ErrUnsupported struct {
	Command string
}

func (e ErrUnsupported) Error() string {
	return fmt.Sprintf("media-core: unsupported command %q", e.Command)
}

// Dispatch runs one mode. version reports the build identity without state
// access; registered capability commands run under their own rules; reserved
// or unknown commands fail before any state directory or server is created.
func Dispatch(args []string, out io.Writer) error {
	if len(args) < 1 {
		return ErrMissingCommand
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(out, buildinfo.Get().Render())
		return nil
	case "serve", "mcp":
		return ErrReserved{Command: args[0]}
	default:
		if h, ok := commandHandlers[args[0]]; ok {
			return h(args[1:], out)
		}
		return ErrUnsupported{Command: args[0]}
	}
}
