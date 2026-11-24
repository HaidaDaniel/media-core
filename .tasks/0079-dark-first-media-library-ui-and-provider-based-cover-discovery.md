# Task 0079 - Dark-first media library UI and provider-based cover discovery

State: done
Owner: opencode-ui-20261124
Mode: mvp
Lane: implementation
Type: feature
Scope: web,artwork,network,storage,tests,docs
Risk: high
Parallel: false
Depends on: none
Tags: ui,covers

## Goal

Redesign the Web UI as a dark-first personal media library and add explicit, provider-based cover discovery with strict network, storage and NAS resource safety.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/architecture.md
- docs/scope.md
- docs/web-ui.md
- docs/decisions.md
- docs/engineering/deployment.md
- docs/engineering/backup-restore.md
- docs/engineering/configuration.md
- web/src/App.tsx
- web/src/styles.css
- web/src/library/LibraryPage.tsx
- web/src/library/WorkCardView.tsx
- web/src/api/client.ts
- internal/httpapi/routes.go
- internal/app/contracts.go
- internal/store/migrations.go

## Files allowed to edit

- web/src/**
- web/e2e/**
- internal/artwork/**
- internal/app/**
- internal/store/**
- internal/httpapi/**
- internal/config/**
- internal/runtime/**
- internal/cli/**
- tests/**
- docs/**
- docs/progress.md
- README.md
- .tasks/0079-dark-first-media-library-ui-and-provider-based-cover-discovery.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- .github/workflows/**
- deploy/**
- scripts/build-release.sh
- go.mod
- go.sum

## Steps

1. Verify provider docs, live endpoints, rate limits, terms and attribution; record findings and exclusions in a provider document and an ADR.
2. Add a dark-first CSS token layer and shared UI components (Button, Input, Select, Badge, Dialog, Card, Skeleton, EmptyState); keep no animation library and no runtime Node dependency.
3. Rework Shell into a desktop sidebar plus compact header (search, Quick Add) and make all screens use the shared system; keep nav, auth, CSRF and route behavior intact.
4. Rework Library to poster-grid by default with list alternative, media-type tabs, status/progress on cards, compact sort/filter panel, URL-persisted state and graceful empty/loading/error states; preserve debounce, cursor pagination and retry.
5. Add append-only SQLite migration 0014 for work/edition artwork records with provider, external id, source URL, attribution, user-selection flag and local cache reference; update migration pins, export registry and migration tests.
6. Add internal/artwork: provider interface, Kitsu, MangaDex, Open Library and Shikimori adapters, title/kind/reference matching, bounded HTTP client with host allowlist, HTTPS-only, no user URLs, redirect cap, timeouts, byte limits, MIME validation, rate limiting and backoff.
7. Add a state-dir image cache with per-file and total size caps, atomic writes and safe deletion; serve cached bytes through an authenticated local route with no outbound fetch on normal page render.
8. Wire explicit HTTP endpoints (artwork/providers, artwork/search, artwork/select, artwork/find-missing, artwork/{id}/image, artwork/cache/clear) behind session/CSRF; expose artwork view fields in WorkCard/Work/Edition contracts.
9. Wire Quick Add cover suggestion (never blocks record creation) and a Find missing covers action with user-controlled small batches, progress, errors and ambiguous results; never overwrite a user-selected image without explicit replacement.
10. Add unit, adapter-fixture, SSRF/oversize/malformed, UI, Playwright desktop/mobile, Trusted LAN, backup/restore and migration tests; run full local build and verification.
11. Produce screenshots, resource estimate, migration/rollback plan, upgrade instructions and GO/NO-GO; complete APK verification, independent review and gate.

## Acceptance criteria

- Dark theme is the default regardless of OS setting; graphite background, strong cover-first hierarchy, sidebar on desktop and usable layout down to 360px with visible focus states and semantic state colors.
- Shared components exist and are used across Library, Work, Edition, Quick Add, Activity and Login; empty, loading, error, conflict and unauthorized states are covered.
- Library keeps all existing filters, debounce, cursor pagination, retry and URL state, adds grid/list toggle, media-type tabs and status/progress on cards; no functionality is removed.
- Provider document records verified API docs, live checks, rate limits, terms and attribution for Kitsu, MangaDex, Open Library and Shikimori GraphQL; TMDB remains documented as optional; AniList, Jikan, HTML scraping and image search are explicitly excluded.
- Implemented providers are Kitsu (anime/manga), MangaDex (manga), Open Library (books) and Shikimori (anime/manga); no invented or fabricated cover data appears anywhere.
- Matching uses preferred, alternative, original-language, romanized titles, existing external references and kind; kind mismatch prevents anime/manga mixing; no automatic first-match binding; one confident result is a dismissible suggestion, several results show cover/title/kind/source for user choice, none leaves a correct fallback.
- Migration 0014 is append-only, preserves every existing row, passes migration and export-registry tests, and has a documented rollback path.
- Artwork records keep provider, external id, source URL, attribution, user-selection flag and local reference; cache lives under the state directory with size caps and can be deleted or rebuilt without losing user data.
- Network client is HTTPS-only, resolves only public IPs, uses allowlisted provider hosts, bounds redirects, bytes and dimensions, validates content type, rate-limits, backs off and never accepts arbitrary user URLs; SSRF, oversize and malformed-response tests pass.
- Quick Add search is bounded and optional; failure or slowness of a provider never blocks work creation; no background mass download runs on page open and Find missing covers works in small controlled batches with progress and no overwrite of user-selected art.
- Rendering a saved library performs no outbound requests; systemd CPU/RAM/I/O limits stay compatible; per-provider concurrency is bounded.
- Full Go and web tests, Playwright desktop/mobile, Trusted LAN smoke, backup/restore regression, migration tests and release build with embedded assets pass with recorded evidence.

## Correctness assumptions

- Provider endpoints and terms observed during this task remain available; adapters fail closed and never claim a cover when metadata is missing.
- Cached image bytes are regenerable; user selection and provenance live in SQLite and are part of backups.
- All artwork discovery is triggered by explicit user action, never by merely opening a page.

## Invariants

- No NAS deployment, NAS configuration change or hosted release from local work.
- No new runtime dependency, Node server, Docker, reverse proxy or heavy image-processing library.
- Personal progress, history, idempotency, state transitions and domain invariants are unchanged.
- No fabricated covers, artificial metadata or silent rewriting of existing records.
- No external URL from a user is fetched by the server.

## Required evidence

- Unit/adapter/SSRF/size tests, web unit and Playwright desktop/mobile results, Trusted LAN and backup/migration regressions, release build, screenshots, provider matrix and reviewed architecture/docs changes.

## Review questions

- Can the new UI remove an existing filter, retry or pagination behavior, or break keyboard access?
- Can any provider response result in an unchecked URL, redirect, content type, byte count or dimension?
- Can a cover be bound automatically, overwrite a user choice, or mix anime and manga?
- Can page rendering trigger network egress or a mass background job?
- Do migration, export, backup and restore preserve all pre-existing records and user-selected art?

## Counterexample searches

- Point a provider fixture at loopback/private IP, a userinfo URL, an oversized body, a decompression bomb and a redirect chain; require failure.
- Return a candidate with mismatched kind and near-equal title; require it to be rejected or shown only as ambiguous.
- Fail one provider while creating a work through Quick Add; require creation to still succeed.
- Open the library with a stale cache file; require no outbound request and a clean placeholder.
- Seed records before 0014 and upgrade; confirm titles, progress, notes and new artwork selection survive backup/restore.

## Verification

- `{"id":"go-build","type":"automated","required":true,"environment":"local","profile":"report","command":"go build ./..."}`
- `{"id":"go-vet","type":"automated","required":true,"environment":"local","profile":"report","command":"go vet ./..."}`
- `{"id":"go-tests","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./..."}`
- `{"id":"go-race","type":"automated","required":true,"environment":"local","profile":"report","command":"go test -race ./..."}`
- `{"id":"web-tsc","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec tsc --noEmit"}`
- `{"id":"web-vitest","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec vitest run"}`
- `{"id":"web-playwright","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec playwright test"}`
- `{"id":"release-build","type":"automated","required":true,"environment":"local","profile":"report","command":"bash scripts/build-release.sh"}`
- `{"id":"planning-links","type":"automated","required":true,"environment":"local","profile":"report","command":"python3 scripts/check-planning.py"}`
- `{"id":"apk-lint","type":"automated","required":true,"environment":"local","profile":"report","apkOperation":"lint"}`
- `{"id":"apk-doctor","type":"automated","required":true,"environment":"local","profile":"report","apkOperation":"doctor"}`

## Documentation updates

- Add docs/engineering/artwork-providers.md and an ADR in docs/decisions.md.
- Amend docs/scope.md, docs/architecture.md, docs/web-ui.md and configuration/backup docs for explicit artwork discovery and regenerable cache.
- Record lifecycle, screenshots, resource estimate, migration/rollback, upgrade steps and GO/NO-GO in docs/progress.md.

## Notes

- Milestone: M5. Dark-first library and explicit cover discovery on a dedicated branch. No NAS deploy, hosted release, tag or push is authorized by this task without a separate operator decision.
- Provider research date: 2026-11-24. Kitsu JSON:API at kitsu.io/kitsu.app /api/edge (no published fixed rate limit; 429 possible), MangaDex api.mangadex.org (~5 rps, credit required, no ads, images must be proxied), Open Library search.json + covers API (1 rps unidentified / 3 rps identified, courtesy attribution, no crawling), Shikimori GraphQL (5 rps / 90 rpm, descriptive User-Agent, prefer GraphQL for new posters), TMDB optional with API key and attribution notice.
