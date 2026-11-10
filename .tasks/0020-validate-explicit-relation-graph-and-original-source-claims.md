# Task 0020 - Validate explicit relation graph and original-source claims

State: todo
Owner: none
Mode: mvp
Lane: application
Scope: application
Risk: high
Parallel: false
Depends on: 0011,0012,0019
Tags: planned,m4,migration,invariant

## Goal

Implement scoped, provenance-bearing relations/origin assertions with proposal and correction lifecycle.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/domain-model.md
- docs/application-contract.md

## Files allowed to edit
- internal/domain/relation.go
- internal/app/relations.go
- internal/app/relations_test.go
- internal/store/relations.go
- internal/store/relations_test.go
- internal/store/migrations/0011_relations.sql
- internal/cli/relations.go
- internal/runtime/runtime.go
- docs/progress.md

- .tasks/0020-validate-explicit-relation-graph-and-original-source-claims.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Add typed edge/origin tables with endpoint constraints and symmetric canonical keys
2. Implement propose/confirm/revoke/list with transaction cycle checks and source traversal bounds
3. Expose CLI link/origin/related and test translations, shared story, inverse navigation and corrections

## Acceptance criteria

- Reject self/mixed-scope/cyclic confirmed source links with no partial mutation
- Agent guess remains proposed until explicit confirmation; duplicate active edge returns existing
- No assumption WN is original; revoked edge and replacement provenance remain visible

## Invariants

- No graph DB, automatic merge, or progress-sharing

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/domain ./internal/app ./internal/store ./internal/cli -run \"Test(Relation|Origin)\""}`

## Documentation updates
- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M4. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
