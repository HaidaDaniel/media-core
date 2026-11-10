# Task 0011 - Persist edition lines and sourced publication observations

State: todo
Owner: none
Mode: mvp
Lane: domain
Scope: domain
Risk: medium
Parallel: false
Depends on: 0010
Tags: planned,m1,migration

## Goal

Add concrete edition identity and independent sourced publication state.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/domain-model.md

## Files allowed to edit
- internal/domain/edition.go
- internal/domain/edition_test.go
- internal/store/editions.go
- internal/store/editions_test.go
- internal/store/migrations/0003_editions.sql
- docs/progress.md

- .tasks/0011-persist-edition-lines-and-sourced-publication-observations.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Persist kind/language/work membership and edition-title rows with real edition foreign keys
2. Add publication observations, explicit unknown and current-source selection
3. Test all supported kinds, other label and invalid work/edition references

## Acceptance criteria

- Translations retain their medium; same work may have anime and manga editions
- Publication completed/hiatus/cancelled never means personally completed/dropped
- Unknown language and publication data remain explicit

## Invariants

- One edition belongs to one work; no shared progress inferred

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/domain ./internal/store -run \"Test(Edition|Publication)\""}`

## Documentation updates
- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M1. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
