# Task 0018 - Add rating observations and rating search filters

State: doing
Owner: opencode-media-20261110
Mode: mvp
Lane: application
Scope: application
Risk: medium
Parallel: false
Depends on: 0013,0017
Tags: planned,m4,migration

## Goal

Implement edition 1–10 ratings with retained changes and explicit clearing/correction.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/domain-model.md
- docs/application-contract.md

## Files allowed to edit

- internal/app/ratings.go
- internal/app/ratings_test.go
- internal/store/ratings.go
- internal/store/ratings_test.go
- internal/store/library.go
- internal/store/editions.go
- internal/store/migrations/0009_ratings.sql
- internal/store/migrations_test.go
- internal/app/library.go
- internal/app/library_test.go
- internal/app/mutation.go
- internal/cli/ratings.go
- internal/cli/library.go
- internal/cli/library_test.go
- internal/cli/cli.go
- internal/runtime/runtime.go
- docs/progress.md
- .tasks/0018-add-rating-observations-and-rating-search-filters.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Add rating history and current selection with revision/request handling
2. Wire CLI rate and minimum-rating/sort predicates
3. Test bounds, null clear, past-dated rating and matching-edition card display

## Acceptance criteria

- 0/11/fractional rating rejected; clear is null not zero
- Original negative/positive rating remains recoverable after change
- Work card never invents an average across editions

## Invariants

- Rating changes never alter progress or status

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/app ./internal/store ./internal/cli -run \"TestRating\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M4. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
- Scope amendment (2026-11-12): the application contract requires the minimum-rating filter and the rating sort ("Sort: title, updated_at, rating, created_at; missing ratings last"), so the rating dimension joins the shared library surface: store/editions.go carries the current rating observation, app/library.go gains the filter/sort dimension and the rating view mapping, app/mutation.go maps the rating errors, and the CLI list command gains the --rating flag and the rating sort value. The 0013 frozen tests that pinned "rating" as an unknown sort (app/library_test.go, cli/library_test.go) flip to the new contract. store/migrations_test.go pins the embedded migration set's shape.
