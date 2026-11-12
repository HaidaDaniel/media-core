// Package httpapi owns the loopback HTTP server, its security boundary and
// the single-owner session. It is a transport adapter: it calls the shared
// app services in-process and never another adapter's port. For the current
// milestone it exposes only the /api/v1/session endpoints; library routes
// arrive later and are all authenticated through this same boundary.
package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// OwnerSecretFileName is the fixed owner-secret file name inside the state
// directory. It holds 32 raw secret bytes and uses mode 0600.
const OwnerSecretFileName = "owner-secret"

// ErrOwnerSecretMissing is returned when the owner secret file is absent, so
// a login attempt can be distinguished from a wrong secret.
var ErrOwnerSecretMissing = errors.New("httpapi: owner secret is not configured; run media-core owner generate")

// OwnerSecretPath returns the absolute owner-secret file path for a state
// directory.
func OwnerSecretPath(stateDir string) string {
	return filepath.Join(stateDir, OwnerSecretFileName)
}

// GenerateOwnerSecret writes a fresh 256-bit owner secret to the state
// directory when the file is absent and returns its hex encoding. An
// existing file is never overwritten, so generation is idempotent and the
// secret is stable across restarts.
func GenerateOwnerSecret(stateDir string) (string, error) {
	path := OwnerSecretPath(stateDir)
	if _, err := os.Stat(path); err == nil {
		return ReadOwnerSecretHex(stateDir)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("httpapi: stat owner secret: %w", err)
	}
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("httpapi: generate owner secret: %w", err)
	}
	if err := os.WriteFile(path, buf[:], 0o600); err != nil {
		return "", fmt.Errorf("httpapi: write owner secret: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

// ReadOwnerSecretHex returns the hex-encoded owner secret for a state
// directory. A missing file reports ErrOwnerSecretMissing.
func ReadOwnerSecretHex(stateDir string) (string, error) {
	raw, err := os.ReadFile(OwnerSecretPath(stateDir))
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrOwnerSecretMissing
		}
		return "", fmt.Errorf("httpapi: read owner secret: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// Session is one live single-owner session: an opaque id, its per-session
// CSRF token, and the expiry bounds. Sessions live in memory only, so a
// process restart invalidates every session.
type Session struct {
	ID         string
	CSRF       string
	Created    time.Time
	LastActive time.Time
}

// idleExpiry and absoluteExpiry are the session bounds: an idle session
// expires after 30 minutes without activity, and every session expires 12
// hours after creation regardless of activity.
const (
	DefaultIdleExpiry     = 30 * time.Minute
	DefaultAbsoluteExpiry = 12 * time.Hour
)

// RateLimiter bounds login attempts to a fixed window: at most maxAttempts
// within window. It is a simple in-memory sliding window, sufficient for a
// single-owner loopback server.
type RateLimiter struct {
	mu          sync.Mutex
	maxAttempts int
	window      time.Duration
	attempts    []time.Time
	now         func() time.Time
}

// NewRateLimiter builds a limiter; a zero maxAttempts or window falls back
// to a permissive default so misconfiguration cannot lock the owner out
// silently.
func NewRateLimiter(maxAttempts int, window time.Duration, now func() time.Time) *RateLimiter {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if window <= 0 {
		window = 15 * time.Second
	}
	if now == nil {
		now = time.Now
	}
	return &RateLimiter{maxAttempts: maxAttempts, window: window, now: now}
}

// Allow reports whether one more attempt is within the bound, recording it
// when true.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	t := rl.now()
	cutoff := t.Add(-rl.window)
	i := 0
	for i < len(rl.attempts) && rl.attempts[i].Before(cutoff) {
		i++
	}
	rl.attempts = rl.attempts[i:]
	if len(rl.attempts) >= rl.maxAttempts {
		return false
	}
	rl.attempts = append(rl.attempts, t)
	return true
}

// SessionStore is the in-memory single-owner session table. It holds at most
// one live session: a new login rotates the session, invalidating the
// previous one.
type SessionStore struct {
	mu        sync.RWMutex
	sessions  map[string]*Session
	idle      time.Duration
	absolute  time.Duration
	limiter   *RateLimiter
	secret    func() ([]byte, error)
	now       func() time.Time
	randToken func() (string, error)
}

// NewSessionStore builds a store over the owner-secret reader. The secret
// reader is called on each login so a generated secret takes effect without
// a restart.
func NewSessionStore(secret func() ([]byte, error)) *SessionStore {
	return &SessionStore{
		sessions:  map[string]*Session{},
		idle:      DefaultIdleExpiry,
		absolute:  DefaultAbsoluteExpiry,
		limiter:   NewRateLimiter(5, 15*time.Second, time.Now),
		secret:    secret,
		now:       time.Now,
		randToken: randomToken,
	}
}

// WithClock overrides the clock for tests.
func (s *SessionStore) WithClock(now func() time.Time) *SessionStore {
	s.now = now
	s.limiter = NewRateLimiter(s.limiter.maxAttempts, s.limiter.window, now)
	return s
}

// WithRateLimit overrides the login rate bound for tests.
func (s *SessionStore) WithRateLimit(maxAttempts int, window time.Duration) *SessionStore {
	s.limiter = NewRateLimiter(maxAttempts, window, s.now)
	return s
}

// randomToken returns a 32-byte random token as a URL-safe hex string.
func randomToken() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// Login exchanges the owner secret for a fresh session. It rate-limits
// attempts, compares the secret in constant time, and on success rotates the
// single session (invalidating any previous one). A missing secret file is
// distinguished from a wrong secret.
func (s *SessionStore) Login(provided string) (*Session, error) {
	if !s.limiter.Allow() {
		return nil, ErrRateLimited
	}
	stored, err := s.secret()
	if err != nil {
		return nil, err
	}
	providedBytes := []byte(provided)
	if subtle.ConstantTimeCompare(providedBytes, stored) != 1 {
		return nil, ErrBadSecret
	}
	id, err := s.randToken()
	if err != nil {
		return nil, err
	}
	csrf, err := s.randToken()
	if err != nil {
		return nil, err
	}
	now := s.now()
	sess := &Session{ID: id, CSRF: csrf, Created: now, LastActive: now}
	s.mu.Lock()
	s.sessions = map[string]*Session{id: sess}
	s.mu.Unlock()
	return sess, nil
}

// Lookup returns the live session for an id, refreshing its idle timestamp.
// Expired (idle or absolute) sessions are dropped and reported absent.
func (s *SessionStore) Lookup(id string) (*Session, bool) {
	if id == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, false
	}
	now := s.now()
	if now.After(sess.Created.Add(s.absolute)) || now.After(sess.LastActive.Add(s.idle)) {
		delete(s.sessions, id)
		return nil, false
	}
	sess.LastActive = now
	return sess, true
}

// Logout invalidates the session for an id.
func (s *SessionStore) Logout(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

// ErrBadSecret is returned when the provided owner secret does not match.
var ErrBadSecret = errors.New("httpapi: owner secret does not match")

// ErrRateLimited is returned when login attempts exceed the bound.
var ErrRateLimited = errors.New("httpapi: too many login attempts; slow down")
