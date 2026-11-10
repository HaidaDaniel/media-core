# Task 0007 - Freeze typed application DTOs errors and contract fixtures

State: todo
Owner: none
Mode: mvp
Lane: foundation
Scope: foundation
Risk: medium
Parallel: true
Depends on: 0005
Tags: planned,m1,public-api

## Goal

Encode the documented v1 records, requests, errors and canonical examples before transport work.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/domain-model.md
- docs/application-contract.md

## Files allowed to edit

- internal/app/contracts.go
- internal/app/contracts_test.go
- internal/domain/values.go
- tests/contracts/fixtures/**
- docs/application-contract.md
- docs/progress.md

- .tasks/0007-freeze-typed-application-dtos-errors-and-contract-fixtures.md

- .tasks/0007-freeze-typed-application-dtos-errors-and-contract-fixtures.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Define bounded value types and errors with stable codes plus revision/actor/request fields
2. Create canonical DTO fixtures for work, edition, candidates, progress and failure envelopes
3. Test JSON round trips including null versus zero and reject invalid enums; do not add persistence

## Acceptance criteria

- Fixtures cover all transport-visible field shapes in application-contract.md
- No separate HTTP/MCP state semantics or success-masked error
- Add contract tests; these fixtures are new deliverables, not existing passing tests

## Invariants

- Unknown scalars stay null; progress decimals round-trip exactly

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/app ./internal/domain"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M1. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.

