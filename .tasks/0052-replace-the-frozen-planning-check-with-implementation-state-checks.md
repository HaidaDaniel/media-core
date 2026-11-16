# Task 0052 - Replace the frozen planning check with implementation-state checks

State: done
Owner: opencode-media-fix-20261116
Mode: mvp
Lane: planning
Scope: docs,planning
Risk: medium
Parallel: false
Depends on: none
Tags: planned,m5,review-fix

## Goal

Make `scripts/check-planning.py` a truthful post-implementation check instead of asserting the obsolete 45-task todo snapshot.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/implementation-plan.md
- scripts/check-planning.py

## Files allowed to edit

- scripts/check-planning.py
- docs/implementation-plan.md
- docs/progress.md
- .tasks/0052-replace-the-frozen-planning-check-with-implementation-state-checks.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- .tasks/0001-start.md
- .tasks/0002-media-core-documentation-and-integration-contract.md
- .tasks/0003-document-adopted-repository.md

## Steps

1. Remove or gate the assertions that every 0005-0049 task is unclaimed todo at a planning boundary
2. Keep the durable checks: unique IDs, dependency existence and acyclicity, required sections, safe paths, verification declarations, inventory agreement and local link resolution
3. Expand the expected task set to include the 0050-0063 remediation tasks and drop the historic-baseline rewrite guard if it no longer applies
4. Add inventory rows for 0050-0063 in docs/implementation-plan.md and update its planning-verification boundary prose
5. Re-run the script and confirm it passes without hiding genuine structural regressions

## Acceptance criteria

- `python3 scripts/check-planning.py` exits 0 on the implemented repository
- Structural regressions (duplicate ID, broken dependency, missing section, broken local link, inventory mismatch) still fail the script
- The script docstring and its success output no longer claim a planning-only snapshot

## Invariants

- The check validates the current task graph; it never asserts that implemented work is still unstarted.

## Review questions

- Did simplifying the state assertions also remove the checks that protect task contract structure?

## Counterexample searches

- Temporarily break a dependency and a local link in a scratch copy and confirm the rewritten script still reports them.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"python3 scripts/check-planning.py"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Post-implementation review remediation; this task follows the documentation refresh so the inventory it validates is final.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
