# Task 0044 - Verify multi-process concurrency and migration integrity

State: todo
Owner: none
Mode: mvp
Lane: reliability
Scope: reliability
Risk: high
Parallel: false
Depends on: 0012,0015,0023,0037
Tags: planned,m5,async,migration

## Goal

Add focused process-level counterexamples for the shared SQLite and maintenance contract.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/product/requirements.md

## Files allowed to edit

- tests/concurrency/**
- tests/migrations/**
- internal/store/database_test.go
- internal/store/migrations_test.go
- docs/progress.md

- .tasks/0044-verify-multi-process-concurrency-and-migration-integrity.md

- .tasks/0044-verify-multi-process-concurrency-and-migration-integrity.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Create binary harness and temporary state fixtures; no production data
2. Race HTTP-equivalent service/CLI/MCP writes, exact adds and stale-revision updates
3. Test active handle blocks restore/migrate, killed writer recovery, ordered migrations and backup snapshot integrity

## Acceptance criteria

- No duplicate exact identities, half history rows or lost successful commit
- Busy deadlines bounded and failures have stable locked/conflict code
- All append-only migrations work from each prior fixture without losing rows

## Invariants

- Tests may reveal a required fix; amend the affected task scope before source changes

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./tests/concurrency ./tests/migrations"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M5. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.

