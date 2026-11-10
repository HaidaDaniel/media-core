# Task 0009 - Run embedded transactional migrations under exclusive maintenance

State: todo
Owner: none
Mode: mvp
Lane: storage
Scope: storage
Risk: high
Parallel: false
Depends on: 0008,0007
Tags: planned,m1,migration

## Goal

Provide explicit init/migrate and a schema-version boundary for all database modes.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md

## Files allowed to edit

- internal/store/migrations.go
- internal/store/migrations_test.go
- internal/store/migrations/0001_schema.sql
- internal/cli/init.go
- internal/cli/init_test.go
- internal/runtime/runtime.go
- docs/progress.md

- .tasks/0009-run-embedded-transactional-migrations-under-exclusive-maintenance.md

- .tasks/0009-run-embedded-transactional-migrations-under-exclusive-maintenance.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Embed ordered migration files and apply once in an exclusive maintenance session
2. Create schema metadata and reject newer/pending schemas from normal openers
3. Add failure rollback, repeat-init and interrupted-migration tests with temporary databases

## Acceptance criteria

- init creates current empty database; repeated init is safe
- Invalid migration leaves previous schema/data intact
- Active shared handle blocks migration; normal startup never hot-upgrades

## Invariants

- Migration files become append-only after committed use

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/store ./internal/cli -run \"Test(Migration|Init|Schema)\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M1. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.

