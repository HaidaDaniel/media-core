package artwork

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Network bounds. They are deliberately small: cover discovery is an
// explicit user action on a resource-limited NAS, never a batch crawler.
const (
	requestTimeout   = 8 * time.Second
	connectTimeout   = 3 * time.Second
	maxRedirects     = 3
	maxJSONBytes     = 1 << 20
	maxImageBytes    = 2 << 20
	perProviderRPS   = 1
	maxProviderTries = 2
)

var (
	ErrNetworkURL    = errors.New("artwork: unsafe upstream URL")
	ErrUpstreamSize  = errors.New("artwork: upstream response too large")
	ErrUpstreamMIME  = errors.New("artwork: unexpected upstream content type")
	ErrUpstreamRetry = errors.New("artwork: upstream retry limit reached")
)

// fetchResult is one bounded upstream response.
type fetchResult struct {
	Body        []byte
	ContentType string
}

// limiter is a one-request-per-second token bucket per provider. It is
// intentionally simple and deterministic: no background goroutine is started.
type limiter struct {
	mu   sync.Mutex
	next time.Time
}

func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	start := l.next
	if start.Before(now) {
		start = now
	}
	l.next = start.Add(time.Second / perProviderRPS)
	l.mu.Unlock()
	if wait := time.Until(start); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

// safeClient is the only outbound HTTP client in the application. It allows
// HTTPS only, checks every redirect against the same host allowlist, rejects
// non-public literal IPs at request time and verifies resolved addresses at
// dial time, and caps bytes read with LimitReader.
type safeClient struct {
	client       *http.Client
	limits       map[string]*limiter
	userAgent    string
	allowPrivate bool // tests only, never set by production wiring
	mu           sync.Mutex
}

func newSafeClient(userAgent string) *safeClient {
	c := &safeClient{limits: map[string]*limiter{}, userAgent: userAgent}
	dialer := &net.Dialer{Timeout: connectTimeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		// DialTLSContext resolves and checks the address, then dials the
		// verified IP directly while keeping the hostname for certificate
		// verification. This closes the usual DNS-rebinding gap between a
		// checked DialContext and a second resolution inside net/http.
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("%w: bad dial address", ErrNetworkURL)
			}
			target := host
			if !c.allowPrivate {
				ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
				if err != nil || len(ips) == 0 {
					return nil, fmt.Errorf("%w: DNS lookup failed", ErrNetworkURL)
				}
				for _, ip := range ips {
					if !isPublicIP(ip.IP) {
						return nil, fmt.Errorf("%w: non-public address", ErrNetworkURL)
					}
				}
				target = ips[0].IP.String()
			}
			raw, err := dialer.DialContext(ctx, network, net.JoinHostPort(target, port))
			if err != nil {
				return nil, err
			}
			tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}
			if c.allowPrivate {
				tlsConfig.InsecureSkipVerify = true
			}
			conn := tls.Client(raw, tlsConfig)
			if err := conn.HandshakeContext(ctx); err != nil {
				_ = raw.Close()
				return nil, err
			}
			return conn, nil
		},
		TLSHandshakeTimeout: connectTimeout,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     15 * time.Second,
	}
	c.client = &http.Client{
		Transport: transport,
		Timeout:   requestTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("%w: too many redirects", ErrNetworkURL)
			}
			return c.validateURL(req.URL, via[0].URL)
		},
	}
	return c
}

func (c *safeClient) limiterFor(provider string) *limiter {
	c.mu.Lock()
	defer c.mu.Unlock()
	l, ok := c.limits[provider]
	if !ok {
		l = &limiter{}
		c.limits[provider] = l
	}
	return l
}

// validateURL requires HTTPS, no credentials, an allowlisted host and a
// public literal IP when the host is an IP. allowed is optional: when set,
// the redirect host must match the original request host set.
func (c *safeClient) validateURL(u *url.URL, original *url.URL) error {
	if u == nil || !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("%w: https is required", ErrNetworkURL)
	}
	if u.User != nil {
		return fmt.Errorf("%w: userinfo is not allowed", ErrNetworkURL)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("%w: host is required", ErrNetworkURL)
	}
	if ip := net.ParseIP(host); ip != nil && !c.allowPrivate && !isPublicIP(ip) {
		return fmt.Errorf("%w: private literal address", ErrNetworkURL)
	}
	if original != nil && strings.ToLower(original.Hostname()) != host {
		// Redirects may move between hosts of the same provider (for example
		// api.kitsu.io -> media.kitsu.app), so the caller supplies the full
		// allowlist in allowedHosts below; strict same-host is checked there.
		return fmt.Errorf("%w: cross-host redirect", ErrNetworkURL)
	}
	return nil
}

func isPublicIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsPrivate() || ip.To4() == nil && !isPublicIPv6(ip) {
		return false
	}
	return true
}

func isPublicIPv6(ip net.IP) bool {
	if ip.To16() == nil {
		return false
	}
	// Unique-local and IPv4-mapped addresses are not public.
	return ip.To4() == nil && !strings.HasPrefix(ip.String(), "fc") && !strings.HasPrefix(ip.String(), "fd")
}

// fetch performs one bounded GET. allowedHosts is the exact provider host
// allowlist for both the initial URL and every redirect.
func (c *safeClient) fetch(ctx context.Context, provider, rawURL string, allowedHosts map[string]bool, maxBytes int64, accept string) (fetchResult, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fetchResult{}, fmt.Errorf("%w: %v", ErrNetworkURL, err)
	}
	if err := c.validateURL(u, nil); err != nil {
		return fetchResult{}, err
	}
	if !allowedHosts[strings.ToLower(u.Hostname())] {
		return fetchResult{}, fmt.Errorf("%w: host %q is not allowlisted", ErrNetworkURL, u.Hostname())
	}
	if err := c.limiterFor(provider).wait(ctx); err != nil {
		return fetchResult{}, err
	}
	var lastErr error
	var got fetchResult
	var retryDelay time.Duration
	for attempt := 0; attempt < maxProviderTries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return fetchResult{}, fmt.Errorf("%w: %v", ErrNetworkURL, err)
		}
		req.Header.Set("User-Agent", c.userAgent)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return fetchResult{}, ctx.Err()
			}
			if attempt+1 < maxProviderTries {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			break
		}
		func() {
			defer resp.Body.Close()
			finalHost := strings.ToLower(resp.Request.URL.Hostname())
			if !allowedHosts[finalHost] {
				lastErr = fmt.Errorf("%w: redirect host %q is not allowlisted", ErrNetworkURL, finalHost)
				return
			}
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
				retryDelay = parseRetryAfter(resp.Header.Get("Retry-After"))
				lastErr = fmt.Errorf("%w: status %d", ErrUpstreamRetry, resp.StatusCode)
				return
			}
			if resp.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("artwork: status %d", resp.StatusCode)
				return
			}
			if ct := resp.Header.Get("Content-Type"); accept == "" || strings.Contains(accept, "json") {
				if ct != "" && !strings.Contains(strings.ToLower(ct), "json") {
					lastErr = fmt.Errorf("%w: %s", ErrUpstreamMIME, ct)
					return
				}
			} else if ct != "" && !isImageMIME(ct) {
				lastErr = fmt.Errorf("%w: %s", ErrUpstreamMIME, ct)
				return
			}
			body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
			if err != nil {
				lastErr = err
				return
			}
			if int64(len(body)) > maxBytes {
				lastErr = ErrUpstreamSize
				return
			}
			lastErr = nil
			got = fetchResult{Body: body, ContentType: resp.Header.Get("Content-Type")}
		}()
		if lastErr == nil {
			return got, nil
		}
		if !errors.Is(lastErr, ErrUpstreamRetry) {
			break
		}
		if attempt+1 < maxProviderTries {
			delay := 400 * time.Millisecond
			if retryDelay > delay {
				delay = retryDelay
			}
			time.Sleep(delay)
		}
	}
	if lastErr == nil {
		lastErr = ErrUpstreamRetry
	}
	return fetchResult{}, lastErr
}

func isImageMIME(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	switch ct {
	case "image/jpeg", "image/png", "image/gif":
		return true
	}
	return false
}

// parseRetryAfter caps an upstream delay so a hostile value cannot pin the
// request for minutes.
func parseRetryAfter(raw string) time.Duration {
	if raw == "" {
		return 0
	}
	if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
		if secs > 2 {
			return 2 * time.Second
		}
		return time.Duration(secs) * time.Second
	}
	return 0
}

// postJSON performs a bounded HTTPS POST for GraphQL providers. It applies
// the same URL, host, byte and rate checks as fetch.
func (c *safeClient) postJSON(ctx context.Context, provider, rawURL string, body []byte, allowedHosts map[string]bool) (fetchResult, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fetchResult{}, fmt.Errorf("%w: %v", ErrNetworkURL, err)
	}
	if err := c.validateURL(u, nil); err != nil {
		return fetchResult{}, err
	}
	if !allowedHosts[strings.ToLower(u.Hostname())] {
		return fetchResult{}, fmt.Errorf("%w: host %q is not allowlisted", ErrNetworkURL, u.Hostname())
	}
	if err := c.limiterFor(provider).wait(ctx); err != nil {
		return fetchResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return fetchResult{}, fmt.Errorf("%w: %v", ErrNetworkURL, err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return fetchResult{}, err
	}
	defer resp.Body.Close()
	if !allowedHosts[strings.ToLower(resp.Request.URL.Hostname())] {
		return fetchResult{}, fmt.Errorf("%w: redirect host not allowlisted", ErrNetworkURL)
	}
	if resp.StatusCode != http.StatusOK {
		return fetchResult{}, fmt.Errorf("%w: status %d", ErrUpstreamRetry, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBytes+1))
	if err != nil {
		return fetchResult{}, err
	}
	if int64(len(data)) > maxJSONBytes {
		return fetchResult{}, ErrUpstreamSize
	}
	return fetchResult{Body: data, ContentType: resp.Header.Get("Content-Type")}, nil
}

// decodeJSON rejects trailing bytes after the first JSON value so a provider
// cannot stream an unbounded document past the byte limit.
func decodeJSON(body []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("artwork: decode upstream JSON: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("artwork: trailing upstream JSON")
	}
	return nil
}
