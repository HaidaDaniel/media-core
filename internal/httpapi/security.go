package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
)

// CSRFHeader carries the per-session CSRF token on every authenticated
// mutation.
const CSRFHeader = "X-CSRF-Token"

// SessionCookieName is the HttpOnly session cookie name.
const SessionCookieName = "mc_session"

// MaxRequestBody is the request body limit: 1 MiB.
const MaxRequestBody = 1 << 20

// acceptedHosts are the Host header values this server accepts. In remote
// mode the single public origin host is the only valid host; in loopback
// mode the configured listener spelling and the bound listener address are
// accepted (they differ only for name-resolving hosts and port-0 test
// binds). An attacker-controlled host matches neither.
func (s *Server) acceptedHosts() []string {
	if s.cfg.PublicOrigin != "" {
		if u, err := url.Parse(s.cfg.PublicOrigin); err == nil && u.Host != "" {
			return []string{u.Host}
		}
	}
	hosts := []string{s.cfg.ListenAddr}
	s.mu.Lock()
	if s.ln != nil {
		hosts = append(hosts, s.ln.Addr().String())
	}
	s.mu.Unlock()
	return hosts
}

// acceptedOrigins are the Origin header values accepted on unsafe
// requests: the public origin in remote mode, otherwise the http origin of
// each accepted host.
func (s *Server) acceptedOrigins() []string {
	if s.cfg.PublicOrigin != "" {
		if u, err := url.Parse(s.cfg.PublicOrigin); err == nil && u.Scheme != "" && u.Host != "" {
			return []string{u.Scheme + "://" + u.Host}
		}
	}
	origins := make([]string, 0, len(s.acceptedHosts()))
	for _, host := range s.acceptedHosts() {
		origins = append(origins, "http://"+host)
	}
	return origins
}

// hostAllowed reports whether a Host header matches an accepted host.
func hostAllowed(provided string, accepted []string) bool {
	for _, a := range accepted {
		if provided == a {
			return true
		}
	}
	return false
}

// originAllowed reports whether an Origin header matches an accepted
// origin; a missing Origin is never allowed on an unsafe request.
func originAllowed(provided string, accepted []string) bool {
	return provided != "" && hostAllowed(provided, accepted)
}

// unsafeMethod reports whether the method is not side-effect free; these
// requests require an exact Origin and, once authenticated, a CSRF token.
func unsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// securityMiddleware is the transport security boundary applied to every
// route: security headers on every response, Host validation on every
// request, an exact Origin requirement on every unsafe request (a missing
// Origin never bypasses the check), and the 1 MiB request body limit.
func (s *Server) securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)
		if !hostAllowed(r.Host, s.acceptedHosts()) {
			writeError(w, http.StatusForbidden, app.NewAppError(app.ErrForbidden, "host not allowed"))
			return
		}
		if unsafeMethod(r.Method) {
			if !originAllowed(r.Header.Get("Origin"), s.acceptedOrigins()) {
				writeError(w, http.StatusForbidden, app.NewAppError(app.ErrForbidden, "origin not allowed"))
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBody)
		next.ServeHTTP(w, r)
	})
}

// setSecurityHeaders applies the response security headers: self-only CSP
// without inline or eval, nosniff, framing denied, and no referrer leakage.
// No CORS headers are ever sent.
func setSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Security-Policy", "default-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
}

// requireCSRF verifies the per-session CSRF token on an authenticated
// mutation; a missing or mismatched token is forbidden.
func requireCSRF(r *http.Request, sess *Session) *app.AppError {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get(CSRFHeader)), []byte(sess.CSRF)) != 1 {
		return app.NewAppError(app.ErrForbidden, "csrf token missing or invalid")
	}
	return nil
}

// decodeBody decodes one strict JSON document from the request body,
// rejecting unknown fields and bodies over the limit.
func decodeBody(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			return fmt.Errorf("request body exceeds the %d byte limit", MaxRequestBody)
		}
		return err
	}
	return nil
}

// writeError renders one failure envelope with the code's HTTP status.
func writeError(w http.ResponseWriter, status int, err *app.AppError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(app.Failure(err))
}

// writeSuccess renders one success envelope.
func writeSuccess(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(app.Success(data))
}

// cookieOptions builds the session cookie: HttpOnly, SameSite=Strict,
// Path=/, Secure when TLS is in use (local loopback HTTP is the sole
// exception), and bounded by the absolute session expiry.
func (s *Server) cookieOptions() cookieOptions {
	return cookieOptions{
		name:     SessionCookieName,
		secure:   s.secureCookie,
		maxAge:   int(time.Duration(DefaultAbsoluteExpiry).Seconds()),
		httpOnly: true,
	}
}

type cookieOptions struct {
	name     string
	secure   bool
	maxAge   int
	httpOnly bool
}

func (c cookieOptions) cookie(value string) *http.Cookie {
	return &http.Cookie{
		Name:     c.name,
		Value:    value,
		Path:     "/",
		HttpOnly: c.httpOnly,
		SameSite: http.SameSiteStrictMode,
		Secure:   c.secure,
		MaxAge:   c.maxAge,
	}
}
