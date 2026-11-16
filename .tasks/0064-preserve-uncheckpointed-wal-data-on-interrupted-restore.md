# Task 0064 - Preserve uncheckpointed WAL data on interrupted restore

State: done
Owner: opencode-media-fix-20261116
Mode: mvp
Lane: reliability
Scope: reliability,storage
Risk: high
Parallel: false
Depends on: 0023,0059
Tags: planned,m5,audit,p1

## Goal

Keep every acknowledged transaction of the destination database intact when a restore is interrupted after sidecar handling but before publication.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/architecture.md
- docs/engineering/backup-restore.md
- internal/backup/restore.go

## Files allowed to edit

- internal/backup/restore.go
- internal/backup/restore_test.go
- docs/engineering/backup-restore.md
- docs/progress.md
- .tasks/0064-preserve-uncheckpointed-wal-data-on-interrupted-restore.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- internal/store/migrations/*.sql

## Steps

1. Reproduce with a subprocess that commits in WAL mode and exits abruptly before a checkpoint, leaving a non-empty `-wal`; confirm a main-file-only copy lacks the committed row
2. Change restore so it checkpoints the destination's WAL into the main database before removing `-wal`/`-shm`, then publishes the staged snapshot with a durable rename
3. Add a regression test that interrupts restore before publish (the existing pre-publish hook) and proves the committed row is still readable from the main file alone
4. Keep the successful restore path, exclusive maintenance lock, archive validation and sidecar cleanup intact

## Acceptance criteria

- A committed transaction that existed only in the destination WAL survives an interrupted restore with no data loss
- Sidecars are removed only after the checkpoint has made their content part of the main database
- A successful restore still publishes exactly one validated snapshot file and leaves no stale `-wal`/`-shm`
- No archive validation or idempotency behavior regresses

## Invariants

- An interrupted restore is a no-op with respect to the destination's acknowledged data.

## Review questions

- Does the checkpoint run under the already-held exclusive maintenance lock without acquiring a second store handle?
- What happens if checkpoint itself fails or times out?

## Counterexample searches

- Crash after commit before checkpoint; interrupt before publish; interrupt during checkpoint; interrupt after publish; clean restore.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/backup ./internal/store ./tests/acceptance -run \"TestRestore|TestBackup\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Confirmed P1: deleting WAL sidecars before publication discards acknowledged writes when restore fails before the rename.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
