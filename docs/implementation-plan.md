# Implementation plan and requirement coverage

This is the inventory of **45 unstarted implementation tasks, 0005–0049**. Task 0004 owns this documentation/planning phase only. Existing 0001 canceled, 0002 done and 0003 todo remain unchanged. No task here certifies an implemented feature or authorizes production deployment.

The task Markdown is the execution contract: context, narrow allowed paths, steps, invariants, acceptance, negative cases, focused future verification and review questions. All task checks are **future commands** which their task or prerequisite must create/make runnable; none is claimed to have run in this phase. Use installed APK 0.5.2 syntax; gated Verification entries are JSON objects supported by that version.

## Task inventory and explicit dependency summary

Each row lists direct dependencies; numbering is not execution order (0049 is an early M1 task). All initial states are todo, owner none.

| Task | Title | Depends on | Risk | Milestone | Initial state |
| --- | --- | --- | --- | --- | --- |
| [0005](../.tasks/0005-bootstrap-go-executable-and-build-identity.md) | Bootstrap Go executable and build identity | 0004 | low | M1 | todo |
| [0006](../.tasks/0006-resolve-explicit-configuration-and-local-state-paths.md) | Resolve explicit configuration and local state paths | 0005 | medium | M1 | todo |
| [0007](../.tasks/0007-freeze-typed-application-dtos-errors-and-contract-fixtures.md) | Freeze typed application DTOs errors and contract fixtures | 0005 | medium | M1 | todo |
| [0008](../.tasks/0008-open-sqlite-with-bounded-connections-and-maintenance-locks.md) | Open SQLite with bounded connections and maintenance locks | 0006 | high | M1 | todo |
| [0009](../.tasks/0009-run-embedded-transactional-migrations-under-exclusive-maintenance.md) | Run embedded transactional migrations under exclusive maintenance | 0008, 0007 | high | M1 | todo |
| [0010](../.tasks/0010-persist-works-and-sourced-multilingual-titles.md) | Persist works and sourced multilingual titles | 0009, 0007 | medium | M1 | todo |
| [0011](../.tasks/0011-persist-edition-lines-and-sourced-publication-observations.md) | Persist edition lines and sourced publication observations | 0010 | medium | M1 | todo |
| [0012](../.tasks/0012-create-library-entries-with-explicit-duplicate-resolution.md) | Create library entries with explicit duplicate resolution | 0011, 0007 | high | M1 | todo |
| [0013](../.tasks/0013-search-and-retrieve-the-persistent-library-through-cli.md) | Search and retrieve the persistent library through CLI | 0012 | medium | M1 | todo |
| [0014](../.tasks/0014-record-personal-consumption-status-with-recoverable-history.md) | Record personal consumption status with recoverable history | 0013 | medium | M1 | todo |
| [0015](../.tasks/0015-record-edition-progress-by-unit-with-correction-history.md) | Record edition progress by unit with correction history | 0012, 0014 | medium | M1 | todo |
| [0016](../.tasks/0016-store-intentions-and-simple-audited-tag-assignments.md) | Store intentions and simple audited tag assignments | 0013, 0014, 0015 | medium | M4 | todo |
| [0017](../.tasks/0017-add-plain-text-notes-with-immutable-revisions.md) | Add plain-text notes with immutable revisions | 0012, 0016 | medium | M4 | todo |
| [0018](../.tasks/0018-add-rating-observations-and-rating-search-filters.md) | Add rating observations and rating search filters | 0013, 0017 | medium | M4 | todo |
| [0019](../.tasks/0019-attach-multiple-sourced-external-references-safely.md) | Attach multiple sourced external references safely | 0012, 0018 | medium | M4 | todo |
| [0020](../.tasks/0020-validate-explicit-relation-graph-and-original-source-claims.md) | Validate explicit relation graph and original-source claims | 0011, 0012, 0019 | high | M4 | todo |
| [0021](../.tasks/0021-project-recent-personal-activity-and-complete-history-queries.md) | Project recent personal activity and complete history queries | 0014, 0015, 0016, 0017, 0018, 0019, 0020 | medium | M4 | todo |
| [0022](../.tasks/0022-create-consistent-sqlite-snapshot-backups-with-manifests.md) | Create consistent SQLite snapshot backups with manifests | 0009, 0015, 0012 | high | M1 | todo |
| [0023](../.tasks/0023-restore-validated-backups-under-exclusive-maintenance.md) | Restore validated backups under exclusive maintenance | 0022 | high | M1 | todo |
| [0024](../.tasks/0024-secure-http-lifecycle-and-single-owner-session-boundary.md) | Secure HTTP lifecycle and single-owner session boundary | 0006, 0007, 0009 | high | M2 | todo |
| [0025](../.tasks/0025-expose-catalog-search-and-detail-http-reads.md) | Expose catalog search and detail HTTP reads | 0024, 0013 | medium | M2 | todo |
| [0026](../.tasks/0026-expose-quick-add-and-catalog-correction-http-writes.md) | Expose quick-add and catalog correction HTTP writes | 0024, 0012, 0013 | medium | M2 | todo |
| [0027](../.tasks/0027-expose-progress-consumption-and-history-http-routes.md) | Expose progress consumption and history HTTP routes | 0024, 0014, 0015 | medium | M2 | todo |
| [0028](../.tasks/0028-expose-personal-annotation-http-adapter-group.md) | Expose personal annotation HTTP adapter group | 0024, 0016, 0017, 0018, 0019 | medium | M4 | todo |
| [0029](../.tasks/0029-expose-relation-origin-and-recent-activity-http-reads-and-writes.md) | Expose relation origin and recent-activity HTTP reads and writes | 0024, 0020, 0021 | medium | M4 | todo |
| [0030](../.tasks/0030-bootstrap-react-shell-typed-api-client-and-login.md) | Bootstrap React shell typed API client and login | 0007, 0024 | medium | M2 | todo |
| [0031](../.tasks/0031-build-library-list-search-filters-and-counts.md) | Build library list search filters and counts | 0030, 0025, 0014 | medium | M2 | todo |
| [0032](../.tasks/0032-build-manual-quick-add-and-duplicate-choice-screen.md) | Build manual quick-add and duplicate choice screen | 0030, 0026 | medium | M2 | todo |
| [0033](../.tasks/0033-build-work-details-and-explicit-related-media-navigation.md) | Build work details and explicit related-media navigation | 0030, 0025, 0029, 0028 | medium | M4 | todo |
| [0034](../.tasks/0034-build-edition-details-with-progress-status-and-history-controls.md) | Build edition details with progress status and history controls | 0030, 0025, 0026, 0027 | medium | M2 | todo |
| [0035](../.tasks/0035-build-edition-personal-panels-for-notes-ratings-intentions-and-sources.md) | Build edition personal panels for notes ratings intentions and sources | 0034, 0028, 0031 | medium | M4 | todo |
| [0036](../.tasks/0036-build-recent-activity-and-history-screen.md) | Build recent-activity and history screen | 0030, 0029 | low | M4 | todo |
| [0037](../.tasks/0037-bootstrap-mcp-stdio-and-shared-service-adapter-boundary.md) | Bootstrap MCP stdio and shared-service adapter boundary | 0005, 0006, 0007, 0009 | medium | M3 | todo |
| [0038](../.tasks/0038-expose-typed-mcp-search-and-catalog-read-tools.md) | Expose typed MCP search and catalog read tools | 0037, 0013 | medium | M3 | todo |
| [0039](../.tasks/0039-expose-typed-mcp-addition-and-catalog-correction-tools.md) | Expose typed MCP addition and catalog correction tools | 0037, 0012, 0013 | medium | M3 | todo |
| [0040](../.tasks/0040-expose-typed-mcp-progress-status-and-history-tools.md) | Expose typed MCP progress status and history tools | 0037, 0014, 0015 | medium | M3 | todo |
| [0041](../.tasks/0041-expose-mcp-personal-annotations-and-confirmed-relation-tools.md) | Expose MCP personal annotations and confirmed relation tools | 0037, 0016, 0017, 0018, 0019, 0020, 0021 | medium | M4 | todo |
| [0042](../.tasks/0042-stage-immutable-release-watcher-events-idempotently.md) | Stage immutable Release Watcher events idempotently | 0009, 0012, 0020 | high | M5 | todo |
| [0043](../.tasks/0043-bind-imported-releases-and-expose-release-history-across-interfaces.md) | Bind imported releases and expose release history across interfaces | 0042, 0019, 0021, 0024, 0037, 0036, 0033 | high | M5 | todo |
| [0044](../.tasks/0044-verify-multi-process-concurrency-and-migration-integrity.md) | Verify multi-process concurrency and migration integrity | 0012, 0015, 0023, 0037 | high | M5 | todo |
| [0045](../.tasks/0045-verify-anime-source-and-duplicate-workflows-through-web-and-mcp.md) | Verify anime-source and duplicate workflows through Web and MCP | 0031, 0032, 0033, 0034, 0035, 0038, 0039, 0040, 0041 | medium | M5 | todo |
| [0046](../.tasks/0046-verify-progress-and-negative-impression-history-through-both-interfaces.md) | Verify progress and negative-impression history through both interfaces | 0034, 0035, 0036, 0038, 0039, 0040, 0041, 0045 | medium | M5 | todo |
| [0047](../.tasks/0047-embed-web-assets-and-verify-complete-deployable-backup-round-trip.md) | Embed Web assets and verify complete deployable backup round trip | 0023, 0024, 0033, 0035, 0036, 0041, 0043, 0044, 0045, 0046, 0049 | high | M5 | todo |
| [0048](../.tasks/0048-document-hermes-connections-and-smoke-stdio-ssh-framing.md) | Document Hermes connections and smoke stdio SSH framing | 0038, 0039, 0040, 0041, 0047 | medium | M5 | todo |
| [0049](../.tasks/0049-provide-readable-library-export-and-read-only-doctor.md) | Provide readable library export and read-only doctor | 0013, 0015 | medium | M1 | todo |

## Execution path and observable increments

First five recommended implementation tasks: **0005 → 0006 → 0007 → 0008 → 0009**. After 0005, 0006 and 0007 may run in separate worktrees. Task 0004 must pass review/gate and be done before 0005 is claimable. Do not start 0003 implicitly or mark it done as part of this plan.

A dependency-critical trunk (no time estimates are implied) is **0005 → 0006 → 0008 → 0009 → 0010 → 0011 → 0012 → 0013** with the 0007 contract prerequisite. It yields the first small useful **CLI add/search/show** application. Add **0014 → 0015 → 0022 → 0023**, plus 0049, for the complete M1 persistent/progress/backup milestone.

The Web branch is **0024 → 0025/0026/0027**, plus **0030 → 0031/0032/0034**, with each route's domain prerequisites. It yields M2: authenticated manual add, search, edition card and progress/status (Vite dev tooling initially; embedded production artifact in 0047). Advanced annotation filters and related work details land in M4, not on this first-use path.

The Hermes branch is **0037 → 0038/0039/0040**, with the same 0012–0015 services. At the intersection of M2 and M3 both interfaces support the same basic library operations. They may arrive in either order; neither is a reason to defer the other. **M4 adds 0016–0021, 0028–0029, 0033, 0035–0036, 0041** for complete practical notes/ratings/intentions/sources/relations. Both Web and MCP support scenario A at M4. **M5 adds 0042–0048** for releases, cross-interface acceptance, comprehensive maintenance safety and a distributable one-binary MVP.

M1 does not require M4 records. Its restore fixture covers available works/editions/progress/request keys. Task 0047 extends scenario F and export to all later annotation/relationship/import tables. Tasks 0045/0046 create real-browser and MCP acceptance, rather than treating component fixtures as proof of complete workflows. M5 is not feature-complete until both interfaces and all scenarios pass.

## Parallel work and merge boundaries

- After 0009, HTTP security/lifecycle 0024 and MCP bootstrap 0037 can progress alongside domain persistence. After 0024, UI shell 0030 can use the frozen 0007 fixtures while catalog routes/services finish.
- After 0013–0015, HTTP read/add/reading adapters and MCP read/add/reading adapters are independent of each other; backup/export can proceed independently. UI add and edition-reading screens depend only on their concrete APIs, not on the whole domain.
- Notes, ratings, tags and relations are separate tasks but their **numbered SQL migration commits integrate in order**: 0010 → 0011 → 0012 → 0014 → 0015 → 0016 → 0017 → 0018 → 0019 → 0020 → 0042 → 0043. Explicit prerequisites encode this intentional serialization. It avoids unsupported out-of-order migrations without building a migration dependency engine. Read/adapters/UI are free to branch from completed capabilities.
- Parallel=true means eligible work in isolated Git worktrees, never multiple mutable tasks in one checkout. Runtime dispatch, route/tool registries, go.mod/go.sum, web routes/styles and docs/progress.md are known merge points. Keep wiring changes small and integrate deliberately; do not absorb another task's dirty files. A parallel flag is not a resource reservation.
- Task 0043 is a deliberately narrow integration vertical slice across already-built interfaces: pending binding/release list only. Task 0028/0041 bind existing annotation services; they do not implement domain persistence or whole HTTP/MCP interfaces. Task 0047 is artifact/complete-state validation, not frontend implementation. Split a task further if actual implementation exceeds these bounds; do not expand it into a backend/frontend omnibus.

## Requirement → design → owner → acceptance

Every requirement has implementation ownership. Scenarios A–G live in docs/product/requirements.md; these references are obligations, not passing test evidence.

| Requirement | Design document | Implementation task(s) | Acceptance scenario |
| --- | --- | --- | --- |
| R01 | [Architecture](architecture.md), [contracts](application-contract.md) | 0005, 0006, 0007, 0009, 0049 | G |
| R02 | [Domain](domain-model.md), [Web](web-ui.md) | 0010, 0011, 0012, 0013, 0026, 0032, 0039 | A, C |
| R03 | [Domain](domain-model.md), [contracts](application-contract.md) | 0010, 0013, 0016, 0018, 0031, 0035, 0038 | A, C |
| R04 | [Domain](domain-model.md) | 0011, 0014, 0016, 0027, 0034, 0040 | A, D, E |
| R05 | [Domain](domain-model.md), [contracts](application-contract.md) | 0015, 0027, 0034, 0040, 0046 | B |
| R06 | [Domain](domain-model.md), [Web](web-ui.md) | 0016, 0017, 0018, 0021, 0028, 0035, 0036, 0041, 0046 | A, D |
| R07 | [Domain](domain-model.md), [integration](integration-contract.md) | 0019, 0028, 0035, 0041 | A, C |
| R08 | [Domain](domain-model.md), [contracts](application-contract.md) | 0020, 0029, 0033, 0041, 0045 | A, C |
| R09 | [Domain](domain-model.md), [contracts](application-contract.md) | 0012, 0019, 0026, 0032, 0039, 0044, 0045 | C |
| R10 | [Web](web-ui.md), [contracts](application-contract.md) | 0024, 0025, 0026, 0027, 0028, 0029, 0030, 0031, 0032, 0033, 0034, 0035, 0036, 0045, 0046 | A, B, C, D, G |
| R11 | [Hermes](hermes.md) | 0037, 0038, 0039, 0040, 0041, 0048 | A, B, C, D, G |
| R12 | [Contracts](application-contract.md), [architecture](architecture.md) | 0007, 0012, 0025, 0026, 0027, 0028, 0029, 0038, 0039, 0040, 0041, 0045, 0046 | A, B, C, D |
| R13 | [Architecture](architecture.md) | 0006, 0024, 0030, 0047 | G |
| R14 | [Architecture](architecture.md), [requirements](product/requirements.md) | 0022, 0023, 0049, 0047 | F |
| R15 | [Architecture](architecture.md), [risks](engineering/risk-register.md) | 0008, 0009, 0022, 0023, 0044 | B, C, F |
| R16 | [Integration](integration-contract.md) | 0042, 0043 | E |
| R17 | [Architecture](architecture.md), [technology](engineering/tech-options.md) | 0030, 0047, 0048 | F, G |
| R18 | [Scope](scope.md), [integration](integration-contract.md), [contracts](application-contract.md) | 0012, 0019, 0020, 0032, 0041, 0045, 0048 | A, C, G |

## Planning verification boundary

`python3 scripts/check-planning.py` checks the frozen planning snapshot: unique IDs, all 45 future tasks unclaimed todo, dependency existence/acyclicity, dependency table agreement, required task sections and runnable-command declarations, existing context paths, safe repo-relative allowed paths, requirement/acceptance coverage and local Markdown links. APK lint additionally parses the actual installed contract syntax/policy/path rules; doctor/sync check tooling health. Static checks do not prove domain implementation or runtime security. Independent review must inspect the whole candidate diff for semantic gaps, task size and interface parity.

This phase never runs future Go/browser commands, claims future tasks, installs runtime dependencies or deploys. After implementation starts, the frozen todo-state check must be deliberately revised before being reused as an ongoing implementation check. Do not treat its planning-only state expectations as a permanent task lifecycle policy.
