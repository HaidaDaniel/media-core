# Task 0050 - Exclude Playwright e2e specs from the Vitest run

State: done
Owner: opencode-media-fix-20261116
Mode: mvp
Lane: web
Scope: web
Risk: low
Parallel: true
Depends on: 0045
Tags: planned,m5,review-fix

## Goal

Make `pnpm --dir web exec vitest run` run only unit tests so the web verification command exits zero.

## Context files

- AGENTS.md
- docs/task-system.md
- web/vite.config.ts
- web/playwright.config.ts

## Files allowed to edit

- web/vite.config.ts
- docs/progress.md
- .tasks/0050-exclude-playwright-e2e-specs-from-the-vitest-run.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- web/e2e/library.spec.ts
- web/e2e/history.spec.ts

## Steps

1. Scope Vitest discovery to `src/**/*.test.{ts,tsx}` or exclude `e2e/**` from its default include set
2. Confirm the Playwright suite still has its own runner and config unchanged
3. Run the full Vitest command and check its exit status

## Acceptance criteria

- `pnpm --dir web exec vitest run` exits 0 and reports only the 14 unit test files
- No Playwright spec is loaded or transformed by Vitest
- `pnpm --dir web exec playwright test` remains the separate e2e entry point

## Invariants

- The e2e specs stay owned by Playwright; Vitest never executes `test.beforeAll` from them.

## Review questions

- Could a future `.spec.ts` under `src/` be silently skipped by an over-narrow include pattern?

## Counterexample searches

- Add a temporary `src/**/*.spec.ts` and confirm it is either intentionally excluded by a documented convention or still discovered.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec vitest run"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Post-implementation review remediation of a broken verification command.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
