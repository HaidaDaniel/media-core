# Task 0036 - Build recent-activity and history screen

State: todo
Owner: none
Mode: mvp
Lane: web
Scope: web
Risk: low
Parallel: false
Depends on: 0030,0029
Tags: planned,m4

## Goal

Show recent personal changes with useful dates and links rather than analytics.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/web-ui.md
- docs/application-contract.md

## Files allowed to edit

- web/src/activity/**
- web/src/App.tsx
- docs/progress.md

- .tasks/0036-build-recent-activity-and-history-screen.md

- .tasks/0036-build-recent-activity-and-history-screen.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Build paginated activity list with recorded/observed time and correction labels
2. Add navigation to edition/history and empty/loading/error states
3. Test deterministic ordering and imported-event variant with a contract fixture only

## Acceptance criteria

- Personal changes and imported releases are visually distinct
- No fabricated release data before importer task extends real reads
- Accessible responsive list, not an analytics dashboard

## Invariants

- 0043 owns real import binding/release controls

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec tsc --noEmit && pnpm --dir web test -- --run src/activity"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M4. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.

