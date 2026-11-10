# Task 0033 - Build work details and explicit related-media navigation

State: todo
Owner: none
Mode: mvp
Lane: web
Scope: web
Risk: medium
Parallel: false
Depends on: 0030,0025,0029,0028
Tags: planned,m4

## Goal

Present one logical work and its edition/source relationships with deliberate correction controls.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/web-ui.md
- docs/application-contract.md

## Files allowed to edit

- web/src/work/**
- web/src/relations/**
- web/src/App.tsx
- docs/progress.md

- .tasks/0033-build-work-details-and-explicit-related-media-navigation.md

- .tasks/0033-build-work-details-and-explicit-related-media-navigation.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Build work titles/aliases/edition summaries and work-note display
2. Add readable relation/source lists and propose/confirm/revoke/origin forms with evidence
3. Test anime→manga navigation, proposed claim labels, unknown source, cycle/conflict and keyboard layout

## Acceptance criteria

- Each edition keeps separate progress/rating/intention
- Users can follow source in either direction and see provenance
- No graph renderer required and no WN-first inference

## Invariants

- Same work grouping does not assert original-source identity

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec tsc --noEmit && pnpm --dir web test -- --run src/work src/relations"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M4. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.

