# Task 0054 - Bound progress decimal parsing against int64 overflow

State: todo
Owner: none
Mode: mvp
Lane: domain
Scope: domain,storage
Risk: high
Parallel: true
Depends on: 0015
Tags: planned,m5,review-fix

## Goal

Reject oversized decimal positions before int64 accumulation can wrap into an accepted value.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/domain-model.md
- internal/store/progress.go

## Files allowed to edit

- internal/store/progress.go
- internal/store/progress_test.go
- internal/app/progress.go
- internal/app/progress_test.go
- docs/progress.md
- .tasks/0054-bound-progress-decimal-parsing-against-int64-overflow.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Reject integer parts with more digits than the 1,000,000,000 bound allows before the multiply-and-add loop
2. Guard the final scaling multiplication against overflow, or compare the accumulated value against the bound during the loop
3. Add table-driven cases for 2^64, long digit strings and values just above and at the bound

## Acceptance criteria

- `18446744073709551616` and other wrapping literals are rejected with `invalid_argument`
- `1000000000` and `1000000000.000` remain accepted
- No stored position can exceed the documented bound regardless of input length

## Invariants

- Exact decimal strings are validated by magnitude, not by their wrapped machine representation.

## Review questions

- Are totals and percent positions covered by the same overflow guard as the position?

## Counterexample searches

- Submit each crafted literal through the app service, not just the parser, and assert the prior state is unchanged.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/store ./internal/app -run TestProgress"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Post-implementation review remediation of a validation bypass confirmed by a standalone reproducer.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
