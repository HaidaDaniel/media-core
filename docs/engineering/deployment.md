# Deployment

This document describes how to build the release artifacts and run
media-core on a host. media-core is a single static Go binary: the HTTP
API, the embedded web UI, the CLI and the MCP stdio server all live in it.
There is no runtime dependency on Node, pnpm or Docker; the only
filesystem requirements are the state directory and, for remote access,
TLS certificate files.

## Release artifacts

`scripts/build-release.sh` builds the complete release. It is the only
step that needs a toolchain beyond Go: the frontend build runs
`pnpm --dir web exec vite build`, after which the script stages and
validates the assets before embedding them. The script:

1. Builds the frontend with Vite.
2. Stages the build in a temporary directory and validates it: the index
   shell is non-empty, it references no external `http(s)` resources, and
   every local reference resolves to a staged file. A manifest with each
   file's name, size and SHA-256 is written alongside and then verified at
   runtime against the real staged files: complete file set, no self-entry,
   matching sizes and hashes. An artifact gate run with
   `MEDIA_CORE_RELEASE_GATE=1 go test ./tests/packaging -run TestPackageReleaseArtifacts`
   fails if either release binary is missing; without the flag a local test
   run may skip when artifacts are absent.
3. Swaps the validated build into `internal/webassets/dist` only after the
   validation passes, then links `CGO_ENABLED=0` binaries for
   `linux/amd64` and `linux/arm64` with the build identity
   (`-ldflags -X` into `internal/buildinfo`: version, short commit, UTC
   build time). The version defaults to `dev` and can be pinned with
   `MEDIA_CORE_VERSION=<version>`.
4. Verifies both artifacts' ELF magic and target machine types with
   `go test ./tests/packaging -run TestPackageReleaseArtifacts`, then
   smoke-tests the host-matching artifact (`version`, `init`, `list` in a
   throwaway state dir). The other architecture is format-checked only: it
   is linked and its ELF machine type is verified, but it is not executed
   when the host cannot run it.
5. Restores the tracked placeholder in `internal/webassets/dist`, so the
   working tree stays clean; the release binaries in `release/` keep the
   real assets.

Artifacts:

```
release/
  media-core-linux-amd64
  media-core-linux-arm64
```

Both are statically linked (`CGO_ENABLED=0`), self-contained, and embed
the validated UI. `media-core version` prints the identity line:
`media-core <version> (commit <commit>, built <time>, go <go>)`.

The embedded UI is the validated build when the script has run and the
tracked placeholder otherwise; the placeholder renders a notice that the
release build has not been staged. Staged build files are git-ignored
(`internal/webassets/dist/*` except the placeholder); `release/` is
git-ignored as well. Nothing in the build publishes: no tag, no push, no
upload.

## Running the service

The state directory holds the database, the owner secret and the
maintenance lock. Initialize it once:

```
media-core init --state-dir <dir>
```

Then serve. The default listener is loopback; there is no wildcard
fallback.

```
media-core serve --state-dir <dir>
```

The UI is served same-origin at `/` (with client-side route fallback),
the API under `/api/v1`, and the owner-secret login at
`POST /api/v1/session`. Static assets under `/assets/` are served
`immutable` (Vite hashes their names); the index shell is served
`no-cache` so a new release is picked up on the next load.

### Remote access

The supported remote topology keeps the listener on loopback and tunnels
through SSH, so the TLS and origin configuration stays out of the
tunnel:

```
ssh -N -L 8480:127.0.0.1:8480 user@host
```

pointed at a local `media-core serve --state-dir <dir> --listen
127.0.0.1:8480`. The browser then talks to `http://127.0.0.1:8480` and
the loopback security policy (host and origin checks) is unchanged.

A directly reachable listener is also possible but requires **all** of:
`--allow-remote`, an https `--public-origin`, and `--tls-cert` /
`--tls-key` files. Missing any of them is a validation error before the
listener binds. The public origin must be https with a host and no
credentials; the origin is the only accepted `Origin` for unsafe
requests, and the session cookie is bound to it.

### Owner secret

The owner secret is created only by `media-core owner generate --state-dir
<dir>` and stored in the state directory with mode 0600; neither `init` nor
`serve` creates it, and login fails with a distinct "not configured" 401
until it exists. It never leaves the state directory: backups and exports
package only the database, and restore retains the destination's own
secret file. Regeneration is idempotent and never overwrites an existing
secret, so a forgotten secret is an operator recovery decision, not a
`generate` retry. Sessions are in-memory and do not survive a restart.

## MCP

`media-core mcp --state-dir <dir>` speaks MCP over stdio with the same
state; it starts no listener and needs no Node. Clients connect over a
local pipe or an SSH-forwarded stdio session, which keeps the same
loopback-only posture as the service. `media-core mcp --http` opts into a
loopback Streamable HTTP listener mounted only at `/mcp`; every request
needs the state directory's owner secret as a Bearer token, and a
non-loopback bind requires the same allow-remote + https public-origin +
TLS trio as `serve`. The Origin guard is configured by the SDK; `serve`
never mounts an MCP endpoint.

## Unsupported architectures

The release script always builds both `linux/amd64` and `linux/arm64`,
but it can only execute the binary that matches the host. It invokes the
packaging test for both artifacts, which checks the ELF magic and the
`e_machine` field against the target architecture; the non-host artifact is
then reported as format-checked only. Run it on a matching host (or
emulator) for a full smoke.

## First-run setup (operator runbook)

1. Install the binary and keep state outside the checkout: `/opt/media-core/media-core`, state in `/var/lib/media-core` (directory 0700, files 0600).
2. `media-core init --state-dir /var/lib/media-core` creates and migrates the database. It does **not** create the owner secret.
3. `media-core owner generate --state-dir /var/lib/media-core` writes a 256-bit secret to a 0600 file and prints it once to the local operator's stdout; `owner show` displays it again locally. Never put it in a unit file, workflow, URL or browser storage.
4. Start with `media-core serve --state-dir /var/lib/media-core --listen 127.0.0.1:8787` (or enable the example `deploy/media-core.service`). Log in from the same host or through an SSH port forward; direct remote access additionally requires `--allow-remote`, an https `--public-origin` and a TLS certificate/key.

## Service management

`deploy/media-core.service`, `deploy/media-core-backup.service` and `deploy/media-core-backup.timer` are examples, not an installation step. They use a dedicated `media-core` account, `NoNewPrivileges`, `ProtectSystem=strict`, `RestrictSUIDSGID` and explicit `ReadWritePaths` for the state and backup directories; they contain no secret values. Review paths and the account before enabling any unit.

The backup unit sets `Environment=MEDIA_CORE_BIN=/opt/media-core/media-core`; the smoke script defaults to that installed absolute path and rejects a relative or non-executable override, so a scheduled run does not depend on `PATH`. Before enabling the timer, create `/var/lib/media-core` and `/var/backups/media-core` owned by `media-core` with mode 0700, install the binary at `/opt/media-core/media-core` and the executable script at `/opt/media-core/scripts/backup-smoke.sh`, and verify that the service account can read the working directory and open `<state-dir>/maintenance.lock` for the shared backup lock. `ProtectSystem=strict`, `ProtectHome` and `PrivateTmp` are compatible with the smoke because only the state and backup directories (plus the private temp directory) need writes; a running `serve`/`mcp` process only holds the shared lock and does not block a backup. The timer uses `Persistent=true` and `WantedBy=timers.target`, so a missed run is executed after reboot. Errors exit non-zero, and neither unit nor script log the owner secret. `tests/packaging/deployment_test.go` checks these configuration invariants and runs a systemd-like smoke with an explicit binary path; it does not start real systemd.

## Backup and restore smoke

`scripts/backup-smoke.sh <state-dir> <backup-dir> [prefix]` creates a consistent snapshot archive, restores it into a fresh temporary state directory and runs a read there. The timer passes the prefix `media-core-auto-`; a manual invocation defaults to `media-core-smoke-`. It never stops the service, changes the live database or publishes anything. The destination directory must be outside the state directory. Run it manually after setup and schedule it with `deploy/media-core-backup.timer` (daily by default). A real restore of a live state remains an explicit operator action: stop all handles first, validate the archive, then use `media-core restore <archive> --state-dir <dir> [--force]`.

## CI and release gate

`.github/workflows/ci.yml` runs build, vet, tests, race tests, shell syntax, structural checks, TypeScript, Vitest, Playwright and a local release build on every push/PR. `.github/workflows/release-gate.yml` is separate and non-publishing: it re-runs verification and uploads local artifacts only. Neither workflow deploys to a NAS.

## Retention

Automatic archives (`media-core-auto-<UTC timestamp>`) receive a `.verified` marker only after a new archive passes restore smoke. `scripts/backup_retention.py <backup-dir>` keeps the newest 7 UTC daily, 4 ISO-weekly and 3 monthly buckets, always keeps the newest verified archive, and deletes only automatic, marked archives. Manual archives, unknown names and symlinks are never touched; deletions run under a bounded lock and only after the successful smoke that invoked the script. Use `--dry-run` to print the exact deletion plan without changing files. Retention is not a substitute for a separate copy on another physical medium.
