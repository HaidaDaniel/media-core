# Task 0035 - Build edition personal panels for notes ratings intentions and sources

State: todo
Owner: none
Mode: mvp
Lane: web
Scope: web
Risk: medium
Parallel: false
Depends on: 0034,0028,0031
Tags: planned,m4

## Goal

Compose compact personal annotation panels over the already implemented annotation API group.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/web-ui.md
- docs/application-contract.md

## Files allowed to edit

- web/src/annotations/**
- web/src/edition/EditionPage.tsx
- web/src/library/**
- docs/progress.md

- .tasks/0035-build-edition-personal-panels-for-notes-ratings-intentions-and-sources.md

- .tasks/0035-build-edition-personal-panels-for-notes-ratings-intentions-and-sources.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Add saved/planned intention, simple tag picker, note revision/rating and source reference controls
2. Display prior/superseded opinions and explicit add/edit/clear/retire actions
3. Test deferred-reading note, negative rating, source validation, conflict and input retention

4. Extend library filters with intention, tags and minimum rating once their services are available

## Acceptance criteria

- Translation-candidate tag is searchable without a workflow engine
- Editing note/rating retains history; no raw HTML rendering
- External links are safe and include no credentials

## Invariants

- No application logic is reimplemented in panels

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec tsc --noEmit && pnpm --dir web test -- --run src/annotations"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M4. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.

