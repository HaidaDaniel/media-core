# Task 0014 - Record personal consumption status with recoverable history

State: doing
Owner: opencode-media-20261110
Mode: mvp
Lane: application
Scope: application
Risk: medium
Parallel: false
Depends on: 0013
Tags: planned,m1,migration

## Goal

Implement independent consumption state updates, correction/retraction and status filtering.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/domain-model.md
- docs/application-contract.md

## Files allowed to edit

- internal/app/consumption.go
- internal/app/consumption_test.go
- internal/store/consumption.go
- internal/store/consumption_test.go
- internal/store/migrations/0005_consumption.sql
- internal/store/library.go
- internal/cli/consumption.go
- internal/cli/consumption_test.go
- internal/runtime/runtime.go
- docs/progress.md
- .tasks/0014-record-personal-consumption-status-with-recoverable-history.md
- internal/store/editions.go
- internal/store/identity.go
- internal/store/migrations_test.go
- internal/app/library.go
- internal/app/mutation.go
- internal/cli/cli.go
- internal/cli/library.go
- internal/store/editions_test.go
- internal/app/addition.go
- internal/store/works.go
- internal/store/works_test.go

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Add consumption observations and effective current selection
2. Expose service and CLI status with revisions/request replay; extend library status predicate
3. Test same-value no-op, unknown, reread, past-dated updates and superseded rows

## Acceptance criteria

- Completed/dropped/paused transitions preserve all observations
- No planned/on_hold duplicate states and no automatic publication/intention/progress mutation
- Add tests for stale revision and same-row concurrent correction

## Invariants

- New manual editions start not_started; historical unknown remains distinct

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/app ./internal/store ./internal/cli -run \"Test(Consumption|Status)\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M1. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
