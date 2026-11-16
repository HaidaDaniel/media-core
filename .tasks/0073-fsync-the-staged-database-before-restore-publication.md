# Task 0073 - Fsync the staged database before restore publication

State: done
Owner: opencode-media-fix-20261116
Mode: mvp
Lane: reliability
Scope: reliability,storage
Risk: high
Parallel: false
Depends on: 0064
Tags: planned,m5,final-audit,p1

## Goal

Make a successful restore crash-consistent by flushing the staged database contents to stable storage before the rename.

## Context files

- AGENTS.md
- docs/task-system.md
- internal/backup/restore.go
- docs/engineering/backup-restore.md

## Files allowed to edit

- internal/backup/restore.go
- internal/backup/restore_test.go
- docs/engineering/backup-restore.md
- docs/progress.md
- .tasks/0073-fsync-the-staged-database-before-restore-publication.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- internal/store/migrations/*.sql

## Steps

1. Reproduce by inspection: the staged file is closed but never synced; only the directory is synced after rename, so a power cut can expose a truncated database after a successful restore
2. Sync the staged file before closing it, before checksum comparison and before publication
3. Add a test seam for the sync step and a regression test proving a sync failure aborts before rename and leaves the destination untouched
4. Update the backup/restore doc to state both file and directory sync explicitly

## Acceptance criteria

- Staged content is synced before the rename; directory sync still follows publication
- A sync error leaves the previous database intact and returns a non-zero error
- Existing interrupted-restore and symlink regressions remain green

## Invariants

- A reported successful restore never points the directory entry at unsynced data.

## Review questions

- Does the sync happen before the hash check so a torn write cannot be accepted?

## Counterexample searches

- Sync failure, crash after publish, interrupted restore, normal restore.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/backup ./tests/acceptance -run 'TestRestore|TestBackup' -count=1"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Confirmed P1 in final reliability review: only the directory was fsynced after rename.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
