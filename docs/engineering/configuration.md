# Configuration

Configuration resolution is pure and deterministic: explicit flags override
environment variables, which override defaults. Resolving configuration
never creates a state directory, binds a listener, or renders secret
values.

## Values

| Value | Flag | Environment | Default |
| --- | --- | --- | --- |
| State directory | explicit flag | `MEDIA_CORE_STATE_DIR` | `$XDG_DATA_HOME/media-core`, else `$HOME/.local/share/media-core` |
| Listener address | explicit flag | — | `127.0.0.1:8787` |
| Public origin | explicit flag | — | unset (loopback mode) |
| Allow remote | explicit flag | — | `false` |
| TLS certificate file | explicit flag | — | unset |
| TLS key file | explicit flag | — | unset |

## State directory

- The value must be an absolute path; relative values are rejected.
- The default is always resolved from `XDG_DATA_HOME` or `HOME`, never from
  the checkout or working directory. A relative `XDG_DATA_HOME` is resolved
  against `HOME` per the XDG specification.
- An existing state directory that is a symlink is rejected; a regular
  directory is accepted and a missing path is accepted (creation happens
  only in explicit maintenance commands).
- Files inside the state directory use mode 0600 and the directory uses
  0700; the database file is fixed as `media-core.db`.

## Listener and remote access

- The default listener is loopback. There is no wildcard fallback: an
  unspecified listener never selects a public interface.
- A non-loopback listener is valid only when **all** of the following are
  explicit: allow-remote, a public origin that is an https URL with a host
  and no credentials, and TLS certificate and key file paths. Missing any
  of them is a validation error. Actually binding the listener and
  enforcing the TLS files happens in the `serve` lifecycle.
- The public origin must use https; credentials in the origin are
  rejected.

## Serve lifecycle (task 0024)

- `media-core serve` runs the resident HTTP lifecycle: resolve
  configuration (a non-loopback bind without the full remote trio fails
  here, before any listener exists), build the server (TLS certificate and
  key must load, again before any listener), bind, and serve until SIGINT
  or SIGTERM.
- Graceful shutdown: on a signal the server stops accepting connections,
  drains in-flight requests for up to 10 seconds, closes the listener, and
  exits 0. A process restart invalidates every session because sessions
  live in memory only.
- This milestone opens no database; the server exposes only the
  `/api/v1/session` routes (login, inspection, logout) behind the security
  boundary. Every other route is a JSON 404, so there is no
  unauthenticated writable endpoint.
- Security boundary: every response carries a self-only CSP (no inline or
  eval), `nosniff`, `frame-ancestors 'none'`/`DENY`, and
  `Referrer-Policy: no-referrer`; no CORS headers are sent. The `Host`
  header must match the configured listener (loopback) or the public
  origin host (remote); unsafe requests (POST/PUT/PATCH/DELETE) require an
  exact `Origin` match, and a missing Origin is never treated as a
  non-browser bypass. Authenticated mutations require the per-session
  `X-CSRF-Token` header. Request bodies are limited to 1 MiB.
- Login is rate limited; the session cookie is `HttpOnly`,
  `SameSite=Strict`, `Path=/`, `Secure` on TLS (local loopback HTTP is the
  sole exception), and bounded by the 12-hour absolute expiry. Idle
  sessions expire after 30 minutes; a new login rotates the single
  session.

## Owner secret

- `media-core owner generate` creates a 256-bit owner secret in the state
  directory as `owner-secret` (mode 0600) and prints its hex encoding to
  stdout; generation is idempotent and never replaces an existing secret.
  `media-core owner show` prints the existing hex encoding and fails when
  the file is absent.
- The secret value is printed only to the local operator's stdout and is
  never written to logs, diagnostics, or envelopes; the server compares it
  in constant time and reports a missing file distinctly from a wrong
  secret.
- Note: the original design had `init` generate the secret; `init` is
  owned by an earlier task, so generation lives in the explicit `owner`
  command instead.

## Secrets

- The owner secret is wrapped in a secret type whose rendered form is
  always `redacted` (or `<unset>`); configuration diagnostics never contain
  the secret value.
- TLS key material is not part of the resolved configuration; only the TLS
  certificate and key **file paths** are held and rendered, and enforcing
  those files happens in the `serve` lifecycle.
- Diagnostics render state directory, listener, public origin, allow-remote
  flag, and TLS file paths only.

## Testing notes

Precedence, XDG/HOME fallback, rejection of relative and symlinked state
directories, the non-loopback trio, public origin validation, secret
redaction, and the no-side-effect guarantee are covered by unit tests in
`internal/config`.
