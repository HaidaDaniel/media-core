# Task 0080 - Remediate artwork cache pagination backup matching and request-boundary defects

State: doing
Owner: opencode-primary
Mode: mvp
Lane: implementation
Type: bugfix
Scope: artwork,backup,storage,web,network,tests,docs
Risk: high
Parallel: false
Depends on: 0079
Tags: artwork,remediation,cache,backup,p1,p2

## Goal

Fix the verified P1/P2 artwork reliability defects from task 0079: enforce a hard cache cap without losing user selections, make missing-cover pagination stable, restore selected cover bytes from backups, correct title matching, bound and cancel artwork requests, and align the UI, API and docs with the implemented behavior.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/decisions.md
- docs/progress.md
- docs/engineering/artwork-providers.md
- docs/engineering/backup-restore.md
- .tasks/0079-dark-first-media-library-ui-and-provider-based-cover-discovery.md
- internal/artwork/cache.go
- internal/artwork/client.go
- internal/artwork/provider.go
- internal/artwork/service.go
- internal/artwork/adapters.go
- internal/backup/archive.go
- internal/backup/restore.go
- internal/store/artwork.go
- internal/httpapi/artwork.go
- internal/app/library.go
- web/src/artwork/FindMissingCovers.tsx
- web/src/artwork/CoverPicker.tsx
- web/src/artwork/CoverImage.tsx
- web/src/artwork/artworkApi.ts
- web/src/api/client.ts
- web/src/library/WorkCardView.tsx
- web/src/library/libraryApi.ts

## Files allowed to edit

- .tasks/0080-remediate-artwork-cache-pagination-backup-matching-and-request-boundary-defects.md
- internal/artwork/**
- internal/backup/**
- internal/store/**
- internal/app/**
- internal/httpapi/**
- internal/runtime/**
- internal/config/**
- internal/cli/**
- web/src/**
- web/e2e/**
- tests/**
- scripts/test_backup_retention.py
- docs/**
- docs/progress.md
- README.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- .github/workflows/**
- deploy/**
- scripts/build-release.sh
- go.mod
- go.sum
- web/package.json
- web/pnpm-lock.yaml

## Steps

1. Add failing offline reproducers for every P1/P2 item before changing behavior.
2. Cache: count protected files in the hard cap; serialize operations; evict only unreferenced files; reject an over-cap selection with `cache_full`; never delete a selection; test concurrent Put/Clear.
3. Replace Find-missing OFFSET with an opaque keyset cursor over (created_at, id); update store, service, wire types and UI; prove no skip or duplicate after a selection.
4. Archive v2: include cache files referenced by active `work_artwork` rows with per-file size and SHA-256; record omitted missing refs; restore validates and installs bytes before the atomic DB publish; keep v1 restore; update CLI, retention and tests.
5. Match user titles only against candidate main/alternative titles pairwise; exact normalized match wins; no candidate-title self-comparison; add inflation and true-alternative tests.
6. Add client AbortController cancellation; apply a 25 second server budget; bound provider/preview concurrency to three; return partial find-missing items with `timeout`; add slow-fixture tests.
7. Reject IPv4-compatible, 6to4, site-local and CGNAT ranges; register Shikimori manga; canonicalize manhwa/manhua to manga and web_novel/light_novel to book; stable empty result for unsupported kinds.
8. Add CoverImage onError placeholder; omit artwork views for --no-artwork; return only previewable candidates; require score >= 0.9 for the confident label; remove dead WorkCard progress.
9. Correct docs/progress claims, add ADRs for cache-full and archive v2, update backup/provider/web docs, then run full verification and independent review.

## Acceptance criteria

- Protected selections are never evicted; cache bytes including protected stay at or below the cap for accepted writes; an over-cap selection fails with a stable error and existing selections stay readable.
- Seven missing works, page size three: selecting a cover on page one then requesting the next cursor yields every remaining work exactly once.
- A v2 backup with a selected cover restores the row and exact bytes; v1 still restores; hashes match; cache clear works after restore; no secret or orphan file enters the archive.
- An unrelated user title cannot match through a candidate's own alternative; a genuine alternative-title match scores at least 0.9; kind mismatches do not match.
- Cancel aborts the browser request and stops server work; slow-provider requests return within the budget below 30 seconds, with partial timeout results.
- Shikimori lists anime and manga; manhwa, manhua, web_novel and light_novel no longer return 400; unsupported kinds return a stable empty result.
- Missing or cleared bytes render the placeholder; --no-artwork emits no dead URLs; every returned candidate is selectable; the confident label needs score >= 0.9.
- Dead WorkCard progress is gone; docs/progress records push status, the hard cap and candidate behavior accurately; ADRs document cache-full and archive v2.
- Go build, vet, test, race, TypeScript, Vitest, Playwright, release build, check-planning, APK lint and doctor pass on the committed candidate.
- A different registered identity passes independent review after focused counterexamples; no merge, tag, release or NAS deployment occurs.

## Correctness assumptions

- Backup v2 may include up to the 64 MiB cache cap of referenced cover images and the retention disk budget is acceptable.
- Keyset pagination defines a stable snapshot; works created after the cursor appear after Start over.
- A per-request budget of about 25 seconds fits the 30 second server WriteTimeout.
- A full cache rejects new selections instead of deleting user-selected images.
- Provider image URLs are not persisted; backup inclusion is the recovery path.

## Invariants

- Dark-first UI, Trusted LAN behavior, auth and CSRF, and NAS resource limits stay intact.
- No new runtime dependency, Node server, Docker, reverse proxy, background worker or heavy image processing.
- No fabricated covers or metadata; no user-supplied URL is fetched; HTTPS allowlist and SSRF protections remain.
- Existing personal progress, history, imports, owner secrets and backup exclusions are untouched.
- No merge, tag, release or NAS deployment without separate operator permission.

## Required evidence

- Failing-before and passing-after tests for each P1/P2 item.
- Backup v2 round-trip proof with exact restored bytes.
- Focused cancellation and deadline proof with slow fixtures.
- Full candidate-bound verification and independent review.

## Review questions

- Can a full cache delete or hide a user-selected cover?
- Can selecting a cover between pages skip or duplicate a missing work?
- Can a tampered or partial v2 archive break restore or leave a half-restored state?
- Can a candidate still inflate its score with its own titles?
- Can Cancel leave a provider request running past the budget?
- Can any domain kind still produce a 400 or a wrong-provider match?
- Can a missing cache file still render a broken image or a dead URL?

## Counterexample searches

- Fill the cache with protected files and attempt another selection; require no deletion of selections and a stable error.
- Select the first page item then fetch the next cursor; require no skip.
- Restore a v1 archive and a v2 archive with a tampered artwork hash; require v1 restore to work and v2 tamper to fail before publication.
- Feed a candidate whose alternative equals its main title with an unrelated user title; require no match.
- Make a provider fixture sleep past the budget and press Cancel; require prompt abort and no server work after the deadline.
- Search manhwa, web_novel and game; require canonical or stable no-provider results.
- Open the library with a cleared cache and with --no-artwork; require placeholders and no dead image requests.

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

- docs/decisions.md: ADR for backup archive v2 and artwork cache inclusion
- docs/decisions.md: ADR for cache-full behavior and matching rules
- docs/engineering/backup-restore.md: archive layout, sizes, v1/v2 compatibility, restore semantics
- docs/engineering/artwork-providers.md: Shikimori manga, canonical kind mapping, rate and timeout budgets
- docs/engineering/configuration.md and deployment.md: --no-artwork behavior and backup size expectations
- docs/web-ui.md: placeholder behavior, candidate selection and Cancel semantics
- docs/progress.md: reproduction results, remediation evidence, corrected claims and GO/NO-GO
- README.md: user-visible backup and cover recovery changes if commands change

## Notes

- Continue on feature/dark-ui-covers-0079 at 4b1bce8; the branch is already pushed; do not merge it to main.
- Reproduction before fix is mandatory for each P1/P2 item; keep tests offline with fixtures and no live provider calls.
- Additional verified 0079 defects are included: Shikimori manga registration, kind canonicalization, CoverImage fallback, --no-artwork dead URLs, candidate selectability, docs overclaims and dead progress code.
- P1 items are the cache hard cap and missing-cover pagination; P2 items are backup cover recovery, title matching and cancellation/timeouts.
- No new dependency, no background worker, no NAS deployment, no tag and no release.
- Report commit SHAs, test results and a merge GO/NO-GO for main at handoff.
- Milestone: M5.
- Cache-full policy: evict unreferenced files first; reject a new selection with `cache_full` when protected plus new exceeds the cap; never delete a user selection.
- Backup v2: archive `media-core.db`, `manifest.json` and `artwork-cache/` files referenced by active rows with per-file size and SHA-256; missing files are recorded, not invented; restore installs validated bytes before the atomic DB publish; v1 archives still restore.
- Pagination uses an opaque keyset over `(created_at, id)`; matching scores user titles against candidate main/alternative titles pairwise; the request budget is 25 seconds with bounded concurrency and partial timeout results.
- Pre-fix reproduction at 4b1bce8: 6 protected images held 3582 bytes over a 1194-byte cap; an OFFSET page skipped work 3 of 7 after a selection; self-alternative scored 1 while a true alternative-title match was lost; the web client had no AbortController; the acceptance test asserted the cache stayed outside archives.
