# Task 0041 - Expose MCP personal annotations and confirmed relation tools

State: todo
Owner: none
Mode: mvp
Lane: mcp
Scope: mcp
Risk: medium
Parallel: false
Depends on: 0037,0016,0017,0018,0019,0020,0021
Tags: planned,m4

## Goal

Complete thin MCP bindings for existing personal annotations, relationship lifecycle and activity.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/hermes.md
- docs/application-contract.md

## Files allowed to edit

- internal/mcp/annotations.go
- internal/mcp/relations.go
- internal/mcp/activity.go
- internal/mcp/annotations_test.go
- internal/mcp/relations_test.go
- internal/mcp/server.go
- docs/progress.md

- .tasks/0041-expose-mcp-personal-annotations-and-confirmed-relation-tools.md

- .tasks/0041-expose-mcp-personal-annotations-and-confirmed-relation-tools.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Bind typed annotation actions to existing services; reuse shared decoder/error helpers
2. Bind related/propose/confirm/revoke/origin/activity and require explicit confirmation context
3. Use table-driven schema/result tests plus source-cycle and proposal-not-fact counterexamples

## Acceptance criteria

- Hermes can save manga for later, add note/tag/rating/source and link it to completed anime
- No business logic, persistence or metadata fetching added in this adapter-only task
- MCP schemas cover all annotation/relation actions listed in hermes.md

## Invariants

- A confidence score never confirms an edge

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/mcp -run \"TestMCP(Annotation|Relation|Origin|Activity)\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M4. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.

