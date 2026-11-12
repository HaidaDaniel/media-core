# Task 0015 - Record edition progress by unit with correction history

State: doing
Owner: opencode-media-20261110
Mode: mvp
Lane: application
Scope: application
Risk: medium
Parallel: false
Depends on: 0012,0014
Tags: planned,m1,migration

## Goal

Persist exact numeric progress observations and current values without erasing history.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/domain-model.md
- docs/application-contract.md

## Files allowed to edit

- internal/app/progress.go
- internal/app/progress_test.go
- internal/store/progress.go
- internal/store/progress_test.go
- internal/store/migrations/0006_progress.sql
- internal/cli/progress.go
- internal/cli/progress_test.go
- internal/runtime/runtime.go
- internal/store/editions.go
- internal/store/migrations_test.go
- internal/app/library.go
- internal/app/mutation.go
- internal/cli/cli.go
- docs/progress.md
- .tasks/0015-record-edition-progress-by-unit-with-correction-history.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Validate decimal/unit/total rules and effective-history selection
2. Implement service and CLI with request replay, revisions, explicit correction/retraction
3. Create tests for 42 then 48, reopen, lower reread, unknown total and invalid page/percent

## Acceptance criteria

- Edition/unit values never sum and releases never advance reading
- Negative/oversized/fractional page and position above known total fail atomically
- Past observations and corrected mistakes remain visible with timestamps

## Invariants

- Updating progress does not implicitly set consumption/intention

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/app ./internal/store ./internal/cli -run \"TestProgress\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M1. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
