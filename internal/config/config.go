// Package config resolves explicit media-core configuration.
//
// Precedence: explicit flags override environment, which overrides
// defaults. Resolution is pure: it never creates state directories,
// binds listeners, or renders secret values.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultListenAddr is the loopback listener used when none is provided.
const DefaultListenAddr = "127.0.0.1:8787"

// Secret wraps a value that must never appear in diagnostics.
type Secret string

// String renders the secret for diagnostics without exposing the value.
func (s Secret) String() string {
	if s == "" {
		return "<unset>"
	}
	return "redacted"
}

// Options carries explicit (flag) values and, for tests, a fixed
// environment. A nil Env reads the real process environment.
type Options struct {
	StateDir     string
	ListenAddr   string
	PublicOrigin string
	AllowRemote  bool
	TLSCertFile  string
	TLSKeyFile   string
	TrustedLAN   bool
	LANHost      string
	Env          map[string]string
}

// lookup returns the environment value for key, preferring the test
// environment when present.
func (o Options) lookup(key string) string {
	if o.Env != nil {
		return o.Env[key]
	}
	return os.Getenv(key)
}

// Config is the resolved, validated configuration.
type Config struct {
	StateDir     string
	ListenAddr   string
	PublicOrigin string
	AllowRemote  bool
	TLSCertFile  string
	TLSKeyFile   string
	TrustedLAN   bool
	LANHost      string
	OwnerSecret  Secret
}

// Resolve computes the configuration from explicit values, the
// environment, and defaults. It performs no filesystem writes.
func Resolve(opts Options) (*Config, error) {
	cfg := &Config{
		AllowRemote: opts.AllowRemote,
		TLSCertFile: opts.TLSCertFile,
		TLSKeyFile:  opts.TLSKeyFile,
		TrustedLAN:  opts.TrustedLAN,
		LANHost:     strings.TrimSpace(opts.LANHost),
	}
	if err := cfg.resolveStateDir(opts.StateDir, opts.lookup); err != nil {
		return nil, err
	}
	if err := cfg.resolveListener(opts.ListenAddr); err != nil {
		return nil, err
	}
	if err := cfg.resolvePublicOrigin(opts.PublicOrigin); err != nil {
		return nil, err
	}
	if cfg.TrustedLAN {
		if err := cfg.validateTrustedLAN(); err != nil {
			return nil, err
		}
	} else {
		if cfg.LANHost != "" {
			return nil, fmt.Errorf("lan host %q requires --trusted-lan", cfg.LANHost)
		}
		if err := cfg.validateRemote(); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// resolveStateDir applies flag > MEDIA_CORE_STATE_DIR > XDG_DATA_HOME >
// HOME precedence and validates the result.
func (c *Config) resolveStateDir(explicit string, lookup func(string) string) error {
	var raw string
	switch {
	case explicit != "":
		raw = explicit
	case lookup("MEDIA_CORE_STATE_DIR") != "":
		raw = lookup("MEDIA_CORE_STATE_DIR")
	default:
		raw = defaultStateDir(lookup)
	}
	if raw == "" {
		return fmt.Errorf("state dir: cannot resolve a default; set MEDIA_CORE_STATE_DIR, XDG_DATA_HOME or HOME")
	}
	if !filepath.IsAbs(raw) {
		return fmt.Errorf("state dir %q must be an absolute path", raw)
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return fmt.Errorf("state dir: %w", err)
	}
	info, err := os.Lstat(abs)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("state dir %q is a symlink", abs)
		}
		if !info.IsDir() {
			return fmt.Errorf("state dir %q is not a directory", abs)
		}
	case !os.IsNotExist(err):
		return fmt.Errorf("state dir: %w", err)
	}
	c.StateDir = abs
	return nil
}

// defaultStateDir returns $XDG_DATA_HOME/media-core (a relative XDG value
// is resolved against $HOME per the XDG specification) or
// $HOME/.local/share/media-core.
func defaultStateDir(lookup func(string) string) string {
	if xdg := lookup("XDG_DATA_HOME"); xdg != "" {
		if !filepath.IsAbs(xdg) {
			if home := lookup("HOME"); home == "" {
				return ""
			} else {
				xdg = filepath.Join(home, xdg)
			}
		}
		return filepath.Join(xdg, "media-core")
	}
	if home := lookup("HOME"); home != "" {
		return filepath.Join(home, ".local", "share", "media-core")
	}
	return ""
}

// resolveListener validates the listener address; the default is loopback
// and a non-loopback address never becomes the default.
func (c *Config) resolveListener(explicit string) error {
	addr := explicit
	if addr == "" {
		addr = DefaultListenAddr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("listen addr %q: %w", addr, err)
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n == 0 {
		return fmt.Errorf("listen addr %q: invalid port", addr)
	}
	if host == "" {
		return fmt.Errorf("listen addr %q: missing host", addr)
	}
	c.ListenAddr = net.JoinHostPort(host, strconv.FormatUint(n, 10))
	return nil
}

// resolvePublicOrigin validates an explicit public origin.
func (c *Config) resolvePublicOrigin(explicit string) error {
	if explicit == "" {
		return nil
	}
	u, err := url.Parse(explicit)
	if err != nil {
		return fmt.Errorf("public origin %q: %w", explicit, err)
	}
	if u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("public origin %q must be an https URL with a host", explicit)
	}
	if u.User != nil {
		return fmt.Errorf("public origin %q must not contain credentials", explicit)
	}
	c.PublicOrigin = explicit
	return nil
}

// validateRemote enforces that a non-loopback listener is never selected
// silently: it requires the explicit remote trio of allow-remote, an https
// public origin, and TLS certificate/key files. Binding itself is enforced
// by the serve lifecycle.
func (c *Config) validateRemote() error {
	if isLoopbackAddr(c.ListenAddr) {
		return nil
	}
	if !c.AllowRemote {
		return fmt.Errorf("listener %s is not loopback; explicit allow-remote is required", c.ListenAddr)
	}
	if c.PublicOrigin == "" {
		return fmt.Errorf("listener %s requires a https public origin", c.ListenAddr)
	}
	if c.TLSCertFile == "" || c.TLSKeyFile == "" {
		return fmt.Errorf("listener %s requires TLS certificate and key files", c.ListenAddr)
	}
	return nil
}

// validateTrustedLAN keeps the explicit LAN mode separate from the remote
// HTTPS trio: a private IP never infers LAN mode, and LAN mode never borrows
// the public-origin or TLS settings.
func (c *Config) validateTrustedLAN() error {
	if c.AllowRemote || c.PublicOrigin != "" || c.TLSCertFile != "" || c.TLSKeyFile != "" {
		return fmt.Errorf("trusted LAN cannot be combined with --allow-remote, --public-origin or TLS files")
	}
	if err := ValidateTrustedLANListen(c.ListenAddr); err != nil {
		return err
	}
	if c.LANHost == "" {
		return nil
	}
	return ValidateLANHost(c.LANHost)
}

// ValidateTrustedLANListen checks that a trusted LAN listener binds exactly
// one explicit private interface address: RFC1918 IPv4 or an IPv6 ULA in
// fc00::/7. Wildcards, public, loopback, link-local and multicast addresses
// are rejected so enabling LAN mode cannot silently expose a wider surface.
func ValidateTrustedLANListen(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("trusted LAN listen addr %q: %w", addr, err)
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n == 0 {
		return fmt.Errorf("trusted LAN listen addr %q: explicit port from 1 to 65535 is required", addr)
	}
	if host == "" {
		return fmt.Errorf("trusted LAN listen addr %q: an explicit private IP is required, not a wildcard", addr)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("trusted LAN listen addr %q: an explicit private IP is required, not %q", addr, host)
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("trusted LAN listen addr %q: wildcard, loopback, link-local and multicast addresses are not allowed", addr)
	}
	if v4 := ip.To4(); v4 != nil {
		if strings.Contains(host, ":") {
			return fmt.Errorf("trusted LAN listen addr %q: use a plain IPv4 address, not an IPv4-mapped IPv6 address", addr)
		}
		rfc1918 := v4[0] == 10 ||
			(v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31) ||
			(v4[0] == 192 && v4[1] == 168)
		if !rfc1918 {
			return fmt.Errorf("trusted LAN listen addr %q: public IPv4 addresses are not allowed; use an RFC1918 address", addr)
		}
		return nil
	}
	if ip[0]&0xfe == 0xfc {
		return nil
	}
	return fmt.Errorf("trusted LAN listen addr %q: IPv6 requires a ULA address in fc00::/7", addr)
}

// ValidateLANHost checks one explicit local DNS name. A scheme, port, path,
// wildcard, IP literal or mDNS .local name is rejected: only the configured
// hostname can be accepted by the HTTP Host/Origin boundary.
func ValidateLANHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("lan host must not be empty")
	}
	if strings.ContainsAny(host, ":/?#@[] \t\r\n") || strings.Contains(host, "*") {
		return fmt.Errorf("lan host %q must be one DNS name without scheme, port, path or wildcard", host)
	}
	if strings.HasSuffix(host, ".") {
		return fmt.Errorf("lan host %q must not have a trailing dot", host)
	}
	if net.ParseIP(host) != nil {
		return fmt.Errorf("lan host %q must be a DNS name, not an IP address", host)
	}
	if strings.HasSuffix(strings.ToLower(host), ".local") {
		return fmt.Errorf("lan host %q uses .local mDNS; use a local DNS name in home.arpa instead", host)
	}
	if host != strings.ToLower(host) {
		return fmt.Errorf("lan host %q must be lowercase", host)
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return fmt.Errorf("lan host %q must be a fully qualified name such as media.home.arpa", host)
	}
	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 {
			return fmt.Errorf("lan host %q has an invalid label length", host)
		}
		for i, r := range label {
			alnum := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
			if i == 0 || i == len(label)-1 {
				if !alnum {
					return fmt.Errorf("lan host %q has an invalid label %q", host, label)
				}
			} else if !alnum && r != '-' {
				return fmt.Errorf("lan host %q has an invalid label %q", host, label)
			}
		}
	}
	if len(host) > 253 {
		return fmt.Errorf("lan host %q exceeds the DNS name length limit", host)
	}
	return nil
}

func isLoopbackAddr(addr string) bool {
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

// Diagnostics renders the configuration for logging and error messages.
// Secret values are redacted.
func (c *Config) Diagnostics() string {
	origin := c.PublicOrigin
	if origin == "" {
		origin = "<unset>"
	}
	lanHost := c.LANHost
	if lanHost == "" {
		lanHost = "<unset>"
	}
	return fmt.Sprintf("stateDir=%s listen=%s publicOrigin=%s allowRemote=%t tlsCertFile=%s tlsKeyFile=%s trustedLan=%t lanHost=%s ownerSecret=%s",
		c.StateDir, c.ListenAddr, origin, c.AllowRemote, c.TLSCertFile, c.TLSKeyFile, c.TrustedLAN, lanHost, c.OwnerSecret)
}
