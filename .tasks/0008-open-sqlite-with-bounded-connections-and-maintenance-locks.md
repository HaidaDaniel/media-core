# Task 0008 - Open SQLite with bounded connections and maintenance locks

State: todo
Owner: none
Mode: mvp
Lane: storage
Scope: storage
Risk: high
Parallel: false
Depends on: 0006
Tags: planned,m1,async

## Goal

Add the pure-Go SQLite connection factory and process-lifetime maintenance exclusion.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md

## Files allowed to edit

- go.mod
- go.sum
- internal/store/database.go
- internal/store/database_test.go
- internal/store/maintenance.go
- internal/store/maintenance_test.go
- internal/runtime/runtime.go
- docs/decisions.md
- docs/progress.md

- .tasks/0008-open-sqlite-with-bounded-connections-and-maintenance-locks.md

- .tasks/0008-open-sqlite-with-bounded-connections-and-maintenance-locks.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Check and pin a supported modernc.org/sqlite version; record dependency choice
2. Apply WAL, foreign keys, synchronous FULL and busy timeout per connection with one-connection default pool
3. Implement shared lifetime and exclusive maintenance lock modes, fixed safe DB path and clean close

## Acceptance criteria

- Create tests using temporary files and subprocess holders for busy/exclusive lock behavior
- Opening failures release locks/handles and never leave a partially configured connection
- Normal opener checks schema without implicit migration once schema runner lands

## Invariants

- SQLite stays on local disk; no uncoordinated restore/open race

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/store -run \"Test(Database|Maintenance)\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M1. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
