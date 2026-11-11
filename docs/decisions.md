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
