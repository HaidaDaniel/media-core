# Task 0069 - Disambiguate the Playwright note-text assertion

State: done
Owner: opencode-media-fix-20261116
Mode: mvp
Lane: acceptance
Scope: acceptance
Risk: medium
Parallel: false
Depends on: 0046,0067
Tags: planned,m5,audit

## Goal

Make the browser history scenario assert the rendered note text instead of matching both the rendered note and the editable textarea.

## Context files

- AGENTS.md
- docs/task-system.md
- web/e2e/history.spec.ts

## Files allowed to edit

- web/e2e/history.spec.ts
- docs/progress.md
- .tasks/0069-disambiguate-the-playwright-note-text-assertion.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- web/src/annotations/NotesPanel.tsx

## Steps

1. Reproduce the strict-mode failure in scenario D with `playwright test e2e/history.spec.ts`
2. Scope the note-text assertions to the rendered notes list under the Notes heading, leaving the form textarea out of the match
3. Re-run the full history spec and confirm all scenarios pass

## Acceptance criteria

- `pnpm --dir web exec playwright test e2e/history.spec.ts` exits 0 with no strict-mode violation
- The assertion still proves the revised note text is rendered, not merely present in an input
- No production UI behavior changes

## Invariants

- Acceptance tests assert user-visible rendered state with unambiguous locators.

## Review questions

- Do the other note assertions in the spec have the same ambiguity?

## Counterexample searches

- Re-run the spec twice to check for order-dependent flakiness.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec playwright test e2e/history.spec.ts"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Newly confirmed during final audit verification: CI would fail because scenario D's global `getByText(note)` matches both the rendered note span and the retained textarea value.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
