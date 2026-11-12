package httpapi

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestSessionOwnerSecretFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadOwnerSecretHex(dir); !errors.Is(err, ErrOwnerSecretMissing) {
		t.Fatalf("read missing secret: err = %v, want ErrOwnerSecretMissing", err)
	}
	secret, err := GenerateOwnerSecret(dir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(secret) != 64 {
		t.Fatalf("secret = %q, want 64 hex chars (256 bits)", secret)
	}
	info, err := os.Stat(OwnerSecretPath(dir))
	if err != nil {
		t.Fatalf("stat owner secret: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("owner secret mode = %o, want 0600", perm)
	}
	// Generation is idempotent: an existing secret is never replaced.
	again, err := GenerateOwnerSecret(dir)
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if again != secret {
		t.Fatalf("regenerate replaced the secret: %q != %q", again, secret)
	}
	got, err := ReadOwnerSecretHex(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != secret {
		t.Fatalf("read = %q, want %q", got, secret)
	}
}

func TestSessionLogin(t *testing.T) {
	dir := t.TempDir()
	secret, err := GenerateOwnerSecret(dir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// A missing secret file is distinguishable from a wrong secret.
	empty := NewSessionStore(secretReader(t.TempDir()))
	if _, err := empty.Login(secret); !errors.Is(err, ErrOwnerSecretMissing) {
		t.Fatalf("login with no secret file: err = %v, want ErrOwnerSecretMissing", err)
	}
	store := NewSessionStore(secretReader(dir))
	if _, err := store.Login("0000000000000000000000000000000000000000000000000000000000000000"); !errors.Is(err, ErrBadSecret) {
		t.Fatalf("login with wrong secret: err = %v, want ErrBadSecret", err)
	}
	sess, err := store.Login(secret)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sess.ID == "" || sess.CSRF == "" {
		t.Fatalf("session id or csrf empty: %+v", sess)
	}
	if sess.ID == sess.CSRF {
		t.Fatal("session id and csrf token must differ")
	}
	if _, ok := store.Lookup(sess.ID); !ok {
		t.Fatal("fresh session not live")
	}
	store.Logout(sess.ID)
	if _, ok := store.Lookup(sess.ID); ok {
		t.Fatal("session still live after logout")
	}
	if _, ok := store.Lookup(""); ok {
		t.Fatal("empty session id must never look up")
	}
}

func TestSessionRotation(t *testing.T) {
	dir := t.TempDir()
	secret, err := GenerateOwnerSecret(dir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	store := NewSessionStore(secretReader(dir))
	first, err := store.Login(secret)
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	second, err := store.Login(secret)
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("rotation must issue a new session id")
	}
	if _, ok := store.Lookup(first.ID); ok {
		t.Fatal("previous session still live after rotation")
	}
	if _, ok := store.Lookup(second.ID); !ok {
		t.Fatal("rotated session not live")
	}
}

func TestSessionExpiry(t *testing.T) {
	dir := t.TempDir()
	secret, err := GenerateOwnerSecret(dir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	base := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	now := base
	store := NewSessionStore(secretReader(dir)).WithClock(func() time.Time { return now })

	sess, err := store.Login(secret)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	now = base.Add(29 * time.Minute)
	if _, ok := store.Lookup(sess.ID); !ok {
		t.Fatal("session not live 29 minutes after login")
	}
	now = base.Add(29*time.Minute + 30*time.Minute + time.Second)
	if _, ok := store.Lookup(sess.ID); ok {
		t.Fatalf("session still live %v after its last activity", 30*time.Minute+time.Second)
	}

	// Absolute expiry: activity within the idle window cannot extend a
	// session past 12 hours.
	now = base
	sess, err = store.Login(secret)
	if err != nil {
		t.Fatalf("re-login: %v", err)
	}
	for m := 30; m <= 11*60+30; m += 30 {
		now = base.Add(time.Duration(m) * time.Minute)
		if _, ok := store.Lookup(sess.ID); !ok {
			t.Fatalf("session expired %d minutes into its absolute life", m)
		}
	}
	now = base.Add(11*time.Hour + 45*time.Minute)
	if _, ok := store.Lookup(sess.ID); !ok {
		t.Fatal("session expired at 11h45m into its absolute life")
	}
	// 12h10m is within 30 minutes of the last activity (11h45m) but past
	// the 12h absolute bound: only the absolute expiry may fire.
	now = base.Add(12*time.Hour + 10*time.Minute)
	if _, ok := store.Lookup(sess.ID); ok {
		t.Fatal("session still live past the absolute expiry despite recent activity")
	}
}

func TestSessionLoginRateLimit(t *testing.T) {
	dir := t.TempDir()
	secret, err := GenerateOwnerSecret(dir)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	base := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	now := base
	store := NewSessionStore(secretReader(dir)).WithRateLimit(3, 15*time.Minute).WithClock(func() time.Time { return now })
	for i := 1; i <= 3; i++ {
		if _, err := store.Login(secret); err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, err := store.Login(secret); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("fourth attempt in window: err = %v, want ErrRateLimited", err)
	}
	// Wrong attempts count against the same bound.
	if _, err := store.Login("0000000000000000000000000000000000000000000000000000000000000000"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("fifth attempt: err = %v, want ErrRateLimited", err)
	}
	now = base.Add(16 * time.Minute)
	if _, err := store.Login(secret); err != nil {
		t.Fatalf("login after the window passed: %v", err)
	}
}
