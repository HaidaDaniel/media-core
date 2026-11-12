# Task 0023 - Restore validated backups under exclusive maintenance

State: doing
Owner: opencode-media-20261110
Mode: mvp
Lane: reliability
Scope: reliability
Risk: high
Parallel: false
Depends on: 0022
Tags: planned,m1

## Goal

Restore a current-schema backup into an isolated or explicitly replaceable state directory safely.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/product/requirements.md

## Files allowed to edit

- internal/backup/restore.go
- internal/backup/restore_test.go
- internal/app/restore.go
- internal/cli/restore.go
- internal/cli/restore_test.go
- internal/cli/cli.go
- internal/runtime/runtime.go
- docs/engineering/backup-restore.md
- docs/progress.md
- .tasks/0023-restore-validated-backups-under-exclusive-maintenance.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Validate entry paths, archive limits, manifest/hash/schema and integrity before touching destination
2. Require exclusive lifetime maintenance lock, stage same-filesystem temp DB and atomic replacement/rollback
3. Create M1 subset scenario F fixture and interruption/corrupt/newer-schema/active-handle tests

## Acceptance criteria

- Round-trip works/titles/editions/progress/request journal matches application-visible state
- Existing destination needs explicit force; active serve/MCP/CLI blocks replacement
- Destination owner secret is retained; corrupted archive leaves old DB usable

## Invariants

- Complete M5 history fixture is added by 0047, not presumed to exist now

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/backup ./internal/app ./internal/cli -run \"TestRestore\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M1. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
- Scope amendment (2026-11-12): the restore command joins the CLI usage text, so cli/cli.go joins the allowed list; the command itself registers in internal/cli/restore.go like the other capability commands.
