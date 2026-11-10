# Task 0046 - Verify progress and negative-impression history through both interfaces

State: todo
Owner: none
Mode: mvp
Lane: acceptance
Scope: acceptance
Risk: medium
Parallel: false
Depends on: 0034,0035,0036,0038,0039,0040,0041,0045
Tags: planned,m5

## Goal

Automate scenarios B/D with restart, correction and retained opinions via Web and MCP.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/product/requirements.md

## Files allowed to edit
- web/e2e/history.spec.ts
- tests/acceptance/history_test.go
- docs/progress.md

- .tasks/0046-verify-progress-and-negative-impression-history-through-both-interfaces.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Extend existing isolated browser/MCP harness, not a new framework
2. Run chapter 42→48, reopen and supersede an error; compare history/current state
3. Run dropped anime with negative note/rating and verify later search/activity

## Acceptance criteria

- Both interfaces observe identical effective and superseded values
- Negative opinion is retained without erasing library entry
- Unknown totals and independent publication/intention survive restart

## Invariants

- No requirement for network or AI

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./tests/acceptance -run \"Test(ScenarioB|ScenarioD)\" && pnpm --dir web exec playwright test e2e/history.spec.ts"}`

## Documentation updates
- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
