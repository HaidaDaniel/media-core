# Task 0027 - Expose progress consumption and history HTTP routes

State: todo
Owner: none
Mode: mvp
Lane: http
Scope: http
Risk: medium
Parallel: true
Depends on: 0024,0014,0015
Tags: planned,m2,public-api

## Goal

Provide reading observation/status endpoints and correction/history access.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/application-contract.md

## Files allowed to edit

- internal/httpapi/reading.go
- internal/httpapi/reading_test.go
- internal/httpapi/routes.go
- docs/progress.md

- .tasks/0027-expose-progress-consumption-and-history-http-routes.md

- .tasks/0027-expose-progress-consumption-and-history-http-routes.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Wire progress, consumption, history and typed retract routes
2. Use common decimal/revision/request fields and map domain errors
3. Create tests for chapter 42→48, correction/retract, invalid unit and stale revision

## Acceptance criteria

- History preserves previous rows and timestamps
- Status/progress never mutate publication/intention implicitly
- Repeated request is identical and authorization enforced

## Invariants

- HTTP and CLI reading rules are identical

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/httpapi -run \"TestHTTPReading\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M2. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
