# Task 0031 - Build library list search filters and counts

State: todo
Owner: none
Mode: mvp
Lane: web
Scope: web
Risk: medium
Parallel: true
Depends on: 0030,0025,0014
Tags: planned,m2

## Goal

Build the daily library screen with matching-edition states and URL-backed search/filter state.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/web-ui.md
- docs/application-contract.md

## Files allowed to edit

- web/src/library/**
- web/src/App.tsx
- web/src/styles.css
- docs/progress.md

- .tasks/0031-build-library-list-search-filters-and-counts.md

- .tasks/0031-build-library-list-search-filters-and-counts.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Implement responsive list/card view and navigation to edition details
2. Add initial search/kind/consumption filters, sorts, counts and bounded pagination; advanced intention/tag/rating controls land in 0035
3. Create component tests for Unicode match, filter reset, stale fetch, missing metadata and empty/error states

## Acceptance criteria

- Watched/read and saved remain visually distinct
- Filters/sort survive refresh/back and total reflects filtered works
- 360px layout has no overflow; keyboard controls labeled

## Invariants

- No invented covers/ratings and no cross-edition filter combination

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec tsc --noEmit && pnpm --dir web test -- --run src/library"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M2. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
