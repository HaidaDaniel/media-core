# Task 0019 - Attach multiple sourced external references safely

State: done
Owner: opencode-media-20261110
Mode: mvp
Lane: application
Scope: application
Risk: medium
Parallel: false
Depends on: 0012,0018
Tags: planned,m4,migration

## Goal

Promote identity reservations into multiple edition references with roles and provenance.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/domain-model.md
- docs/application-contract.md

## Files allowed to edit

- internal/app/references.go
- internal/app/references_test.go
- internal/store/references.go
- internal/store/references_test.go
- internal/store/migrations/0010_references.sql
- internal/store/editions.go
- internal/store/migrations_test.go
- internal/app/library.go
- internal/app/mutation.go
- internal/cli/references.go
- internal/cli/references_test.go
- internal/cli/cli.go
- internal/runtime/runtime.go
- docs/progress.md
- .tasks/0019-attach-multiple-sourced-external-references-safely.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Reuse 0012 identity normalization/uniqueness; add role and source assertions without a second dedupe policy
2. Implement add/retire and CLI reference with rollback on conflicting identity
3. Test multiple URLs, provider opaque IDs, canonical equivalence, dangerous schemes and historical retirement

## Acceptance criteria

- New URL on the same edition never creates another edition
- Reference on another edition returns identity_conflict with no silent reassignment
- No URL fetch or inferred authority; source claim is explicit

## Invariants

- Publication and metadata roles remain distinguishable

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/app ./internal/store ./internal/cli -run \"TestReference\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M4. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
- Scope amendment (2026-11-12): the edition view carries references, so the wiring files join the allowed list: store/editions.go carries the active references on the edition, store/migrations_test.go pins the embedded migration set's shape (pin moves to 10), app/library.go renders the frozen reference view, app/mutation.go maps the new reference not-found error, cli/references_test.go carries the reference command's test suite and cli/cli.go lists the reference command in the usage.
