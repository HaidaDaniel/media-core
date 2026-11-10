# Task 0013 - Search and retrieve the persistent library through CLI

State: todo
Owner: none
Mode: mvp
Lane: application
Scope: application
Risk: medium
Parallel: false
Depends on: 0012
Tags: planned,m1

## Goal

Implement normalized title/alias lookup, paginated work cards and work/edition retrieval.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/domain-model.md
- docs/application-contract.md

## Files allowed to edit
- internal/app/library.go
- internal/app/library_test.go
- internal/store/library.go
- internal/store/library_test.go
- internal/cli/library.go
- internal/cli/library_test.go
- internal/runtime/runtime.go
- docs/progress.md

- .tasks/0013-search-and-retrieve-the-persistent-library-through-cli.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Add bounded search query, cursor and title/created/updated sorting with edition-qualified filters
2. Wire CLI list/show and title/edition metadata correction/archive services
3. Implement base kind/publication filters; define shared query predicates extended by state/tag/rating tasks

## Acceptance criteria

- JP/RU/English/romaji aliases find the same work and identify matching editions
- Wildcard queries are literal; cursor mismatch fails; totals count distinct filtered works
- Missing ID and stale title/metadata revisions return stable errors; archive preserves history

## Invariants

- No filter may combine states from unrelated editions

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/app ./internal/store ./internal/cli -run \"Test(Library|Search|Catalog)\""}`

## Documentation updates
- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M1. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
