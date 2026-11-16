# Task 0074 - Serialize backup archive publication by name

State: done
Owner: opencode-media-fix-20261116
Mode: mvp
Lane: reliability
Scope: reliability,storage
Risk: high
Parallel: false
Depends on: 0022
Tags: planned,m5,final-audit,p1

## Goal

Prevent two concurrent `backup create` calls with the same name from silently overwriting each other.

## Context files

- AGENTS.md
- docs/task-system.md
- internal/backup/archive.go
- docs/engineering/backup-restore.md

## Files allowed to edit

- internal/backup/archive.go
- internal/backup/archive_test.go
- docs/engineering/backup-restore.md
- docs/progress.md
- .tasks/0074-serialize-backup-archive-publication-by-name.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- internal/store/migrations/*.sql
- release/**

## Steps

1. Reproduce the check-then-rename window: two processes can both pass the ErrExists check before either publishes; on Linux the later rename replaces the first archive
2. Add a short-lived exclusive advisory lock file in the archive destination held from the existence check through publication; keep the shared SQLite maintenance lock so live writers are not blocked
3. Add a concurrency regression with several simultaneous creates of the same name, asserting exactly one success, distinct conflict errors for the rest and one archive on disk
4. Verify overwrite mode still sets aside the prior archive and only removes it after a successful publish

## Acceptance criteria

- Racing creates with one name yield one archive and no silent replacement
- A losing publisher reports the stable archive-exists error
- Normal backup coexistence with serve/MCP shared handles is unchanged
- A lock timeout produces a distinguishable error, not a false success

## Invariants

- Archive publication is serialized by destination, independently of the database maintenance lock.

## Review questions

- Can the new lock file be confused with an archive or removed by retention?

## Counterexample searches

- 8 racing creates, overwrite race, lock held by another process, missing destination dir.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/backup -run TestBackup -count=1"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Confirmed P1 in final reliability review: ErrExists was checked before a plain os.Rename that silently overwrites.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
