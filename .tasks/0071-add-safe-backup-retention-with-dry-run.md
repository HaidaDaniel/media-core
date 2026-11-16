# Task 0071 - Add safe backup retention with dry-run

State: done
Owner: opencode-media-fix-20261116
Mode: mvp
Lane: reliability
Scope: reliability
Risk: high
Parallel: false
Depends on: 0067,0070
Tags: planned,m5,final-audit,p2

## Goal

Bound the daily backup archive count with explicit, auditable retention (7 daily, 4 weekly, 3 monthly UTC buckets) that never touches manual or unverified archives.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/engineering/deployment.md
- docs/engineering/backup-restore.md
- scripts/backup-smoke.sh

## Files allowed to edit

- scripts/backup_retention.py
- scripts/test_backup_retention.py
- scripts/backup-smoke.sh
- deploy/media-core-backup.service
- tests/packaging/deployment_test.go
- docs/engineering/deployment.md
- docs/engineering/backup-restore.md
- docs/progress.md
- .tasks/0071-add-safe-backup-retention-with-dry-run.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- release/**
- internal/webassets/dist/**
- Any archive outside an isolated test directory

## Steps

1. Give timer-created archives a dedicated automatic prefix and write a `.verified` marker only after the restore smoke succeeds
2. Add a small stdlib-only Python component with a pure planning function: UTC daily/weekly/monthly buckets, keep the newest 7/4/3, always keep the newest verified archive, consider only automatic archives with a verified marker
3. Use `lstat` only, reject symlinks, use a bounded lock file for concurrent runs, provide `--dry-run`, and fail closed on filesystem errors
4. Invoke retention from the smoke script only after a successful backup and restore; add unit tests covering overlaps, reruns, symlinks, manual archives, missing markers, last-backup preservation, dry-run and errors
5. Document that retention does not replace a separate copy on another physical medium

## Acceptance criteria

- Manual/unknown archives and symlinks are never deleted
- No archive is deleted before a new archive passes restore smoke and receives its marker
- The most recent verified archive is never deleted, even if policy buckets would drop it
- Re-running retention is idempotent; concurrent runs cannot both delete the same archive
- `--dry-run` lists exactly the same deletions without changing any file
- Disk-full/FS errors produce a non-zero exit and no partial unsafe deletion

## Invariants

- Deletion is a pure, prefix-scoped, verified-only decision under a lock; no glob removal and no symlink following.

## Review questions

- Can an archive lose all backups if the newest marker is removed externally?

## Counterexample searches

- Overlapping week/month boundaries, duplicate timestamps, manual archive with matching date, symlink archive, unverified archive, dry-run, concurrent runs, read-only backup dir.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts -p 'test_backup_retention.py' && bash -n scripts/backup-smoke.sh scripts/build-release.sh"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Confirmed P2: the daily timer creates archives without bound; no retention exists. Scope expanded before editing: the backup unit ExecStart is pinned by TestDeploymentUnits, so the retention prefix argument requires updating that assertion.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
