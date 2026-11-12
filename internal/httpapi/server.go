package httpapi

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/config"
)

// drainTimeout bounds how long graceful shutdown waits for in-flight
// requests before giving up.
const drainTimeout = 10 * time.Second

// Server is the media-core HTTP server: lifecycle, routes and the security
// boundary for the single-owner session. Only the /api/v1/session endpoints
// exist in this milestone; every other route is a JSON 404, so there is no
// unauthenticated writable endpoint.
type Server struct {
	cfg          *config.Config
	sessions     *SessionStore
	mux          *http.ServeMux
	tlsConfig    *tls.Config
	secureCookie bool
	httpServer   *http.Server

	mu sync.Mutex
	ln net.Listener
}

// New builds the server for a resolved configuration. A non-loopback
// listener requires the explicit remote trio (allow-remote, https public
// origin, TLS certificate and key) and a loadable certificate; this fails
// before any listener is created.
func New(cfg *config.Config) (*Server, error) {
	if cfg == nil || cfg.StateDir == "" || cfg.ListenAddr == "" {
		return nil, fmt.Errorf("httpapi: configuration is missing state dir or listen addr")
	}
	var tlsCfg *tls.Config
	secure := false
	if !isLoopback(cfg.ListenAddr) {
		if !cfg.AllowRemote {
			return nil, fmt.Errorf("httpapi: listener %s is not loopback; explicit allow-remote is required", cfg.ListenAddr)
		}
		if cfg.PublicOrigin == "" {
			return nil, fmt.Errorf("httpapi: listener %s requires a https public origin", cfg.ListenAddr)
		}
		if cfg.TLSCertFile == "" || cfg.TLSKeyFile == "" {
			return nil, fmt.Errorf("httpapi: listener %s requires TLS certificate and key files", cfg.ListenAddr)
		}
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("httpapi: load TLS certificate: %w", err)
		}
		tlsCfg = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
		secure = true
	}
	s := &Server{
		cfg:          cfg,
		sessions:     NewSessionStore(secretReader(cfg.StateDir)),
		tlsConfig:    tlsCfg,
		secureCookie: secure,
	}
	s.buildMux()
	return s, nil
}

// secretReader reads the owner secret as hex on each login, so a generated
// secret takes effect without a restart and a missing file is
// distinguishable from a wrong secret.
func secretReader(stateDir string) func() ([]byte, error) {
	return func() ([]byte, error) {
		secret, err := ReadOwnerSecretHex(stateDir)
		return []byte(secret), err
	}
}

// buildMux wires the routes behind the security boundary.
func (s *Server) buildMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/api/v1/session", s.securityMiddleware(http.HandlerFunc(s.sessionHandler)))
	mux.Handle("/", s.securityMiddleware(http.HandlerFunc(s.notFoundHandler)))
	s.mux = mux
	return mux
}

// isLoopback reports whether a listener address binds only to loopback.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Serve binds the configured listener and serves until ctx is canceled. On
// cancellation it stops accepting connections, drains in-flight requests
// for up to drainTimeout, and then returns. It is the resident serve
// lifecycle; a process restart invalidates every session because sessions
// live in memory only.
func (s *Server) Serve(ctx context.Context) error {
	var ln net.Listener
	var err error
	if s.tlsConfig != nil {
		ln, err = tls.Listen("tcp", s.cfg.ListenAddr, s.tlsConfig)
	} else {
		ln, err = net.Listen("tcp", s.cfg.ListenAddr)
	}
	if err != nil {
		return fmt.Errorf("httpapi: listen %s: %w", s.cfg.ListenAddr, err)
	}
	// The http server is assigned before the listener is published so
	// readers that observe the bound address also see the configured
	// timeouts.
	s.httpServer = &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	s.mu.Lock()
	s.ln = ln
	s.mu.Unlock()
	errCh := make(chan error, 1)
	go func() { errCh <- s.httpServer.Serve(ln) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), drainTimeout)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
		<-errCh
		return nil
	case err := <-errCh:
		return err
	}
}

// Addr returns the bound listener address; nil before Serve has bound. It
// is the way tests discover a server started on port 0.
func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return nil
	}
	return s.ln.Addr()
}

// loginRequest is the owner-secret exchange body for POST /api/v1/session.
type loginRequest struct {
	OwnerSecret string `json:"owner_secret"`
}

// loginPayload is the session data returned on login and inspection: the
// per-session CSRF token, which the frontend holds in memory only.
type loginPayload struct {
	CSRFToken string `json:"csrf_token"`
}

// sessionHandler dispatches the session routes; unsafe methods are already
// Origin-checked by the security boundary.
func (s *Server) sessionHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleLogin(w, r)
	case http.MethodGet:
		s.handleSessionGet(w, r)
	case http.MethodDelete:
		s.handleLogout(w, r)
	default:
		writeError(w, http.StatusNotFound, app.NewAppError(app.ErrNotFound, "no such route"))
	}
}

// handleLogin exchanges the owner secret for a fresh single-owner session.
// A missing owner secret and a wrong secret are distinguished from each
// other and from transport failures, and login attempts are rate limited.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, app.NewAppError(app.ErrInvalidArgument, "invalid login request: "+err.Error()))
		return
	}
	if req.OwnerSecret == "" {
		writeError(w, http.StatusBadRequest, app.NewAppError(app.ErrInvalidArgument, "owner_secret is required"))
		return
	}
	sess, err := s.sessions.Login(req.OwnerSecret)
	switch {
	case err == nil:
		w.Header().Add("Set-Cookie", s.cookieOptions().cookie(sess.ID).String())
		writeSuccess(w, loginPayload{CSRFToken: sess.CSRF})
	case errors.Is(err, ErrOwnerSecretMissing):
		writeError(w, http.StatusUnauthorized, app.NewAppError(app.ErrUnauthorized, "owner secret is not configured; run media-core owner generate"))
	case errors.Is(err, ErrBadSecret):
		writeError(w, http.StatusUnauthorized, app.NewAppError(app.ErrUnauthorized, "owner secret does not match"))
	case errors.Is(err, ErrRateLimited):
		writeError(w, http.StatusTooManyRequests, app.NewAppError(app.ErrRateLimited, "too many login attempts; slow down"))
	default:
		writeError(w, http.StatusInternalServerError, app.NewAppError(app.ErrInternalError, "login failed"))
	}
}

// handleSessionGet inspects the current session so the frontend can refresh
// its in-memory CSRF token.
func (s *Server) handleSessionGet(w http.ResponseWriter, r *http.Request) {
	sess, appErr := s.currentSession(r)
	if appErr != nil {
		writeError(w, appErr.Code.HTTPStatus(), appErr)
		return
	}
	writeSuccess(w, loginPayload{CSRFToken: sess.CSRF})
}

// handleLogout invalidates the current session and clears the cookie. As an
// authenticated mutation it requires the per-session CSRF token.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sess, appErr := s.currentSession(r)
	if appErr != nil {
		writeError(w, appErr.Code.HTTPStatus(), appErr)
		return
	}
	if csrfErr := requireCSRF(r, sess); csrfErr != nil {
		writeError(w, http.StatusForbidden, csrfErr)
		return
	}
	s.sessions.Logout(sess.ID)
	c := s.cookieOptions()
	c.maxAge = -1
	w.Header().Add("Set-Cookie", c.cookie("").String())
	writeSuccess(w, nil)
}

// currentSession resolves the live session from the cookie; absent or
// expired sessions are unauthorized.
func (s *Server) currentSession(r *http.Request) (*Session, *app.AppError) {
	c, err := r.Cookie(SessionCookieName)
	if err != nil || c.Value == "" {
		return nil, app.NewAppError(app.ErrUnauthorized, "not authenticated")
	}
	sess, ok := s.sessions.Lookup(c.Value)
	if !ok {
		return nil, app.NewAppError(app.ErrUnauthorized, "session is not active")
	}
	return sess, nil
}

// notFoundHandler is the catch-all: every route outside the session boundary
// is a JSON 404, which is what keeps the server free of unauthenticated
// writable endpoints.
func (s *Server) notFoundHandler(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, app.NewAppError(app.ErrNotFound, "no such route"))
}
