# Task 0022 - Create consistent SQLite snapshot backups with manifests

State: doing
Owner: opencode-media-20261110
Mode: mvp
Lane: reliability
Scope: reliability
Risk: high
Parallel: true
Depends on: 0009,0015,0012
Tags: planned,m1

## Goal

Publish a consistent database archive with integrity metadata while normal writers may run.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/product/requirements.md

## Files allowed to edit

- internal/backup/archive.go
- internal/backup/archive_test.go
- internal/app/backup.go
- internal/cli/backup.go
- internal/cli/backup_test.go
- internal/cli/cli.go
- internal/runtime/runtime.go
- docs/engineering/backup-restore.md
- docs/progress.md
- .tasks/0022-create-consistent-sqlite-snapshot-backups-with-manifests.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Create a temporary VACUUM INTO snapshot under shared maintenance lock; validate it
2. Package DB plus manifest size/hash/schema/app identity, publish atomically and refuse overwrite unless explicit
3. Add concurrent-write snapshot and disk/write failure tests; document archive contract

## Acceptance criteria

- Backup reflects one consistent snapshot including WAL commits, not a bare live-file copy
- Archive excludes owner secret/session/TLS keys
- Failure leaves source untouched and no apparently complete corrupt archive

## Invariants

- No claim about newer commits after the snapshot boundary

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/backup ./internal/app ./internal/cli -run \"TestBackup\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M1. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
- Scope amendment (2026-11-12): the backup command joins the CLI usage text, so cli/cli.go joins the allowed list; the command itself registers in internal/cli/backup.go like the other capability commands.
