# Roadmap

Implementation milestones are observable capability boundaries, not approval or completion inferred from documents. M2 and M3 may proceed in parallel after shared contracts; both are required for MVP. See the [45-task inventory/dependency graph/coverage](implementation-plan.md).

| Milestone | Tasks (including their explicit prerequisites) | Observable exit |
| --- | --- | --- |
| M0 — Product and contracts specified | 0004 | Coherent docs, superseding ADRs, validated task graph, fresh-context review and APK gate. Operator's product instruction supplies the target; no invented human acceptance is recorded. |
| M1 — Persistent local library | 0005–0015, 0022–0023, 0049 | CLI manually adds/searches work+edition, records progress/status, reopens, exports and restores its available state. Earliest useful CLI after 0013. |
| M2 — First usable Web UI | 0024–0027, 0030–0032, 0034 | Authenticated add/duplicate decision/search/open edition/update status/progress in responsive UI. Initial dev server is tooling; production embedding in M5. |
| M3 — First usable Hermes/MCP | 0037–0040 | Same basic operations over typed stdio tools without HTTP. Can run alongside M2, not after the whole product. |
| M4 — Practical related-media library | 0016–0021, 0028–0029, 0033, 0035–0036, 0041 | Completed anime → saved unread manga, note/rating/tag/source, confirmed relation/origin and retained corrections through both interfaces. |
| M5 — Integrated distributable MVP | 0042–0048 | Version 1.0 import/replay/binding, cross-interface A–F workflows plus G security/offline checks, full-state backup/restore, embedded binary and documented secure local/NAS arrangements. No actual deployment implied. |

A small useful application exists before all tasks finish: CLI at 0013, first browser workflow at M2, matching Hermes workflow at M3. Full source relationships and annotations land at M4; imported release workflows and artifact packaging complete M5. Task numbers are identifiers, not the execution sequence: 0049 belongs early in M1.

Foundation order: 0005, then 0006/0007, 0008, 0009. Ordered migrations deliberately serialize domain integration; HTTP/MCP/UI branches and backup/export branch at their named dependencies. Use isolated worktrees for parallel mutable work, never two claimed tasks in one checkout.

Future after MVP: optional provider proposals/covers, native remote MCP if SSH is unsuitable, content/asset references beyond URLs, translation-project adapters and device delivery. No OCR/inpainting, content acquisition, rendering, translation execution or unused plugin framework is scheduled now.

Existing task 0003 is an unstarted adoption follow-up, preserved rather than silently completed. Its overlapping generic documentation objective is now substantially addressed by 0004, but a later owner/operator should explicitly reconcile/cancel it if appropriate; no false completion is recorded here.
