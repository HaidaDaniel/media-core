# Task 0005 - Bootstrap Go executable and build identity

State: todo
Owner: none
Mode: mvp
Lane: foundation
Scope: foundation
Risk: low
Parallel: false
Depends on: 0004
Tags: planned,m1

## Goal

Create the smallest compiling executable with version and mode dispatch seams, without library logic.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md

## Files allowed to edit

- go.mod
- cmd/media-core/main.go
- internal/runtime/runtime.go
- internal/cli/cli.go
- internal/buildinfo/**
- docs/progress.md

- .tasks/0005-bootstrap-go-executable-and-build-identity.md

- .tasks/0005-bootstrap-go-executable-and-build-identity.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Create Go module github.com/HaidaDaniel/media-core and runtime/CLI entry points
2. Add version fields with unknown development defaults and ldflags injection
3. Return usage for unsupported commands; reserve serve and mcp without pretending they work

## Acceptance criteria

- go run ./cmd/media-core version prints truthful build identity
- Unsupported commands fail clearly; no state directory or server is created
- Add buildinfo/version unit tests in the allowed packages

## Invariants

- No database or application feature is implemented by bootstrap

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/buildinfo ./internal/cli ./internal/runtime"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M1. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.

