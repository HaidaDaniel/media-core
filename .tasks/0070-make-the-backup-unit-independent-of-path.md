# Task 0070 - Make the backup unit independent of PATH

State: todo
Owner: none
Mode: mvp
Lane: reliability
Scope: reliability,packaging
Risk: high
Parallel: false
Depends on: 0067
Tags: planned,m5,final-audit,p1

## Goal

Ensure the scheduled backup service always invokes the installed binary by an absolute path, regardless of the service environment, and prove the unit configuration is runnable in an automated check.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/engineering/deployment.md
- deploy/media-core-backup.service
- deploy/media-core-backup.timer
- scripts/backup-smoke.sh

## Files allowed to edit

- deploy/media-core-backup.service
- deploy/media-core.service
- scripts/backup-smoke.sh
- tests/packaging/deployment_test.go
- docs/engineering/deployment.md
- docs/progress.md
- .tasks/0070-make-the-backup-unit-independent-of-path.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- internal/webassets/dist/**
- release/**

## Steps

1. Reproduce the failure with a clear environment (no MEDIA_CORE_BIN, PATH without /opt/media-core): the smoke script reports `media-core: command not found`
2. Set an explicit absolute binary path in the backup unit environment and make the script default to the installed absolute path, rejecting relative or non-executable values
3. Verify directory existence/ownership, script executability, working directory, hardening options, shared maintenance lock acquisition and non-zero exit propagation; confirm no secret material appears in units or logs
4. Add an automated Go test that parses the units, checks their invariants (absolute ExecStart, timer Persistent/Install, ReadWritePaths state+backup, no secrets), and runs a systemd-like integration smoke with a minimal environment and the unit's binary path
5. Document that real `systemd` startup was not exercised by this task

## Acceptance criteria

- With an empty environment except the unit-provided variables, the smoke script creates a backup and restores it into a scratch state dir
- A missing or relative binary path produces a non-zero exit with a clear message
- Unit configuration checks pass locally and, where available, `systemd-analyze verify` reports no syntax errors
- No running HTTP/MCP handle conflict is introduced and no user data is modified
- Timer survives reboot (`Persistent=true`, installed into `timers.target`)

## Invariants

- The backup unit never depends on an ambient shell environment and contains no secret values.

## Review questions

- Could the script still pick up a different binary from PATH under a hardened unit?

## Counterexample searches

- Empty environment, PATH stripped, relative MEDIA_CORE_BIN, missing binary, running serve handle, missing directories, non-executable script.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./tests/packaging -run TestDeploymentUnits -count=1 && bash -n scripts/backup-smoke.sh"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Confirmed P1: the unit calls the script, but the script defaults to PATH lookup while the installed binary is /opt/media-core/media-core.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
