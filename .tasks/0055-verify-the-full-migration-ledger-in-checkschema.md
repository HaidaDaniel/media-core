# Task 0055 - Verify the full migration ledger in CheckSchema

State: done
Owner: opencode-media-fix-20261116
Mode: mvp
Lane: storage
Scope: storage,backup
Risk: high
Parallel: true
Depends on: 0009,0023
Tags: planned,m5,review-fix

## Goal

Stop treating a ledger with a matching maximum version but missing earlier migrations as a current schema.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/architecture.md
- internal/store/migrations.go
- internal/backup/restore.go

## Files allowed to edit

- internal/store/migrations.go
- internal/store/migrations_test.go
- internal/backup/archive.go
- internal/backup/archive_test.go
- internal/backup/restore.go
- internal/backup/restore_test.go
- docs/progress.md
- .tasks/0055-verify-the-full-migration-ledger-in-checkschema.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- internal/store/migrations/*.sql

## Steps

1. Load the applied version set and compare it with the full embedded migration set instead of using only COUNT and MAX(version)
2. Distinguish missing, pending, newer and incomplete ledgers with the existing stable schema errors
3. Add a fixture whose ledger contains only the current version and assert that Open, backup validation and restore reject it

## Acceptance criteria

- A ledger missing any embedded migration version is rejected even when MAX(version) equals the current version
- Gaps and extra versions are rejected with the correct stable error class
- Normal current, pending and newer ledgers keep their existing behavior

## Invariants

- Schema boundary checks describe the complete applied schema, not two scalar aggregates.

## Review questions

- Does the stricter check reject a legitimate current ledger produced by the existing migration runner and all prior version fixtures?

## Counterexample searches

- Build a database with only version 13 in `schema_migrations` and confirm backup validation and restore both refuse it.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/store ./internal/backup ./tests/migrations"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Post-implementation review remediation of a schema-boundary and restore-validation weakness.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
