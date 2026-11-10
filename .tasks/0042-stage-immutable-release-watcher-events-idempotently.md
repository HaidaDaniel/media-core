# Task 0042 - Stage immutable Release Watcher events idempotently

State: todo
Owner: none
Mode: mvp
Lane: integration
Scope: integration
Risk: high
Parallel: false
Depends on: 0009,0012,0020
Tags: planned,m5,integration,migration

## Goal

Import version 1.0 file/stdin envelopes into immutable pending records with event-level transactions.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/integration-contract.md
- docs/domain-model.md

## Files allowed to edit
- internal/importer/decode.go
- internal/importer/decode_test.go
- internal/app/imports.go
- internal/app/imports_test.go
- internal/store/imports.go
- internal/store/imports_test.go
- internal/store/migrations/0012_imports.sql
- internal/cli/imports.go
- internal/runtime/runtime.go
- tests/contracts/release-watcher/**
- docs/integration-contract.md
- docs/progress.md

- .tasks/0042-stage-immutable-release-watcher-events-idempotently.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Recheck producer contract and pin sanitized representative fixtures with source revision
2. Validate envelope/major before writes; process bounded events with per-item results
3. Add CLI import/pending and tests for replay, conflicting payload, unknown fields/type and partial item failure

## Acceptance criteria

- Same event_id/payload is no-op; different payload is conflict
- No edition inferred from title and unsupported major writes nothing
- Raw payload and source IDs preserved; canonical JSON equality ignores object-key order/whitespace on replay; no producer DB access

## Invariants

- Importer never advances personal reading

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/importer ./internal/app ./internal/store -run \"Test(Import|EventDecode)\""}`

## Documentation updates
- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
