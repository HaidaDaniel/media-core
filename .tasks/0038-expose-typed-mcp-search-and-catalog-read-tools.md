# Task 0038 - Expose typed MCP search and catalog read tools

State: todo
Owner: none
Mode: mvp
Lane: mcp
Scope: mcp
Risk: medium
Parallel: true
Depends on: 0037,0013
Tags: planned,m3

## Goal

Expose search_library/get_work/get_edition over the existing query service.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/hermes.md
- docs/application-contract.md

## Files allowed to edit

- internal/mcp/library.go
- internal/mcp/library_test.go
- internal/mcp/server.go
- docs/progress.md

- .tasks/0038-expose-typed-mcp-search-and-catalog-read-tools.md

- .tasks/0038-expose-typed-mcp-search-and-catalog-read-tools.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Define bounded tool schemas/read-only annotations and common DTO results
2. Map unsupported filters/missing IDs/query cursor errors
3. Create MCP request tests for aliases, matching editions and unknown values

## Acceptance criteria

- Catalog result is equivalent to HTTP/CLI shared DTOs
- Read tool is side-effect-free and no source claims fabricated
- Invalid extra fields/oversized queries fail clearly

## Invariants

- Search never calls an LLM/provider

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/mcp -run \"TestMCPLibrary\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M3. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
