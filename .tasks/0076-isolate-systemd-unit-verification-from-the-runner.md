# Task 0076 - Isolate systemd unit verification from the runner

State: todo
Owner: none
Mode: mvp
Lane: acceptance
Scope: acceptance,packaging
Risk: medium
Parallel: false
Depends on: 0070
Tags: planned,m5,ci,p1

## Goal

Make TestDeploymentUnits work on a clean GitHub runner while still detecting real errors in our unit files.

## Context files

- AGENTS.md
- docs/task-system.md
- tests/packaging/deployment_test.go
- deploy/media-core.service
- deploy/media-core-backup.service
- deploy/media-core-backup.timer
- .github/workflows/ci.yml

## Files allowed to edit

- tests/packaging/deployment_test.go
- docs/progress.md
- .tasks/0076-isolate-systemd-unit-verification-from-the-runner.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- deploy/*.service
- deploy/*.timer
- .github/workflows/*.yml

## Steps

1. Study the failing CI run and reproduce the failure mode locally
2. Replace direct `systemd-analyze verify` on production paths with verification of temporary copies: rewrite `/opt/media-core` and `/var/lib|backups/media-core` paths to a temp sandbox containing executable stubs and directories, and run `systemd-analyze verify --recursive-errors=no` on those copies
3. Keep the existing production-path assertions on the original unit files, and keep the backup/restore smoke unchanged
4. Add regression tests proving corrupted unit copies (unknown section, empty ExecStart, unknown key, broken timer) are detected
5. Confirm no root, no system directory changes and no dependence on installed Media Core binaries

## Acceptance criteria

- `go test ./tests/packaging -run TestDeploymentUnits` passes in a clean environment without /opt/media-core
- Real syntax/config errors in our unit copies produce a non-zero verification result and fail the test
- Unrelated host unit warnings cannot affect the result
- Production path/hardening assertions and the systemd-like backup smoke still run
- No `systemd` units are installed and no system directories are modified

## Invariants

- Static verification is sandboxed; production assertions stay explicit.

## Review questions

- Could the path rewriting hide a real production-path error that the line assertions would miss?

## Counterexample searches

- Corrupt each unit in a different way; remove a stub binary; run with a minimal PATH.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./tests/packaging -run TestDeploymentUnits -count=1"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Confirmed CI P1: job verify failed on HEAD f8ec527 at step Go checks; later steps were skipped.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
