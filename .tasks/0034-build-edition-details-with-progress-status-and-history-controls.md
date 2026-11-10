# Task 0034 - Build edition details with progress status and history controls

State: todo
Owner: none
Mode: mvp
Lane: web
Scope: web
Risk: medium
Parallel: true
Depends on: 0030,0025,0026,0027
Tags: planned,m2

## Goal

Provide the minimal usable edition page for daily progress/status updates and history corrections.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/web-ui.md
- docs/application-contract.md

## Files allowed to edit

- web/src/edition/**
- web/src/App.tsx
- docs/progress.md

- .tasks/0034-build-edition-details-with-progress-status-and-history-controls.md

- .tasks/0034-build-edition-details-with-progress-status-and-history-controls.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Show titles/kind/language/publication source and basic work identity; add narrow metadata/title corrections
2. Add per-unit progress, consumption and observation history/correction/retraction controls
3. Test restart fetch, stale revision, unknown total, invalid fields, loading/error and mobile layout

## Acceptance criteria

- Chapter 42→48 retains prior record; changing status does not fake publication status
- New update and correct mistake are separate actions
- Library can navigate here before advanced work-details screen exists

## Invariants

- This task does not bundle notes, ratings or relation forms

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec tsc --noEmit && pnpm --dir web test -- --run src/edition"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M2. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
