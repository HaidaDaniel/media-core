# Implementation plan and requirement coverage

This is the inventory of **73 implementation tasks, 0005–0077**: the original 45 MVP tasks, 14 post-implementation remediation tasks, 6 post-implementation audit tasks, 6 final readiness/reliability tasks and one Trusted LAN deployment-readiness task. Tasks 0005–0076 are implemented and done; 0077 starts as todo. Existing 0001 canceled, 0002 done and 0003 todo remain unchanged. No task here authorizes production deployment.

The task Markdown is the execution contract: context, narrow allowed paths, steps, invariants, acceptance, negative cases, focused verification and review questions. Verification entries are JSON objects; each task records its own evidence when run. Use the installed APK 0.6.0 syntax.

## Task inventory and explicit dependency summary

Each row lists direct dependencies; numbering is not execution order (0049 is an early M1 task). The Initial state column records the state at registration, not the current lifecycle state; owners and evidence are managed by APK.

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
| [0044](../.tasks/0044-verify-multi-process-concurrency-and-migration-integrity.md) | Verify multi-process concurrency and migration integrity | 0012, 0015, 0023, 0037, 0043 | high | M5 | todo |
| [0045](../.tasks/0045-verify-anime-source-and-duplicate-workflows-through-web-and-mcp.md) | Verify anime-source and duplicate workflows through Web and MCP | 0031, 0032, 0033, 0034, 0035, 0038, 0039, 0040, 0041 | medium | M5 | todo |
| [0046](../.tasks/0046-verify-progress-and-negative-impression-history-through-both-interfaces.md) | Verify progress and negative-impression history through both interfaces | 0034, 0035, 0036, 0038, 0039, 0040, 0041, 0045 | medium | M5 | todo |
| [0047](../.tasks/0047-embed-web-assets-and-verify-complete-deployable-backup-round-trip.md) | Embed Web assets and verify complete deployable backup round trip | 0023, 0024, 0033, 0035, 0036, 0041, 0043, 0044, 0045, 0046, 0049 | high | M5 | todo |
| [0048](../.tasks/0048-document-hermes-connections-and-smoke-stdio-ssh-framing.md) | MCP connectivity for any agent: stdio and Streamable HTTP, documented and smoked | 0038, 0039, 0040, 0041, 0047 | high | M5 | todo |
| [0049](../.tasks/0049-provide-readable-library-export-and-read-only-doctor.md) | Provide readable library export and read-only doctor | 0013, 0015 | medium | M1 | todo |
| [0050](../.tasks/0050-exclude-playwright-e2e-specs-from-the-vitest-run.md) | Exclude Playwright e2e specs from the Vitest run | 0045 | low | M5 | todo |
| [0051](../.tasks/0051-restore-generated-agent-instruction-freshness.md) | Restore generated agent instruction freshness | 0063 | low | M5 | todo |
| [0052](../.tasks/0052-replace-the-frozen-planning-check-with-implementation-state-checks.md) | Replace the frozen planning check with implementation-state checks | none | medium | M5 | todo |
| [0053](../.tasks/0053-reserve-the-idempotency-key-on-identity-match-no-ops.md) | Reserve the idempotency key on identity-match no-ops | 0012 | high | M5 | todo |
| [0054](../.tasks/0054-bound-progress-decimal-parsing-against-int64-overflow.md) | Bound progress decimal parsing against int64 overflow | 0015 | high | M5 | todo |
| [0055](../.tasks/0055-verify-the-full-migration-ledger-in-checkschema.md) | Verify the full migration ledger in CheckSchema | 0009, 0023 | high | M5 | todo |
| [0056](../.tasks/0056-configure-origin-protection-for-streamable-http-mcp.md) | Configure Origin protection for Streamable HTTP MCP | 0048 | medium | M5 | todo |
| [0057](../.tasks/0057-stop-echoing-raw-internal-errors-to-clients.md) | Stop echoing raw internal errors to clients | 0024, 0037 | medium | M5 | todo |
| [0058](../.tasks/0058-make-the-release-manifest-and-cross-build-report-truthful.md) | Make the release manifest and cross-build report truthful | 0047 | medium | M5 | todo |
| [0059](../.tasks/0059-verify-restore-paths-before-creating-directories.md) | Verify restore paths before creating directories | 0023 | medium | M5 | todo |
| [0060](../.tasks/0060-freeze-the-quickadd-retry-payload-with-its-request-id.md) | Freeze the QuickAdd retry payload with its request id | 0050, 0032 | medium | M5 | todo |
| [0061](../.tasks/0061-align-graceful-shutdown-error-semantics.md) | Align graceful-shutdown error semantics | 0024, 0048 | low | M5 | todo |
| [0062](../.tasks/0062-refresh-cli-usage-text-and-doctor-capability-detection.md) | Refresh CLI usage text and doctor capability detection | 0049 | low | M5 | todo |
| [0063](../.tasks/0063-refresh-stale-product-and-progress-documentation.md) | Refresh stale product and progress documentation | 0050, 0052, 0053, 0054, 0055, 0056, 0057, 0058, 0059, 0060, 0061, 0062 | medium | M5 | todo |
| [0064](../.tasks/0064-preserve-uncheckpointed-wal-data-on-interrupted-restore.md) | Preserve uncheckpointed WAL data on interrupted restore | 0023, 0059 | high | M5 | todo |
| [0065](../.tasks/0065-prevent-late-session-inspection-from-clearing-the-csrf-token.md) | Prevent late session inspection from clearing the CSRF token | 0030, 0050 | medium | M5 | todo |
| [0066](../.tasks/0066-make-the-release-gate-verify-real-artifacts-and-manifest-json.md) | Make the release gate verify real artifacts and manifest.json | 0047, 0058 | medium | M5 | todo |
| [0067](../.tasks/0067-prepare-ci-hardened-service-units-and-a-backup-runbook.md) | Prepare CI, hardened service units and a backup runbook | 0047, 0058, 0066 | high | M5 | todo |
| [0068](../.tasks/0068-reconcile-operator-documentation-with-the-implemented-cli.md) | Reconcile operator documentation with the implemented CLI | 0064, 0065, 0066, 0067 | medium | M5 | todo |
| [0069](../.tasks/0069-disambiguate-the-playwright-note-text-assertion.md) | Disambiguate the Playwright note-text assertion | 0046, 0067 | medium | M5 | todo |
| [0070](../.tasks/0070-make-the-backup-unit-independent-of-path.md) | Make the backup unit independent of PATH | 0067 | high | M5 | todo |
| [0071](../.tasks/0071-add-safe-backup-retention-with-dry-run.md) | Add safe backup retention with dry-run | 0067, 0070 | high | M5 | todo |
| [0072](../.tasks/0072-verify-staged-assets-recursively-in-the-release-manifest-gate.md) | Verify staged assets recursively in the release manifest gate | 0066 | medium | M5 | todo |
| [0073](../.tasks/0073-fsync-the-staged-database-before-restore-publication.md) | Fsync the staged database before restore publication | 0064 | high | M5 | todo |
| [0074](../.tasks/0074-serialize-backup-archive-publication-by-name.md) | Serialize backup archive publication by name | 0022 | high | M5 | todo |
| [0075](../.tasks/0075-advance-session-generation-before-awaiting-login-or-logout.md) | Advance session generation before awaiting login or logout | 0065 | medium | M5 | todo |

| [0076](../.tasks/0076-isolate-systemd-unit-verification-from-the-runner.md) | Isolate systemd unit verification from the runner | 0070 | medium | M5 | todo |

| [0077](../.tasks/0077-trusted-lan-local-dns-access-and-deployment-readiness.md) | Trusted LAN, local DNS access and deployment readiness | none | high | M5 | todo |

| [0078](../.tasks/0078-publish-verified-linux-release-assets-from-version-tags.md) | Publish verified Linux release assets from version tags | none | high | M5 | todo |

| [0079](../.tasks/0079-dark-first-media-library-ui-and-provider-based-cover-discovery.md) | Dark-first media library UI and provider-based cover discovery | none | high | M5 | todo |

| [0080](../.tasks/0080-remediate-artwork-cache-pagination-backup-matching-and-request-boundary-defects.md) | Remediate artwork cache pagination backup matching and request-boundary defects | 0079 | high | M5 | todo |

| [0081](../.tasks/0081-upgrade-agenticprojectkit-to-v062.md) | Upgrade AgenticProjectKit to v0.6.2 | none | medium | M5 | todo |

| [0082](../.tasks/0082-fix-planning-contract-for-the-apk-upgrade-task.md) | Fix planning contract for the APK upgrade task | 0081 | low | M5 | todo |

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
- Task 0044 depends on 0043 so its all-migrations report covers the final MVP schema. Task 0043 explicitly permits the named ActivityPage/WorkPage and typed API integration files and tests, making new controls reachable.
- Task 0043 is a deliberately narrow integration vertical slice across already-built interfaces: pending binding/release list only. Task 0028/0041 bind existing annotation services; they do not implement domain persistence or whole HTTP/MCP interfaces. Task 0047 is artifact/complete-state validation, not frontend implementation. Split a task further if actual implementation exceeds these bounds; do not expand it into a backend/frontend omnibus.

## Requirement → design → owner → acceptance

Every requirement has implementation ownership. Scenarios A–G live in docs/product/requirements.md; these references are obligations, not passing test evidence.

| Requirement | Design document | Implementation task(s) | Acceptance scenario |
| --- | --- | --- | --- |
| R01 | [Architecture](architecture.md), [contracts](application-contract.md) | 0005, 0006, 0007, 0009, 0049, 0051, 0052, 0062, 0063, 0068, 0081, 0082 | G |
| R02 | [Domain](domain-model.md), [Web](web-ui.md) | 0010, 0011, 0012, 0013, 0026, 0032, 0039 | A, C |
| R03 | [Domain](domain-model.md), [contracts](application-contract.md) | 0010, 0013, 0016, 0018, 0031, 0035, 0038 | A, C |
| R04 | [Domain](domain-model.md) | 0011, 0014, 0016, 0027, 0034, 0040 | A, D, E |
| R05 | [Domain](domain-model.md), [contracts](application-contract.md) | 0015, 0027, 0034, 0040, 0046, 0054 | B |
| R06 | [Domain](domain-model.md), [Web](web-ui.md) | 0016, 0017, 0018, 0021, 0028, 0035, 0036, 0041, 0046 | A, D |
| R07 | [Domain](domain-model.md), [integration](integration-contract.md) | 0019, 0028, 0035, 0041 | A, C |
| R08 | [Domain](domain-model.md), [contracts](application-contract.md) | 0020, 0029, 0033, 0041, 0045 | A, C |
| R09 | [Domain](domain-model.md), [contracts](application-contract.md) | 0012, 0019, 0026, 0032, 0039, 0044, 0045, 0053 | C |
| R10 | [Web](web-ui.md), [contracts](application-contract.md) | 0024, 0025, 0026, 0027, 0028, 0029, 0030, 0031, 0032, 0033, 0034, 0035, 0036, 0045, 0046, 0050, 0060, 0065, 0069, 0075, 0079, 0080 | A, B, C, D, G |
| R11 | [Hermes](hermes.md) | 0037, 0038, 0039, 0040, 0041, 0048, 0056 | A, B, C, D, G |
| R12 | [Contracts](application-contract.md), [architecture](architecture.md) | 0007, 0012, 0025, 0026, 0027, 0028, 0029, 0038, 0039, 0040, 0041, 0045, 0046, 0057 | A, B, C, D |
| R13 | [Architecture](architecture.md) | 0006, 0024, 0030, 0047, 0061, 0076, 0077 | G |
| R14 | [Architecture](architecture.md), [requirements](product/requirements.md) | 0022, 0023, 0049, 0047, 0064, 0070, 0071, 0073, 0074, 0080 | F |
| R15 | [Architecture](architecture.md), [risks](engineering/risk-register.md) | 0008, 0009, 0022, 0023, 0044, 0055, 0059 | B, C, F |
| R16 | [Integration](integration-contract.md) | 0042, 0043 | E |
| R17 | [Architecture](architecture.md), [technology](engineering/tech-options.md) | 0030, 0047, 0048, 0058, 0066, 0067, 0072, 0078 | F, G |
| R18 | [Scope](scope.md), [integration](integration-contract.md), [contracts](application-contract.md) | 0012, 0019, 0020, 0032, 0041, 0045, 0048 | A, C, G |

## Planning verification boundary

`python3 scripts/check-planning.py` checks the current task graph: unique IDs, dependency existence/acyclicity, inventory agreement, required task sections and runnable-command declarations, existing context paths, safe repo-relative allowed paths, requirement/acceptance coverage and local Markdown links. It no longer asserts a frozen planning state; APK owns lifecycle, ownership and verification evidence. Static checks do not prove domain implementation or runtime security. Independent review is required only when a task's policy says so.

The inventory's Initial state column records registration state. Current states, owners, evidence and gates are read from `pnpm exec apk status --detail`; do not infer them from this document.
