# Task 0057 - Stop echoing raw internal errors to clients

State: todo
Owner: none
Mode: mvp
Lane: http
Scope: http,mcp
Risk: medium
Parallel: true
Depends on: 0024,0037
Tags: planned,m5,review-fix

## Goal

Replace verbatim `err.Error()` client messages with generic stable errors while keeping diagnostics on the server side.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/application-contract.md
- internal/mcp/errors.go
- internal/httpapi/routes.go

## Files allowed to edit

- internal/mcp/errors.go
- internal/mcp/errors_test.go
- internal/httpapi/routes.go
- internal/httpapi/routes_test.go
- docs/progress.md
- .tasks/0057-stop-echoing-raw-internal-errors-to-clients.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Map unclassified errors to a generic `internal_error` message with the stable code and request id, logging the raw error server-side only
2. Apply the same treatment to the HTTP storage-error mapping that currently embeds `err.Error()`
3. Update tests that assert specific raw text to assert the stable code and absence of internal detail

## Acceptance criteria

- No SQLite message, filesystem path or marshaling detail appears in a tool error or HTTP envelope
- Known `*app.AppError` codes, messages, retryability and revisions are unchanged
- A catch-all failure is still distinguishable by its stable `internal_error` code

## Invariants

- The client-visible wire error is a stable contract value; raw errors are operator diagnostics.

## Review questions

- Does generic wording break any documented error message that clients rely on?

## Counterexample searches

- Force an unclassified store failure and inspect both MCP tool error and HTTP JSON bodies for leaked detail.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/mcp ./internal/httpapi -run TestError"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Post-implementation review remediation of internal-detail leakage over network adapters.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
