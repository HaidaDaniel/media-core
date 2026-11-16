# Task 0053 - Reserve the idempotency key on identity-match no-ops

State: todo
Owner: none
Mode: mvp
Lane: application
Scope: application
Risk: high
Parallel: true
Depends on: 0012
Tags: planned,m5,review-fix

## Goal

Close the journal gap where an exact identity match returns `already_exists` without reserving the request key.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/application-contract.md
- docs/decisions.md
- internal/app/addition.go

## Files allowed to edit

- internal/app/addition.go
- internal/app/addition_test.go
- internal/store/identity.go
- internal/store/identity_test.go
- docs/progress.md
- .tasks/0053-reserve-the-idempotency-key-on-identity-match-no-ops.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. In the `EntryIdentityMatch` branch of `createEntry`, reserve the journal key and the canonical `already_exists` result in the same way `useExisting` does
2. Settle a reservation PK conflict through the stored-result replay path so concurrent identical calls stay idempotent
3. Add a regression test: the same key with a different, non-matching payload must conflict instead of creating a new work

## Acceptance criteria

- An exact identity match records a journal row whose result replays byte-identically for the same key and payload
- The same key with a different payload returns `conflict` and creates nothing
- The existing no-write and contradiction behavior of identity matches is unchanged

## Invariants

- A request key maps to exactly one canonical result for the lifetime of the journal; no silent result substitution.

## Review questions

- Does the new reservation happen on every identity-match path, including the concurrent-commit replay branch?

## Counterexample searches

- Race two adds with the same key where one matches an identity and one would create; assert exactly one work and identical results.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/app ./internal/store -run TestAddition"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Post-implementation review remediation of an idempotency contract violation.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
