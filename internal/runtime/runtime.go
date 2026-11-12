// Package runtime owns process lifecycle and mode dispatch for media-core.
package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/HaidaDaniel/media-core/internal/buildinfo"
	"github.com/HaidaDaniel/media-core/internal/config"
	"github.com/HaidaDaniel/media-core/internal/httpapi"
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
// access; serve runs the resident HTTP lifecycle; registered capability
// commands run under their own rules; reserved or unknown commands fail
// before any state directory or server is created.
func Dispatch(args []string, out io.Writer) error {
	if len(args) < 1 {
		return ErrMissingCommand
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(out, buildinfo.Get().Render())
		return nil
	case "serve":
		return runServe(args[1:], out)
	case "mcp":
		return ErrReserved{Command: "mcp"}
	default:
		if h, ok := commandHandlers[args[0]]; ok {
			return h(args[1:], out)
		}
		return ErrUnsupported{Command: args[0]}
	}
}

// runServe runs the resident HTTP lifecycle. Configuration is resolved
// first, so a non-loopback bind without the explicit remote trio (allow
// remote, https public origin, TLS certificate and key) fails before any
// listener is created. The server then serves until SIGINT or SIGTERM,
// stops accepting connections, drains in-flight requests for up to 10
// seconds, and returns. This milestone opens no database: the session
// boundary is transport-only and sessions live in memory, so a restart
// invalidates them.
func runServe(args []string, out io.Writer) error {
	// Exit codes per docs/application-contract.md: 2 invalid input, 1
	// config or runtime failure.
	opts, err := parseServeArgs(args)
	if err != nil {
		return &ExitCodeError{Exit: 2, Err: err}
	}
	cfg, err := ResolveConfig(opts)
	if err != nil {
		return &ExitCodeError{Exit: 1, Err: err}
	}
	srv, err := httpapi.New(cfg)
	if err != nil {
		return &ExitCodeError{Exit: 1, Err: err}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.Serve(ctx); err != nil {
		return &ExitCodeError{Exit: 1, Err: err}
	}
	fmt.Fprintln(out, "serve: stopped")
	return nil
}

// parseServeArgs maps serve flags onto explicit configuration options; the
// loopback default and the remote trio come from config resolution.
func parseServeArgs(args []string) (config.Options, error) {
	const serveUsage = "usage: media-core serve [--state-dir <dir>] [--listen <addr>] [--allow-remote] [--public-origin <url>] [--tls-cert <file>] [--tls-key <file>]"
	var opts config.Options
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir", "--listen", "--public-origin", "--tls-cert", "--tls-key":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("media-core serve: %s requires a value", args[i])
			}
			i++
			switch args[i-1] {
			case "--state-dir":
				opts.StateDir = args[i]
			case "--listen":
				opts.ListenAddr = args[i]
			case "--public-origin":
				opts.PublicOrigin = args[i]
			case "--tls-cert":
				opts.TLSCertFile = args[i]
			case "--tls-key":
				opts.TLSKeyFile = args[i]
			}
		case "--allow-remote":
			opts.AllowRemote = true
		default:
			return opts, fmt.Errorf("media-core serve: unknown argument %q (%s)", args[i], serveUsage)
		}
	}
	return opts, nil
}
