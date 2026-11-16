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
   file's name, size and SHA-256 is written alongside.
3. Swaps the validated build into `internal/webassets/dist` only after the
   validation passes, then links `CGO_ENABLED=0` binaries for
   `linux/amd64` and `linux/arm64` with the build identity
   (`-ldflags -X` into `internal/buildinfo`: version, short commit, UTC
   build time). The version defaults to `dev` and can be pinned with
   `MEDIA_CORE_VERSION=<version>`.
4. Smoke-tests the artifact for the host architecture (`version`, `init`,
   `list` in a throwaway state dir) and reports the other architecture as
   format-checked only: its binary is linked and verified as a valid ELF
   for its target, but it is not executed when the host cannot run it.
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

The owner secret is generated on first serve (or with the owner command)
and stored in the state directory with mode 0600. It never leaves the
state directory: backups and exports package only the database, and
restore retains the destination's own secret file. A lost secret is
recovered by re-generating and re-logging in; sessions are in-memory and
do not survive a restart.

## MCP

`media-core mcp --state-dir <dir>` speaks MCP over stdio with the same
state; it starts no listener and needs no Node. Clients connect over a
local pipe or an SSH-forwarded stdio session, which keeps the same
loopback-only posture as the service.

## Unsupported architectures

The release script always builds both `linux/amd64` and `linux/arm64`,
but it can only execute the binary that matches the host. On an amd64
host the arm64 artifact (and vice versa) is verified as a valid ELF for
its target machine and reported as format-checked only; run it on a
matching host (or emulator) for a full smoke.
