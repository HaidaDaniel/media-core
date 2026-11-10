# Task 0040 - Expose typed MCP progress status and history tools

State: todo
Owner: none
Mode: mvp
Lane: mcp
Scope: mcp
Risk: medium
Parallel: true
Depends on: 0037,0014,0015
Tags: planned,m3

## Goal

Expose daily reading updates and correction/history through shared use cases.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/hermes.md
- docs/application-contract.md

## Files allowed to edit
- internal/mcp/reading.go
- internal/mcp/reading_test.go
- internal/mcp/server.go
- docs/progress.md

- .tasks/0040-expose-typed-mcp-progress-status-and-history-tools.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Bind record_progress/set_consumption/get_history and typed retraction action
2. Validate transport schemas then defer semantic validation to app
3. Test chapter 42→48/restart, stale revision, correction and invalid unit

## Acceptance criteria

- Both progress observations survive and current value matches HTTP
- Reading statement never implicitly means completed
- Retry returns identical result and history is not duplicated

## Invariants

- Progress is edition/unit scoped

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/mcp -run \"TestMCPReading\""}`

## Documentation updates
- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M3. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
