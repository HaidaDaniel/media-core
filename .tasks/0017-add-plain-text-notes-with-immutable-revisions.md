# Task 0017 - Add plain-text notes with immutable revisions

State: todo
Owner: none
Mode: mvp
Lane: application
Scope: application
Risk: medium
Parallel: false
Depends on: 0012,0016
Tags: planned,m4,migration

## Goal

Preserve ordinary work/edition notes and distinguish edits from later thoughts.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/domain-model.md
- docs/application-contract.md

## Files allowed to edit

- internal/app/notes.go
- internal/app/notes_test.go
- internal/store/notes.go
- internal/store/notes_test.go
- internal/store/migrations/0008_notes.sql
- internal/cli/notes.go
- internal/runtime/runtime.go
- docs/progress.md

- .tasks/0017-add-plain-text-notes-with-immutable-revisions.md

- .tasks/0017-add-plain-text-notes-with-immutable-revisions.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Implement add/revise/retract with bounded plain text, spoiler flag and optional location
2. Expose CLI note and shared read DTOs
3. Test revision conflicts, old text recovery and work-versus-edition targets

## Acceptance criteria

- Deferral explanation remains visible without marking dropped
- Editing retains original text and audit; adding a later note creates a distinct note
- Oversized text/invalid target fails without reserving request key

## Invariants

- Plain text only; no hidden raw HTML execution

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/app ./internal/store ./internal/cli -run \"TestNote\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M4. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.

