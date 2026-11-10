# Task 0039 - Expose typed MCP addition and catalog correction tools

State: todo
Owner: none
Mode: mvp
Lane: mcp
Scope: mcp
Risk: medium
Parallel: true
Depends on: 0037,0012,0013
Tags: planned,m3

## Goal

Expose resolve_addition/create_entry/update_catalog without bypassing identity decisions.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/hermes.md
- docs/application-contract.md

## Files allowed to edit

- internal/mcp/catalog_writes.go
- internal/mcp/catalog_writes_test.go
- internal/mcp/server.go
- docs/progress.md

- .tasks/0039-expose-typed-mcp-addition-and-catalog-correction-tools.md

- .tasks/0039-expose-typed-mcp-addition-and-catalog-correction-tools.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Define explicit write schemas with actor, request and revision fields
2. Bind create/add-edition/title/metadata/archive actions to shared services
3. Test candidate choice, exact conflict, lost-response retry and invalid action

## Acceptance criteria

- Unresolved candidate creates nothing until explicit choice
- MCP same edition/new URL cannot silently clone or reassign
- Domain failure returns isError plus stable code

## Invariants

- Agent-generated identity hints are not confirmed facts

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/mcp -run \"TestMCPCatalogWrite\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M3. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
