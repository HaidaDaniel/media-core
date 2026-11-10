# Architecture

Target: one Go + SQLite modular monolith with three adapters and no mandatory external service. ADR-0009 supersedes the previous prohibition of Web UI/server mode.

```text
media-core <command>   CLI, human or versioned JSON, exits after operation
media-core mcp         MCP stdio, lives for the Hermes session
media-core serve       HTTP API + embedded UI, resident only when enabled
            │
     internal/app      shared validation, identity decisions, transactions
            │
     internal/domain   records, invariants, pure validation
            │
     internal/store    SQLite queries + embedded migrations
```

CLI, HTTP and MCP call the same app services in-process, never each other's ports. DTOs and stable errors are defined in [application-contract.md](application-contract.md). No interface-specific databases, duplicated mutation rules, generic SQL tools, event bus or network-dependent core.

## Planned layout and wiring

- `cmd/media-core/main.go`: entry point; `internal/runtime`: configuration, lifecycle, service wiring and mode dispatch.
- `internal/cli`: command adapters only; `internal/httpapi`: routes and sessions; `internal/mcp`: typed tool adapters.
- `internal/app`: small per-capability services; `internal/domain`: values/invariants; `internal/store`: SQLite repositories and transaction boundary.
- `internal/store/migrations`: embedded ordered SQL; `internal/buildinfo`: version/commit/time.
- `internal/backup`: archive/validation; `internal/importer`: Release Watcher parsing/projections through app services.
- `web`: React + TypeScript + Vite source; `internal/webassets`: embedded production build and static serving.
- `tests/acceptance`, `tests/contracts`, `tests/concurrency`: focused integration tests using temporary state directories; no real user DB.

Dependencies point inward. App owns a transaction interface and receives repositories bound to that transaction; stores never commit independently mid-use-case. Keep interfaces only at actual storage/transport seams. Avoid a general dependency-injection framework.

## Configuration and process lifecycle

Explicit flags override environment, which overrides defaults. Resolve state to `MEDIA_CORE_STATE_DIR`, otherwise `$XDG_DATA_HOME/media-core`, otherwise `$HOME/.local/share/media-core`; never use the checkout by default. Files mode 0600, directory 0700. Fixed DB name `media-core.db`. Errors redact secrets. Local disk on the NAS is supported; NFS/SMB-mounted SQLite is not.

`init`/`migrate` are explicit exclusive maintenance commands. Normal CLI/MCP/HTTP startup checks schema and refuses missing/newer/older schema with `schema_mismatch` and guidance; it never upgrades under a running server. First setup is `media-core init`, then `serve` or normal commands. `version` and read-only environment doctor do not create state. SIGTERM stops HTTP acceptance, drains requests for up to 10 seconds, closes SQLite and releases file locks. MCP EOF closes cleanly. No scheduler; disabled HTTP has no listener or background resource use. Resident HTTP and active MCP necessarily consume idle resources; the earlier universal “zero idle RAM” claim no longer applies.

## SQLite and concurrent processes

Use pure-Go SQLite (`modernc.org/sqlite` recommended; pin a supported version in task 0008 and record it in decisions). WAL, foreign_keys ON, synchronous FULL and 5000 ms busy timeout on **every connection**, bounded pool (initially one connection per process), bounded read queries. SQLite permits one writer; short `BEGIN IMMEDIATE` writes serialize identity checks, history changes and response storage. Never hold a transaction across network/agent/UI interaction. Busy exhaustion returns retryable `locked`; do not automatically retry an uncertain commit. Idempotency keys handle safe retries. Expected revisions reject stale writes with `conflict`.

All DB-opening processes acquire a shared OS advisory lock on a stable `maintenance.lock` in the canonical state directory for their **whole DB lifetime**. Backup takes the shared lock too. Init, migrations and restore acquire the exclusive lock before opening or replacing the DB; if any server/MCP/CLI handle remains active they return `locked` after at most five seconds. Do not unlink or replace this lock file; separate task/APK locks are unrelated. Local filesystem ownership is the trust boundary. Resolve the same directory for every mode; reject symlinked DB/lock files and unsupported network-filesystem deployment. No global application write lock is needed in addition to SQLite transactions.

A consistent backup uses SQLite `VACUUM INTO` to a new temporary database (or a proven driver online-backup API if task 0022 documents a reason), never copies a live DB alone while ignoring WAL. Validate the snapshot and package DB plus SHA-256/size/schema/app-version manifest. Restore validates archive paths/limits/hash/schema/integrity into a temp directory **before** replacing anything, requires exclusive maintenance access, keeps a previous DB for recovery, and atomically renames on the same filesystem. Refuse newer schemas; restore supported current-schema backups initially. Stop all modes before restore; no hot replacement under open handles. Test interruption and WAL sidecar cleanup. Backup excludes owner secret and sessions; restore retains destination security settings.

The rationale for local-disk WAL and snapshot backups follows [SQLite WAL](https://sqlite.org/wal.html) and [SQLite backup documentation](https://sqlite.org/backup.html); driver/version behavior is verified during implementation.

## HTTP and browser security

Default `serve` listens on `127.0.0.1:8787`. No wildcard fallback. Require a single-owner login even on loopback: init generates a 256-bit owner secret in a 0600 file; an explicit local CLI command displays it, never logs it. POST `/api/v1/session` exchanges it for an opaque in-memory session and CSRF token. Constant-time secret comparison, bounded login rate limit, idle expiry 30 minutes, absolute expiry 12 hours, rotation on login, logout invalidation, restart invalidation. Cookies: HttpOnly, SameSite=Strict, Path=/, Secure on TLS (local HTTP exception only). No credentials embedded in JS/build env/localStorage or URLs. Frontend holds the CSRF token only in memory, refreshing through authenticated session inspection.

Validate Host against configured listener/public host and validate exact Origin on login and every unsafe browser request. Require a matching per-session CSRF header on all authenticated mutations; GET/HEAD are side-effect-free. No permissive CORS; no origin wildcard; reject cross-site requests and hostile DNS-rebinding Host values. Limit body to 1 MiB, page size to 100 (default 30), note text to 16 KiB and title to 512 Unicode code points. Parameterized SQL, plain-text rendering, CSP with self-only scripts and no inline/eval, nosniff, frame-ancestors none, Referrer-Policy no-referrer. Links use safe protocols and noopener/noreferrer; no remote covers or URL fetching in MVP.

Non-loopback startup requires **all** of: `--allow-remote`, configured `--public-origin https://...`, and Go TLS certificate/key. Missing any fails before binding. Single-owner sessions remain mandatory; TLS does not replace authentication. No proxy-header trust by default. Initial NAS recommendation: keep loopback and use an SSH port forward; authenticated direct TLS is available for users who deliberately expose a listener. No unauthenticated writable LAN mode, hosted account or RBAC. Native HTTP MCP is not mounted by `serve`.

## Web build and delivery

Vite builds static files; Go embeds a staged build under `internal/webassets/dist` (task 0047). Production build runs frontend build first and validates the asset manifest. `serve` serves same-origin `/api/v1` and the SPA; API misses return JSON 404, never index.html. Hashed assets use immutable cache, index uses no-cache; route fallback is limited to UI paths. CLI/MCP do not start HTTP or require Node. During development Vite proxies `/api` to loopback Go with an explicit permitted dev origin; production restrictions stay closed. [Vite production builds](https://vite.dev/guide/build.html) support this static asset arrangement.

## Remote Hermes and boundaries

Same host: Hermes launches `media-core mcp`. Different host: Hermes launches SSH without a PTY to run the NAS binary and carry stdio, with a dedicated OS account/key and host-key verification; SQLite remains local to NAS. Exact deployment and client smoke tests belong to [Hermes](hermes.md). Streamable HTTP MCP is a future adapter only if a client cannot launch SSH/stdio; it would require authenticated TLS and MCP Origin protections and reuse these same services, not open a second DB over the network.

Release Watcher integration is file/stdin JSON import, never a shared DB or shell tool invoked by MCP. Translation/content providers stay external and optional. No general plugin runtime is designed.
