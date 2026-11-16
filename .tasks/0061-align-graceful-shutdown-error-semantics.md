# Task 0061 - Align graceful-shutdown error semantics

State: done
Owner: opencode-media-fix-20261116
Mode: mvp
Lane: reliability
Scope: reliability,http,mcp
Risk: low
Parallel: true
Depends on: 0024,0048
Tags: planned,m5,review-fix

## Goal

Make the HTTP server and the Streamable HTTP MCP listener treat a shutdown drain deadline consistently.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/architecture.md
- internal/httpapi/server.go
- internal/runtime/runtime.go

## Files allowed to edit

- internal/httpapi/server.go
- internal/httpapi/server_test.go
- internal/runtime/runtime.go
- internal/runtime/runtime_test.go
- docs/progress.md
- .tasks/0061-align-graceful-shutdown-error-semantics.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Choose one documented policy for a shutdown drain that exceeds its deadline
2. Apply it to both `serve` and `mcp --http`; if a force close is used, make it explicit and bounded
3. Test that SIGTERM with a slow in-flight request produces the same exit classification in both modes

## Acceptance criteria

- Both modes return the same status on a clean drain and the same status on a timed-out drain
- SQLite is closed and the maintenance lock released on every shutdown path
- A hung client cannot keep a process alive past the bounded deadline

## Invariants

- Shutdown behavior is a lifecycle policy of the binary, not a per-adapter decision.

## Review questions

- Is a non-zero exit on a timed-out drain desirable for an operator, or should both modes force close and exit cleanly?

## Counterexample searches

- Hold an in-flight request past the 10-second deadline against both modes and compare exit codes and released locks.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/httpapi ./internal/runtime -run TestShutdown"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Post-implementation review remediation of inconsistent shutdown semantics.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
