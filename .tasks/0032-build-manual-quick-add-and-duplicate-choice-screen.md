# Task 0032 - Build manual quick-add and duplicate choice screen

State: todo
Owner: none
Mode: mvp
Lane: web
Scope: web
Risk: medium
Parallel: true
Depends on: 0030,0026
Tags: planned,m2

## Goal

Provide fast title/URL/ID addition with explicit possible-duplicate decisions.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/web-ui.md
- docs/application-contract.md

## Files allowed to edit
- web/src/add/**
- web/src/App.tsx
- docs/progress.md

- .tasks/0032-build-manual-quick-add-and-duplicate-choice-screen.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Implement title-first optional edition/reference fields and URL-only manual-title fallback
2. Show existing/candidates before commit and support explicit work/edition/distinct choice
3. Create tests for retry key reuse, stale token, invalid input and offline manual success

## Acceptance criteria

- Existing identity navigates to existing record without a duplicate
- Possible match never silently merges or creates until user chooses
- Input retained on failure; add can complete without metadata/cover/LLM

## Invariants

- Use server candidate decisions, not client fuzzy matching

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec tsc --noEmit && pnpm --dir web test -- --run src/add"}`

## Documentation updates
- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M2. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
