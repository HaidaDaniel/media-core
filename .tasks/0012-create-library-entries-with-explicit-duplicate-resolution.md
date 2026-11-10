# Task 0012 - Create library entries with explicit duplicate resolution

State: todo
Owner: none
Mode: mvp
Lane: application
Scope: application
Risk: high
Parallel: false
Depends on: 0011,0007
Tags: planned,m1,invariant,migration

## Goal

Implement atomic manual add with exact identity replay and title-candidate decisions.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/domain-model.md
- docs/application-contract.md

## Files allowed to edit
- internal/app/addition.go
- internal/app/addition_test.go
- internal/app/mutation.go
- internal/store/identity.go
- internal/store/identity_test.go
- internal/store/migrations/0004_identity.sql
- internal/cli/add.go
- internal/cli/add_test.go
- internal/runtime/runtime.go
- docs/progress.md

- .tasks/0012-create-library-entries-with-explicit-duplicate-resolution.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Create request-result journal and normalized external identity reservation table used by later references
2. Implement ResolveAddition/CreateLibraryEntry/AddEdition with candidate tokens and transaction recheck
3. Expose CLI add with explicit use-existing/add-edition/create-distinct choice; test rollback/races

## Acceptance criteria

- Title-only manual creation works offline; URL-only without known identity requests a title
- Existing exact identity returns created false; contradictory fields and stale decisions fail without writes
- Create tests for identical retry, key collision, same title distinct edition and response loss

## Invariants

- No silent title merge; one transaction includes identity and response

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/app ./internal/store ./internal/cli -run \"Test(Addition|Identity|Mutation|Add)\""}`

## Documentation updates
- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M1. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
