package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolatedEnv returns a test environment whose HOME points at a fresh
// temporary directory so defaults never touch the real user state.
func isolatedEnv(t *testing.T, vars map[string]string) map[string]string {
	t.Helper()
	env := map[string]string{"HOME": t.TempDir()}
	for k, v := range vars {
		env[k] = v
	}
	return env
}

func mustResolve(t *testing.T, opts Options) *Config {
	t.Helper()
	cfg, err := Resolve(opts)
	if err != nil {
		t.Fatalf("Resolve(%+v) error: %v", opts, err)
	}
	return cfg
}

func TestFlagOverridesEnvAndDefault(t *testing.T) {
	flag := filepath.Join(t.TempDir(), "explicit")
	envState := filepath.Join(t.TempDir(), "env")
	env := isolatedEnv(t, map[string]string{"MEDIA_CORE_STATE_DIR": envState})

	if cfg := mustResolve(t, Options{StateDir: flag, Env: env}); cfg.StateDir != flag {
		t.Errorf("flag precedence: StateDir = %q, want %q", cfg.StateDir, flag)
	}
	if cfg := mustResolve(t, Options{Env: env}); cfg.StateDir != envState {
		t.Errorf("env precedence: StateDir = %q, want %q", cfg.StateDir, envState)
	}
}

func TestXDGDefault(t *testing.T) {
	xdg := filepath.Join(t.TempDir(), "xdg")
	cfg := mustResolve(t, Options{Env: isolatedEnv(t, map[string]string{"XDG_DATA_HOME": xdg})})
	if want := filepath.Join(xdg, "media-core"); cfg.StateDir != want {
		t.Errorf("XDG default: StateDir = %q, want %q", cfg.StateDir, want)
	}
}

func TestRelativeXDGDefaultResolvesAgainstHome(t *testing.T) {
	home := t.TempDir()
	cfg := mustResolve(t, Options{Env: map[string]string{"HOME": home, "XDG_DATA_HOME": "rel/data"}})
	if want := filepath.Join(home, "rel", "data", "media-core"); cfg.StateDir != want {
		t.Errorf("relative XDG default: StateDir = %q, want %q", cfg.StateDir, want)
	}
}

func TestHomeDefault(t *testing.T) {
	home := t.TempDir()
	cfg := mustResolve(t, Options{Env: map[string]string{"HOME": home}})
	if want := filepath.Join(home, ".local", "share", "media-core"); cfg.StateDir != want {
		t.Errorf("HOME default: StateDir = %q, want %q", cfg.StateDir, want)
	}
}

func TestNoHomeNoXDGRejected(t *testing.T) {
	if _, err := Resolve(Options{Env: map[string]string{}}); err == nil {
		t.Error("Resolve with no HOME/XDG/flag: want error, got nil")
	}
}

func TestRelativeStateDirRejected(t *testing.T) {
	_, err := Resolve(Options{StateDir: "relative/state", Env: isolatedEnv(t, nil)})
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("relative state dir: err = %v, want absolute-path rejection", err)
	}
}

func TestSymlinkedStateDirRejected(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if _, err := Resolve(Options{StateDir: link, Env: isolatedEnv(t, nil)}); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("symlinked state dir: err = %v, want symlink rejection", err)
	}
}

func TestStateDirNotADirectoryRejected(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := Resolve(Options{StateDir: file, Env: isolatedEnv(t, nil)}); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("file state dir: err = %v, want not-a-directory rejection", err)
	}
}

func TestResolveCreatesNoState(t *testing.T) {
	target := filepath.Join(t.TempDir(), "missing", "nested")
	mustResolve(t, Options{StateDir: target, Env: isolatedEnv(t, nil)})
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Errorf("Resolve created %q; want no side effects", target)
	}
}

func TestExistingRealStateDirAccepted(t *testing.T) {
	dir := t.TempDir()
	cfg := mustResolve(t, Options{StateDir: dir, Env: isolatedEnv(t, nil)})
	if want, _ := filepath.Abs(dir); cfg.StateDir != want {
		t.Errorf("StateDir = %q, want canonical %q", cfg.StateDir, want)
	}
}

func TestDefaultIsNeverCheckoutRelative(t *testing.T) {
	home := t.TempDir()
	checkout := t.TempDir()
	if err := os.Chdir(checkout); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	cfg := mustResolve(t, Options{Env: map[string]string{"HOME": home}})
	if !strings.HasPrefix(cfg.StateDir, home) {
		t.Errorf("default StateDir = %q, want it under HOME %q, not the checkout %q", cfg.StateDir, home, checkout)
	}
}

func TestListenerDefaultIsLoopback(t *testing.T) {
	cfg := mustResolve(t, Options{Env: isolatedEnv(t, nil)})
	if cfg.ListenAddr != DefaultListenAddr {
		t.Errorf("default listener = %q, want %q", cfg.ListenAddr, DefaultListenAddr)
	}
}

func TestListenerFlagOverride(t *testing.T) {
	cfg := mustResolve(t, Options{ListenAddr: "127.0.0.1:9000", Env: isolatedEnv(t, nil)})
	if cfg.ListenAddr != "127.0.0.1:9000" {
		t.Errorf("listener = %q, want 127.0.0.1:9000", cfg.ListenAddr)
	}
}

func TestListenerInvalid(t *testing.T) {
	for _, addr := range []string{"127.0.0.1", "127.0.0.1:0", "127.0.0.1:65536", "127.0.0.1:notaport", ":8787"} {
		if _, err := Resolve(Options{ListenAddr: addr, Env: isolatedEnv(t, nil)}); err == nil {
			t.Errorf("listener %q: want error, got nil", addr)
		}
	}
}

func TestNonLoopbackRequiresExplicitRemoteTrio(t *testing.T) {
	base := Options{ListenAddr: "192.168.0.94:8787", Env: isolatedEnv(t, nil)}

	if _, err := Resolve(base); err == nil || !strings.Contains(err.Error(), "allow-remote") {
		t.Errorf("non-loopback without allow-remote: err = %v, want allow-remote rejection", err)
	}
	base.AllowRemote = true
	if _, err := Resolve(base); err == nil || !strings.Contains(err.Error(), "public origin") {
		t.Errorf("non-loopback without origin: err = %v, want public origin rejection", err)
	}
	base.PublicOrigin = "https://media.example.org"
	if _, err := Resolve(base); err == nil || !strings.Contains(err.Error(), "TLS") {
		t.Errorf("non-loopback without TLS: err = %v, want TLS rejection", err)
	}
	base.TLSCertFile = "/tls/cert.pem"
	base.TLSKeyFile = "/tls/key.pem"
	cfg := mustResolve(t, base)
	if !cfg.AllowRemote || cfg.PublicOrigin != "https://media.example.org" {
		t.Errorf("complete trio: cfg = %+v, want remote values retained", cfg)
	}
}

func TestLoopbackNamesDoNotRequireTrio(t *testing.T) {
	for _, addr := range []string{"localhost:8787", "[::1]:8787", "127.5.5.5:8787"} {
		cfg := mustResolve(t, Options{ListenAddr: addr, Env: isolatedEnv(t, nil)})
		if cfg.AllowRemote {
			t.Errorf("listener %q: AllowRemote = true, want false", addr)
		}
	}
}

func TestPublicOriginValidation(t *testing.T) {
	for _, origin := range []string{"http://media.example.org", "https://", "https://user:pass@media.example.org", "not a url"} {
		if _, err := Resolve(Options{PublicOrigin: origin, Env: isolatedEnv(t, nil)}); err == nil {
			t.Errorf("public origin %q: want error, got nil", origin)
		}
	}
	cfg := mustResolve(t, Options{PublicOrigin: "https://media.example.org", Env: isolatedEnv(t, nil)})
	if cfg.PublicOrigin != "https://media.example.org" {
		t.Errorf("public origin = %q, want it retained", cfg.PublicOrigin)
	}
}

func TestDiagnosticsRedactSecret(t *testing.T) {
	cfg := mustResolve(t, Options{Env: isolatedEnv(t, nil)})
	cfg.OwnerSecret = Secret("hunter2-super-secret-value")
	d := cfg.Diagnostics()
	if strings.Contains(d, "hunter2-super-secret-value") {
		t.Errorf("Diagnostics() %q exposes the owner secret", d)
	}
	if !strings.Contains(d, "ownerSecret=redacted") {
		t.Errorf("Diagnostics() %q does not show redacted secret", d)
	}
}

func TestDiagnosticsUnsetSecret(t *testing.T) {
	cfg := mustResolve(t, Options{Env: isolatedEnv(t, nil)})
	d := cfg.Diagnostics()
	if !strings.Contains(d, "ownerSecret=<unset>") {
		t.Errorf("Diagnostics() %q does not show unset secret", d)
	}
	if !strings.Contains(d, "stateDir="+cfg.StateDir) || !strings.Contains(d, "listen="+cfg.ListenAddr) {
		t.Errorf("Diagnostics() %q does not show state dir and listener", d)
	}
}
