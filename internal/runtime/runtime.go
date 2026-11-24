// Package runtime owns process lifecycle and mode dispatch for media-core.
package runtime

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/artwork"
	"github.com/HaidaDaniel/media-core/internal/buildinfo"
	"github.com/HaidaDaniel/media-core/internal/config"
	"github.com/HaidaDaniel/media-core/internal/httpapi"
	"github.com/HaidaDaniel/media-core/internal/mcp"
	"github.com/HaidaDaniel/media-core/internal/store"
	"github.com/HaidaDaniel/media-core/internal/webassets"
)

// mcpShutdownDrain bounds the Streamable HTTP listener's graceful drain;
// tests shorten it to exercise the deadline branch without a long wait.
var mcpShutdownDrain = 10 * time.Second

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
		return runMCP(args[1:], out)
	default:
		if h, ok := commandHandlers[args[0]]; ok {
			return h(args[1:], out)
		}
		return ErrUnsupported{Command: args[0]}
	}
}

// runServe runs the resident HTTP lifecycle. Configuration is resolved
// first: a non-loopback bind requires either explicit Trusted LAN mode on a
// single private interface with an optional local DNS host, or the complete
// remote HTTPS trio (allow remote, https public origin, TLS certificate and
// key). Invalid combinations fail before any listener is created. The state
// database is then opened under the shared
// lifetime maintenance lock, which enforces the schema boundary: a missing
// or drifted schema fails before any listener exists. The server serves
// until SIGINT or SIGTERM, stops accepting connections, drains in-flight
// requests for up to 10 seconds, closes the database, and returns. Sessions
// live in memory, so a restart invalidates them.
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
	db, err := OpenDatabase(cfg)
	if err != nil {
		return &ExitCodeError{Exit: 1, Err: err}
	}
	defer db.Close()
	srv.WithCatalog(app.NewService(db))
	if cfg.Artwork {
		cache, err := artwork.NewCache(cfg.StateDir, artwork.DefaultMaxCacheBytes, artwork.DefaultMaxImageBytes)
		if err != nil {
			return &ExitCodeError{Exit: 1, Err: err}
		}
		registry, client := artwork.NewDefault()
		srv.WithArtwork(artwork.NewService(db, registry, client, cache))
	}
	// Finalize the route table after the catalog is attached: the catch-all
	// then serves the embedded UI same-origin, so one listener carries both
	// the API and the SPA. WithAssets must follow WithCatalog because the
	// catalog routes are only known once the service is attached.
	srv.WithAssets(webassets.FS)
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
	const serveUsage = "usage: media-core serve [--state-dir <dir>] [--listen <addr>] [--trusted-lan [--lan-host <name>]] [--allow-remote --public-origin <url> --tls-cert <file> --tls-key <file>] [--artwork|--no-artwork]"
	var opts config.Options
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir", "--listen", "--public-origin", "--tls-cert", "--tls-key", "--lan-host":
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
			case "--lan-host":
				opts.LANHost = args[i]
			}
		case "--allow-remote":
			opts.AllowRemote = true
		case "--trusted-lan":
			opts.TrustedLAN = true
		case "--no-artwork":
			opts.Artwork = false
			opts.ArtworkSet = true
		case "--artwork":
			opts.Artwork = true
			opts.ArtworkSet = true
		default:
			return opts, fmt.Errorf("media-core serve: unknown argument %q (%s)", args[i], serveUsage)
		}
	}
	return opts, nil
}

// runMCP runs the MCP stdio lifecycle. Configuration is resolved first,
// so an invalid state dir fails before any protocol frame is written. The
// state database is then opened under the shared lifetime maintenance
// lock, which enforces the schema boundary: a missing or drifted schema
// fails before any frame is written and no serve process is needed. The
// protocol serves over stdio until the client closes stdin or the
// process receives SIGINT/SIGTERM; stdout carries protocol frames only
// and every diagnostic goes to stderr.
func runMCP(args []string, out io.Writer) error {
	// Exit codes per docs/application-contract.md: 2 invalid input, 1
	// config or runtime failure.
	opts, httpMode, err := parseMCPArgs(args)
	if err != nil {
		return &ExitCodeError{Exit: 2, Err: err}
	}
	cfg, err := ResolveConfig(opts)
	if err != nil {
		return &ExitCodeError{Exit: 1, Err: err}
	}
	db, err := OpenDatabase(cfg)
	if err != nil {
		return &ExitCodeError{Exit: 1, Err: err}
	}
	defer db.Close()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv := mcp.New(app.NewService(db), logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if !httpMode {
		if err := srv.Run(ctx, os.Stdin, os.Stdout); err != nil {
			return &ExitCodeError{Exit: 1, Err: err}
		}
		return nil
	}
	if err := serveMCPHTTP(ctx, cfg, srv); err != nil {
		return &ExitCodeError{Exit: 1, Err: err}
	}
	return nil
}

// serveMCPHTTP runs the opt-in Streamable HTTP transport: the same
// in-process service as stdio, authenticated by the state directory's
// owner secret, on the configured listener. It blocks until ctx is
// cancelled, then stops accepting connections and drains in-flight
// requests for up to 10 seconds.
func serveMCPHTTP(ctx context.Context, cfg *config.Config, srv *mcp.Server) error {
	handler := srv.StreamableHTTP(mcp.HTTPOptions{SecretReader: func() ([]byte, error) {
		secret, err := httpapi.ReadOwnerSecretHex(cfg.StateDir)
		if err != nil {
			if errors.Is(err, httpapi.ErrOwnerSecretMissing) {
				return nil, mcp.ErrSecretMissing
			}
			return nil, err
		}
		return []byte(secret), nil
	}})
	var ln net.Listener
	var err error
	if cfg.TLSCertFile != "" {
		cert, certErr := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if certErr != nil {
			return fmt.Errorf("mcp http: load TLS certificate: %w", certErr)
		}
		ln, err = tls.Listen("tcp", cfg.ListenAddr, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	} else {
		ln, err = net.Listen("tcp", cfg.ListenAddr)
	}
	if err != nil {
		return fmt.Errorf("mcp http: listen on %s: %w", cfg.ListenAddr, err)
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	httpServer := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.Serve(ln) }()
	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), mcpShutdownDrain)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		// Match serve: an exhausted drain force-closes and still exits
		// cleanly rather than reporting a failed shutdown.
		_ = httpServer.Close()
	}
	<-errCh
	return nil
}

// parseMCPArgs maps mcp flags onto explicit configuration options. The
// transport is stdio unless --http selects the opt-in Streamable HTTP
// listener; the listener flags and the remote trio then apply exactly as
// they do for serve.
func parseMCPArgs(args []string) (config.Options, bool, error) {
	const mcpUsage = "usage: media-core mcp [--state-dir <dir>] [--http] [--listen <addr>] [--allow-remote] [--public-origin <url>] [--tls-cert <file>] [--tls-key <file>]"
	var opts config.Options
	var httpMode bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--state-dir", "--listen", "--public-origin", "--tls-cert", "--tls-key":
			if i+1 >= len(args) {
				return opts, httpMode, fmt.Errorf("media-core mcp: %s requires a value (%s)", args[i], mcpUsage)
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
		case "--http":
			httpMode = true
		default:
			return opts, httpMode, fmt.Errorf("media-core mcp: unknown argument %q (%s)", args[i], mcpUsage)
		}
	}
	return opts, httpMode, nil
}
