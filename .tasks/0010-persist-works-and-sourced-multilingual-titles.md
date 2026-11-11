# Task 0010 - Persist works and sourced multilingual titles

State: doing
Owner: opencode-media-20261110
Mode: mvp
Lane: domain
Scope: domain
Risk: medium
Parallel: false
Depends on: 0009,0007
Tags: planned,m1,migration

## Goal

Persist story groupings and arbitrary multilingual titles with deterministic Unicode keys.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/domain-model.md

## Files allowed to edit

- go.mod
- go.sum
- internal/domain/title.go
- internal/domain/title_test.go
- internal/store/works.go
- internal/store/works_test.go
- internal/store/migrations/0002_works_titles.sql
- internal/store/migrations_test.go
- docs/decisions.md
- docs/progress.md
- .tasks/0010-persist-works-and-sourced-multilingual-titles.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Pin x/text and document dependency; implement NFKC/full-fold/whitespace search keys
2. Add work and work-title tables with preferred-title constraints; edition-title table/foreign key lands in 0011
3. Test JP/RU/romaji titles, retirement history and literal wildcard characters

## Acceptance criteria

- Work creation requires a title and round-trips IDs/timestamps
- Multiple aliases and languages survive reopen; no three-field language limit
- Exact normalized title is not a unique identity key

## Invariants

- Original display text is preserved; no fabricated translation

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/domain ./internal/store -run \"Test(Work|Title|Normalize)\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Scope amendment 2026-11-11: added internal/store/migrations_test.go. The 0009 runner tests assert the embedded set's shape and version; appending 0002_works_titles.sql requires updating those counts. No runner behavior changes.
- Milestone: M1. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
