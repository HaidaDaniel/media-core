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
  enforcing the TLS files happens in the `serve` lifecycle (task 0024).
- The public origin must use https; credentials in the origin are
  rejected.

## Secrets

- The owner secret is wrapped in a secret type whose rendered form is
  always `redacted` (or `<unset>`); configuration diagnostics never contain
  the secret value.
- TLS key material is not part of the resolved configuration; only the TLS
  certificate and key **file paths** are held and rendered, and enforcing
  those files happens in the `serve` lifecycle (task 0024).
- Diagnostics render state directory, listener, public origin, allow-remote
  flag, and TLS file paths only.

## Testing notes

Precedence, XDG/HOME fallback, rejection of relative and symlinked state
directories, the non-loopback trio, public origin validation, secret
redaction, and the no-side-effect guarantee are covered by unit tests in
`internal/config`.
