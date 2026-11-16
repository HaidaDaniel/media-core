# Task 0059 - Verify restore paths before creating directories

State: done
Owner: opencode-media-fix-20261116
Mode: mvp
Lane: reliability
Scope: reliability,storage
Risk: medium
Parallel: true
Depends on: 0023
Tags: planned,m5,review-fix

## Goal

Reject a symlinked state-directory chain before `MkdirAll` can create anything through it.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/architecture.md
- internal/backup/restore.go
- internal/store/database.go

## Files allowed to edit

- internal/backup/restore.go
- internal/backup/restore_test.go
- internal/store/maintenance.go
- internal/store/maintenance_test.go
- docs/progress.md
- .tasks/0059-verify-restore-paths-before-creating-directories.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Run the path-chain verification before the state directory is created, matching the order used by `store.Open`
2. Keep the exclusive maintenance lock acquisition and all existing archive validation semantics unchanged
3. Add a regression test with a symlinked parent component and assert nothing is created beyond it

## Acceptance criteria

- A symlinked component in the destination state dir is rejected before any directory creation
- A normal missing state dir is still created and restored into under the exclusive lock
- A failure leaves the destination untouched

## Invariants

- Path trust is established before filesystem mutation, in every mode that creates state.

## Review questions

- Are other maintenance entry points already using the safe verify-then-create order?

## Counterexample searches

- Point the destination at a symlink to another directory and confirm no files appear in the target.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/backup -run TestRestore"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Post-implementation review remediation of path verification ordering. Scope expanded before editing: the shared path-chain guard is unexported in internal/store, so this task may add a thin exported wrapper there rather than duplicating symlink rules in the backup package.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
