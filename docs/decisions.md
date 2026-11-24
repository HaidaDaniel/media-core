# Decisions

Architecture and dependency decisions for Media Core. Add a new entry for every material decision.

## ADR-0001 — Separate repository, database and event contract from Release Watcher

- Date: 2026-11-04
- Status: Accepted
- Context: Release Watcher detects publications and owns watch state. Media Core owns personal history, progress and opinions. Sharing a database would couple deployments, schemas and failure modes.
- Decision: Media Core and Release Watcher are separate Go binaries with separate SQLite files. Integration is through the versioned JSON event contract. Media Core never opens Release Watcher's database.
- Consequences: Either project works alone. Import can lag or be replayed. Cross-project changes require contract versioning, not a shared migration.

## ADR-0002 — Go + pure-Go SQLite, one binary, on demand

- Date: 2026-11-04
- Status: Accepted
- Decision: Use Go and a pure-Go SQLite driver with `CGO_ENABLED=0`, one CLI binary, WAL and short transactions. State lives outside the source tree. No daemon, server, Docker, PostgreSQL, Redis or broker is required.
- Consequences: Linux x86-64/ARM64 builds are simple and idle resource use is zero. A future feature that needs a resident process requires a new explicit decision.

## ADR-0003 — CLI first, MCP later over the same use cases

- Date: 2026-11-04
- Status: Accepted
- Decision: The CLI with stable JSON is the first and primary interface. MCP stdio is added only after the CLI is proven, and it calls the same application use cases. It does not duplicate business logic.
- Consequences: Any agent can drive Media Core locally without a server. MCP is optional and cannot become a second source of rules.

## ADR-0004 — Editions are explicit; no automatic title merge

- Date: 2026-11-04
- Status: Accepted
- Decision: WN, LN, book, manga, film, series, anime and translations are separate Editions under a MediaWork. An Edition always has one owning MediaWork; grouping several editions is the purpose of that Work, not automatic title matching. Relations (`same_story`, `translation_of`, `adaptation_of`, etc.) are explicit and provenance-bearing. Titles, covers or fuzzy similarity never merge editions automatically.
- Consequences: Counts and progress are edition-scoped. Users/agents can link editions deliberately. Release Watcher publication lines map cleanly to editions.

## ADR-0005 — Immutable event import deduplicated by event_id

- Date: 2026-11-04
- Status: Accepted
- Decision: Release Watcher events are stored as immutable imported records with the original payload and schema version. Import is idempotent by `event_id`. Re-import is a no-op. Corrections are new events or explicit user annotations.
- Consequences: At-least-once delivery is safe. Historical projections can be rebuilt without asking Release Watcher to resend. Unsupported major schema versions fail closed.

## ADR-0006 — Content delivery is a future module, not an MVP dependency

- Date: 2026-11-04
- Status: Accepted concept
- Decision: Document `ContentReference`, `ContentAsset`, `ContentProvider` and `DeliveryTarget` so the schema can accommodate them later. Do not implement downloading, streaming, DRM handling, provider crawling or delivery in the MVP.
- Consequences: Library truth stays small and local. Future delivery requires separate tasks, explicit provider contracts and rights/provenance handling.

## ADR-0007 — Translations are linked assets with external harness provenance

- Date: 2026-11-04
- Status: Accepted concept
- Decision: An original and its translation are linked as separate assets/editions with language and translator/harness provenance. Media Core does not embed an LLM or execute translation in the MVP. Missing translations fall back to the original; none are invented.
- Consequences: Translation is optional and auditable. The translation harness remains an external integration.

## ADR-0008 — Local-first without mandatory cloud or LLM

- Date: 2026-11-04
- Status: Accepted
- Decision: All core library operations, backup and restore work offline. External metadata, covers, translation and delivery are optional explicit integrations that may fail without blocking local data. No telemetry or mandatory hosted account.
- Consequences: The product remains usable on a home device and independent of any model/provider.


## ADR-0009 — First-class Web UI and MCP; optional resident HTTP mode

- Date: 2026-11-10
- Status: Accepted for the revised target, implementing the operator's product instruction; not implementation approval/evidence.
- Supersedes: ADR-0002 only where it prohibits server mode or universally promises zero idle RAM; ADR-0003 only where it makes CLI primary and MCP a deferrable optional stage. Original entries above remain historical records.
- Decision: One Go binary has CLI, MCP stdio and `serve` modes over shared app services. React + TypeScript + Vite produces embedded static assets. Both Web UI and Hermes/MCP are MVP requirements; CLI/MCP need no HTTP. Node remains build tooling only.
- Rationale: Daily manual use and agent interaction are both core product needs. A modest resident process is justified when the UI is enabled; disabled modes require no daemon.
- Consequences: HTTP lifecycle/session/security and frontend build tasks are required. Loopback default; no automatic network exposure. No extra backend service, production Node server or Docker dependency.

## ADR-0010 — Orthogonal state and recoverable personal observations

- Date: 2026-11-10
- Status: Accepted target clarification of ADR-0004.
- Decision: Consumption, intention and sourced publication state are independent. Editions hold progress/rating/state histories; work or edition notes use revisions. Genuine changes append observations, corrections preserve superseded/retracted records. Translation is a relationship, not a medium. Multiple provenance-bearing title rows replace three hardcoded language fields.
- Consequences: Unknown is explicit, paused/on-hold aliases disappear, progress never silently completes a work. Ordinary relational history, not event sourcing. Shared validation and idempotency prevent interface drift.

## ADR-0011 — Explicit graph and deterministic identity resolution

- Date: 2026-11-10
- Status: Accepted target clarification of ADR-0004/0005.
- Decision: Work and edition relationships have constrained endpoint types, direction, provenance and proposed/confirmed/revoked states. Titles only suggest candidates. External exact identities and request keys support safe retries; contradictory identities fail. Source-edge cycles are rejected transactionally. Imported payloads stay immutable; corrected bindings are separate audited records.
- Consequences: Users can preserve completed anime and unread manga under one story or link separate groupings. No inferred WN-first origin, silent fuzzy merge, history purge or graph database. Origin assertions and sources retain uncertainty.

## ADR-0012 — Single-owner sessions and SSH for initial remote Hermes

- Date: 2026-11-10
- Status: Accepted target deployment choice; no host deployment authorized.
- Decision: Loopback HTTP still requires owner login, Origin/Host/CSRF protections and short-lived opaque sessions. Non-loopback requires explicit allow-remote, configured HTTPS origin and Go TLS keys. Initial NAS Hermes access uses stdio via verified SSH, not unauthenticated remote MCP. Native Streamable HTTP MCP is deferred pending a concrete client requirement and its own auth design.
- Consequences: No multi-user RBAC. Secrets live outside the frontend/repository. SSH tunnel is the simplest UI option; local-only use needs neither TLS nor SSH. Tests must cover missing credentials and unsafe defaults.

## ADR-0013 — Shared SQLite lifecycle and consistent maintenance

- Date: 2026-11-10
- Status: Accepted target refinement of ADR-0002.
- Decision: Local-disk WAL, per-connection foreign keys/busy timeout, short immediate write transactions; shared lifetime maintenance lock for open handles and exclusive init/migrate/restore. Explicit migrations rather than hot schema changes. Snapshot backup includes manifest/hash; restore validates before replacement and requires stopped processes.
- Consequences: A persistent serve/MCP process blocks restore until stopped. No copying an uncheckpointed live DB alone, no NFS database, no cross-host SQLite mount. Pure-Go driver must prove snapshot/concurrency behavior in focused tests. Dependencies are selected/pinned in implementation tasks with additional decision entries; none are added by planning.

## ADR-0014 — Pin modernc.org/sqlite v1.62.0 as the pure-Go driver

- Date: 2026-11-10
- Status: Accepted
- Context: ADR-0013 defers driver selection to implementation. Task 0008 needs a supported pure-Go SQLite driver (ADR-0002, `CGO_ENABLED=0`). modernc.org/sqlite v1.62.0 is the newest release at selection time; it requires Go >= 1.26 (the installed toolchain, Go 1.27.1, is newer; go.mod raises the go directive accordingly). It is a direct dependency together with golang.org/x/sys (used directly for `flock`-based maintenance locking); the remaining indirect closure in go.mod is modernc.org/libc, modernc.org/memory, modernc.org/mathutil, dustin/go-humanize, google/uuid, mattn/go-isatty, ncruces/go-strftime and remyoudompheng/bigfft.
- Decision: Pin modernc.org/sqlite v1.62.0. WAL, foreign keys, synchronous FULL and the 5000 ms busy timeout are applied per connection through driver pragma parameters and verified on the live connection after open.
- Consequences: No C toolchain for builds; Linux x86-64/ARM64 remain simple. Snapshot (`VACUUM INTO`) and multi-process behavior are proven by the focused tests of tasks 0008/0022/0044, not assumed. Upgrades to a new major/minor driver line require a new decision entry.

## ADR-0015 — Pin golang.org/x/text v0.44.0 for title search keys

- Date: 2026-11-11
- Status: Accepted
- Context: Task 0010 stores title search keys computed by Go: Unicode NFKC, full Unicode case fold, trim and collapse of Unicode whitespace (docs/domain-model.md). SQLite's built-in ASCII lower()/NOCASE is insufficient for JP/RU/romaji text; the standard library has no full case folding. golang.org/x/text v0.44.0 is the newest release at selection time and provides unicode/norm (NFKC) and cases.Fold (full case folding per Unicode CaseFolding.txt). It is pure Go, requires no CGO, and adds no new indirect dependencies to go.mod.
- Decision: Pin golang.org/x/text v0.44.0 as a direct dependency. Search keys are computed once in Go, stored with the title row, and matched with escaped SQL LIKE (percent/underscore literal); no FTS or vector search at personal-library scale.
- Consequences: Diacritics, kana, kanji, punctuation and ё are preserved in keys (no transliteration, no ё→е equivalence); romaji and localized titles are separate rows. Upgrading to a new x/text major line requires a new decision entry and a re-run of the normalization tests.

## ADR-0016 — Pin the React 19 / Vite 8 / TypeScript 5.9 frontend toolchain

- Date: 2026-11-13
- Status: Accepted
- Context: Task 0030 bootstraps `web/` (ADR-0009: React + TypeScript + Vite, Node as build tooling only, no production Node server). The pinned closure: react 19.3.1, react-dom 19.3.1, react-router-dom 7.18.5; typescript 5.9.3, vite 8.3.8, vitest 5.1.0, @vitejs/plugin-react 6.3.0, @testing-library/react 16.3.3, @testing-library/dom 10.4.2, @types/react 19.3.0, @types/react-dom 19.3.0, jsdom 31.0.1. All versions are the newest releases at selection time on Node 24.
- Decision: Pin exact versions in web/package.json (no ranges) with pnpm-lock.yaml as the full closure. TypeScript is pinned to the 5.9 line rather than the 7.x native compiler line because the task verification runs the classic `tsc --noEmit` CLI and the vitest/Vite plugin ecosystem targets the 5.x line; moving to the 7.x line requires a new decision entry. The dev server proxies `/api` to the loopback Go server; the production build is static and embedded by the Go binary in task 0047.
- Consequences: No global state framework, design system or e2e browser dependency in the MVP; tests are component-level (vitest + jsdom + Testing Library) and run via `pnpm --dir web test`. Upgrading any pinned major line requires a new decision entry and a re-run of the web build and tests.

## ADR-0017 — Pin github.com/modelcontextprotocol/go-sdk v1.7.0 for the MCP stdio adapter

- Date: 2026-11-14
- Status: Accepted
- Context: ADR-0003 and ADR-0009 make MCP a first-class transport over the same in-process application services, and docs/hermes.md defers protocol initialization, capabilities and framing to a maintained Go MCP SDK selected and pinned with an ADR dependency note in task 0037. github.com/modelcontextprotocol/go-sdk v1.7.0 is the official MCP Go SDK and the newest stable release at selection time (the v1.8.x line is pre-release only). It requires go >= 1.25; the module directive stays at 1.26.0 on the Go 1.27.1 toolchain. The direct dependency pulls one indirect closure: google/jsonschema-go (schema inference and validation for typed tool handlers), segmentio/encoding and segmentio/asm (JSON wire encoding), yosida95/uritemplate/v3 (resource templates), and golang.org/x/oauth2 plus golang.org/x/time (SDK auth/rate machinery the stdio adapter does not use).
- Decision: Pin github.com/modelcontextprotocol/go-sdk v1.7.0. The MCP transport is stdio only (newline-delimited JSON-RPC 2.0) through the SDK's IOTransport: the protocol output stream carries frames only, every SDK diagnostic goes to an slog logger bound to stderr, and no native remote listener is created. The bootstrap server registers no tools at all — no generic SQL, shell or file tool exists at any point — and domain tools arrive in tasks 0038-0041 as typed handlers that call the shared service (internal/app) and render outcomes through the adapter's Result/Failure translation, which keeps the transport-visible wire error object (code, message, details, retryable) shared by every adapter.
- Consequences: The SDK owns protocol framing and lifecycle, so its failure semantics are the contract: a syntactically broken stream line is a fatal transport error (the session ends and the process exits with a distinguishable non-zero code), a well-formed request for an unknown method is answered with a JSON-RPC method-not-found while the session continues, and a clean client EOF ends the session with exit zero. Protocol traffic never mutates the database because the transport owns no domain rules. Upgrading to a new SDK major/minor line requires a new decision entry and a re-run of the subprocess lifecycle tests.

## ADR-0018 — Pin @playwright/test v1.65.1 for the web E2E suite

- Date: 2026-11-15
- Status: Accepted
- Context: Task 0045 verifies the anime-source and duplicate workflows through the web UI in addition to the Go acceptance and MCP/HTTP parity tests. ADR-0009 keeps Node as build tooling only (no production Node server), and ADR-0012's security boundary accepts only the listener's own Host spelling and, on unsafe methods, only the loopback http origin (an explicit public origin must be https). The Vite dev proxy forwards the browser's `Host: 127.0.0.1:4173` to the API on 8787, which the boundary rejects, so the suite cannot drive the UI through the dev-server proxy. @playwright/test 1.65.1 is the newest release at selection time; its chromium headless shell is downloaded to the user cache by `playwright install chromium` without system packages (the sandbox has no sudo), and the launch smoke test passes.
- Decision: Pin @playwright/test 1.65.1 as a web devDependency (web/pnpm-lock.yaml carries the closure). The suite lives in web/e2e/library.spec.ts with a minimal web/playwright.config.ts: one worker, no retries, baseURL http://127.0.0.1:8787. The spec's beforeAll builds the Go binary and the web bundle, starts `media-core serve` on a private loopback port, and serves the built UI and the API from one browser-facing origin through an in-process proxy that rewrites the Host header (always) and Origin header (unsafe methods) to the listener's own spelling; it then seeds the Scenario A library through that same origin. The suite uses the seeded state across its three tests (Scenario A walkthrough, Scenario C quick-add walkthrough, wrong-secret rejection), and afterAll tears down the proxy, the API process and the temp state dir.
- Consequences: The E2E suite is a dev-only dependency; it never runs in the Go module build and adds nothing to the embedded production UI path of task 0047. The single-origin proxy mirrors the future embedded-UI topology (one host serves UI and API), so when task 0047 lands the proxy can be deleted and the baseURL pointed at the Go binary. Upgrading the Playwright major line requires a new decision entry, a fresh `playwright install chromium`, and a re-run of the suite.

## ADR-0019 — Hosted CI and a non-publishing release gate

- Date: 2026-11-16
- Status: Accepted
- Context: The post-implementation audit confirmed that no hosted CI, release gate, service units or scheduled backup existed. AGENTS.md discourages mandated hosted CI for discovery/MVP without a concrete reason; the operator explicitly requested CI and a separate release gate for this audit, which is that reason. The repository already requires Go, pnpm/Node build tooling and Python for static checks; no new application dependency is introduced.
- Decision: Add `.github/workflows/ci.yml` on push/PR with `contents: read` only, and `.github/workflows/release-gate.yml` on tags/manual dispatch with the same permission. Both pin third-party actions to major versions (`actions/checkout@v4`, `actions/setup-go@v5`, `pnpm/action-setup@v4`, `actions/setup-node@v4`, `actions/upload-artifact@v4`). CI runs build, vet, tests, race tests, shell syntax, structural checks, TypeScript, Vitest, Playwright and a local release build plus backup/restore smoke. The release gate re-runs verification, builds local artifacts with the pinned version and uploads them as workflow artifacts only: it never creates a tag, pushes, opens a GitHub Release, logs into a registry or uses secrets.
- Consequences: Hosted CI is now an explicit quality contract, not an implied deployment step. NAS deployment remains an operator action using the example units in `deploy/`, which carry no secrets and restrict writable paths. Release artifacts remain local until an operator deliberately publishes them outside this workflow.

## ADR-0020 — Explicit Trusted LAN mode on a private interface

- Date: 2026-11-23
- Status: Accepted target for task 0077; implementation/deployment evidence belongs to that task and to docs/progress.md.
- Context: A single-owner Web UI on a NAS should be reachable from other home devices without a reverse proxy, container, public HTTPS or per-application IPs. ADR-0012 intentionally kept the default loopback and made non-loopback require the allow-remote + https public-origin + TLS trio. Plain HTTP on a LAN is a distinct, narrower transport that must not weaken that default or that trio.
- Decision: Add an explicit `media-core serve --trusted-lan` mode. It binds one concrete RFC1918 IPv4 address or IPv6 ULA (`fc00::/7`) on an explicit port; wildcards, loopback, link-local, multicast, public, hostname and mapped addresses are rejected before binding. `--lan-host` accepts one lowercase DNS name without scheme, port or path and is recommended in `home.arpa`, never `.local`. The only accepted `Host` values are the listener IP and that name on the same port; the only accepted unsafe `Origin` values are their `http://` forms. Owner login, in-memory sessions, `HttpOnly`/`SameSite=Strict` cookies (deliberately not `Secure` on HTTP), exact CSRF, rate limiting, CSP, body limits and safe errors remain mandatory. Trusted LAN cannot be combined with `--allow-remote`, `--public-origin` or TLS files. DNS resolution, DHCP reservation and firewall policy stay operator responsibilities outside the binary. The web client mints UUID v4 request ids from `crypto.getRandomValues` because `crypto.randomUUID` is a secure-context API unavailable on HTTP LAN origins. The systemd example is an optional drop-in, leaving the loopback default unit unchanged.
- Consequences: `http://media.home.arpa:8787` works with the same Go binary and embedded UI on any permitted LAN device, with no external web server. HTTP exposes the owner secret, cookie and traffic to anyone on the LAN; it is not secure context and does not prove isolation from the internet, so firewall restriction and trusted-network-only operation are mandatory. Public HTTPS mode, other transports and the data schema are unchanged. No NAS deployment is claimed until a real smoke is performed there.

## ADR-0021 — Explicit dark-first UI and bounded cover discovery

- Date: 2026-11-24
- Status: Accepted target for task 0079; implementation evidence belongs to that task and to docs/progress.md.
- Context: The MVP UI is functional but flat, with a light-default stylesheet and a text-first library. A personal media library benefits from cover-first visual hierarchy, but the product runs on a resource-limited NAS with no Node runtime, no reverse proxy and strict security boundaries. Earlier ADR-0006/0008 require external metadata to be optional, explicit, provenance-bearing and never blocking local data; ADR-0004 forbids automatic merging on fuzzy title similarity. Provider research found usable public APIs for Kitsu, Shikimori, MangaDex and Open Library, each with different limits and attribution duties, and rules that exclude AniList and unofficial Jikan/HTML scraping.
- Decision: Make a dark graphite theme the default without consulting the OS, add a small shared component layer with visible focus states, and keep Library filters, debounce, cursors and retry intact while adding a grid/list view and URL-persisted state. Add four bounded provider adapters behind one allowlisted HTTPS client. Cover search is user-triggered; candidate images are downloaded server-side into short-lived opaque previews, and only a user's explicit selection copies bytes into a state-directory cache and writes a provenance row. Kind must match, no first match is auto-bound, ambiguous results are presented for choice, and a failed provider never blocks record creation. `--no-artwork` or `MEDIA_CORE_ARTWORK=false` disables discovery routes and omits saved artwork URLs (ADR-0022 also supersedes the backup exclusion below). Cached bytes are regenerable; SQLite rows are backed up. No new runtime dependency, image-processing library or background worker is introduced.
- Consequences: A saved library renders with zero outbound requests, staying compatible with loopback, Trusted LAN and remote HTTPS modes and the existing CSP. The cache cannot grow past 64 MiB through current writes and is pruned only for unreferenced files; selected bytes travel in archive v2 (ADR-0022). Provider terms remain an operator-visible document, and adding TMDB or another source requires a new ADR with its key/attribution obligations. The UI changes are presentational plus one provenance table; personal history, idempotency, state transitions and backup semantics are unchanged. The cache accounting, backup inclusion, `--no-artwork` behavior and candidate limits below are superseded by ADR-0022.

## ADR-0022 — Artwork reliability remediation: hard cache cap, keyset discovery, v2 backups and bounded requests

- Date: 2026-11-24
- Status: Accepted target for task 0080; implementation evidence belongs to that task and to docs/progress.md.
- Context: A post-0079 audit found that the cache cap excluded protected selections, so saved covers could grow without limit; Find missing covers used OFFSET over a set that shrinks when a cover is selected, skipping works; backup archives kept artwork metadata but not the selected image bytes, so restored libraries showed no covers and the missing-cover action ignored them; title matching compared a candidate's main title against its own alternative titles; Cancel did not abort the in-flight request and a slow provider batch could exceed the 30 second WriteTimeout; Shikimori manga was implemented but not registered; domain kinds such as manhwa and web_novel returned 400; a missing cache file rendered a broken image; `--no-artwork` still emitted image URLs; unselectable candidates were returned; and docs claimed a hard cap and a clean candidate list that the code did not provide.
- Decision: Enforce the cache cap over protected and unreferenced files together; evict only unreferenced files and reject a new selection with a stable `cache_full` error instead of deleting a user choice. Serialize cache operations. Replace OFFSET with an opaque keyset cursor over `(created_at, id)`. Publish archive format v2 that includes only the cache files referenced by active `work_artwork` rows, with per-file size and SHA-256 in the manifest, while restore still accepts v1. Score user titles against candidate main and alternative titles pairwise. Add an AbortController on the client, a per-request deadline below WriteTimeout, bounded provider/preview concurrency, and partial timeout results for find-missing. Register Shikimori manga, canonicalize domain kinds, reject embedded/special IP ranges, render a placeholder on image load failure, omit artwork views when `--no-artwork` is set, return only previewable candidates, and require score >= 0.9 for the confident label. Remove the dead WorkCard progress field and correct the documentation.
- Consequences: The cache is a real hard limit; a full cache asks the operator to remove a selection rather than silently losing one. Backup archives grow by the selected-cover cache and restore covers offline; current cache writes are capped at 64 MiB, while a legacy over-cap cache is archived as-is rather than dropping a selection. v1 archives remain readable. Discovery pages never skip works after a selection. A slow provider cannot hold a request past the budget, and Cancel stops the server work when the browser disconnects. No new dependency, background worker, image processor or NAS resource class is added. The archive format marker changes to `media-core-snapshot-v2`; older binaries refuse newer archives and v2-aware binaries read both.
