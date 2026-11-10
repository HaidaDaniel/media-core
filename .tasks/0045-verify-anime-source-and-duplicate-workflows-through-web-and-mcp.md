# Task 0045 - Verify anime-source and duplicate workflows through Web and MCP

State: todo
Owner: none
Mode: mvp
Lane: acceptance
Scope: acceptance
Risk: medium
Parallel: false
Depends on: 0031,0032,0033,0034,0035,0038,0039,0040,0041
Tags: planned,m5

## Goal

Automate scenarios A/C and DTO/error parity across real Web/MCP boundaries.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/product/requirements.md
- docs/application-contract.md

## Files allowed to edit

- web/package.json
- web/pnpm-lock.yaml
- web/playwright.config.ts
- web/e2e/library.spec.ts
- tests/acceptance/library_test.go
- tests/contracts/parity_test.go
- docs/decisions.md
- docs/progress.md

- .tasks/0045-verify-anime-source-and-duplicate-workflows-through-web-and-mcp.md

- .tasks/0045-verify-anime-source-and-duplicate-workflows-through-web-and-mcp.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Add/pin browser test tooling if not present and document dependency
2. Create isolated fixture servers and MCP subprocess flows for A/C
3. Compare normalized DTOs and assert duplicate/cycle/conflict failures, proposal labels and responsive navigation

## Acceptance criteria

- Completed anime and unread saved manga remain connected with note/tag/source
- Alias lookup and exact/candidate add avoid silent duplicate/merge
- No external service, actual Hermes install or real DB required

## Invariants

- Tests assert user-visible semantics, not mirrored implementation

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./tests/acceptance ./tests/contracts -run \"Test(ScenarioA|ScenarioC|Parity)\" && pnpm --dir web exec playwright test e2e/library.spec.ts"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M5. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.

