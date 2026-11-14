package mcp

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/buildinfo"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server adapts the MCP protocol to the shared service. It owns the
// protocol lifecycle and the tool registration seam; the tools themselves
// are domain capabilities that later tasks register through AddTool, each
// calling the shared service and rendering outcomes with Result and
// Failure.
type Server struct {
	service *app.Service
	srv     *mcpsdk.Server
}

// New builds the MCP server over the shared service. The logger receives
// every protocol diagnostic and must write to a stream that is not the
// protocol output, which carries frames only.
func New(service *app.Service, logger *slog.Logger) *Server {
	impl := &mcpsdk.Implementation{
		Name:        "media-core",
		Title:       "media-core",
		Description: "Personal media library over MCP stdio. Domain reads and personal corrections through the shared service; no generic SQL, shell or file tools.",
		Version:     buildinfo.Get().Version,
	}
	opts := &mcpsdk.ServerOptions{Logger: logger}
	if logger == nil {
		opts.Logger = slog.Default()
	}
	return &Server{service: service, srv: mcpsdk.NewServer(impl, opts)}
}

// Service exposes the shared service for tool registration.
func (s *Server) Service() *app.Service { return s.service }

// AddTool registers a typed tool handler on the server. It is the only
// seam through which domain capabilities reach the protocol: the handler
// calls the shared service and renders the outcome with Result or
// Failure.
func AddTool[In, Out any](s *Server, t *mcpsdk.Tool, h mcpsdk.ToolHandlerFor[In, Out]) {
	mcpsdk.AddTool(s.srv, t, h)
}

// Run serves the MCP protocol over in and out until the client closes in,
// the context is canceled, or the protocol fails. in and out are the
// process stdio streams; a clean client EOF ends the session without
// error.
func (s *Server) Run(ctx context.Context, in io.ReadCloser, out io.WriteCloser) error {
	err := s.srv.Run(ctx, &mcpsdk.IOTransport{Reader: in, Writer: out})
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
