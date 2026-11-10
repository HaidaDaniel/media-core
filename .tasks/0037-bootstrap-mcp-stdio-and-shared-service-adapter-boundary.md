# Task 0037 - Bootstrap MCP stdio and shared-service adapter boundary

State: todo
Owner: none
Mode: mvp
Lane: mcp
Scope: mcp
Risk: medium
Parallel: true
Depends on: 0005,0006,0007,0009
Tags: planned,m3

## Goal

Add maintained Go MCP SDK and clean stdio mode without HTTP dependency.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/hermes.md
- docs/application-contract.md

## Files allowed to edit
- go.mod
- go.sum
- internal/mcp/server.go
- internal/mcp/server_test.go
- internal/mcp/errors.go
- internal/mcp/errors_test.go
- internal/runtime/runtime.go
- docs/decisions.md
- docs/progress.md

- .tasks/0037-bootstrap-mcp-stdio-and-shared-service-adapter-boundary.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Check/pin SDK/protocol compatibility and document dependency
2. Initialize MCP capabilities, shared service context and typed result/error translation
3. Create subprocess initialization/list-tools/EOF/invalid-message tests with stdout purity

## Acceptance criteria

- MCP can start on initialized temporary DB with no serve process
- Logs go only to stderr and malformed calls do not mutate
- No generic SQL/shell/file tools; no native remote listener

## Invariants

- Protocol transport does not own domain rules

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/mcp -run \"TestMCP(Server|Error|Lifecycle)\""}`

## Documentation updates
- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M3. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
