# Task 0021 - Project recent personal activity and complete history queries

State: doing
Owner: opencode-media-20261110
Mode: mvp
Lane: application
Scope: application
Risk: medium
Parallel: false
Depends on: 0014,0015,0016,0017,0018,0019,0020
Tags: planned,m4

## Goal

Provide bounded recent activity/history over existing observation and revision tables.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/domain-model.md
- docs/application-contract.md

## Files allowed to edit

- internal/app/activity.go
- internal/app/activity_test.go
- internal/store/activity.go
- internal/store/activity_test.go
- internal/store/library.go
- internal/cli/activity.go
- internal/cli/activity_test.go
- internal/cli/cli.go
- internal/runtime/runtime.go
- docs/progress.md
- .tasks/0021-project-recent-personal-activity-and-complete-history-queries.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Use indexed UNION/query projections; do not introduce a second event store
2. Expose current annotation/reference/title enrichment in library views and CLI activity/history
3. Test recorded/observed ordering, superseded/revoked labels and pagination

## Acceptance criteria

- Progress, status, opinions and relation corrections remain traceable
- No hidden duplication of history or cross-edition state
- Reserve typed imported-release activity variant; importer extends it in 0043

## Invariants

- History is a projection over authoritative relational rows

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/app ./internal/store ./internal/cli -run \"Test(Activity|HistoryView|LibraryFacets)\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M4. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
- Scope amendment (2026-11-12): the activity command joins the CLI usage text, so cli/cli.go joins the allowed list; the command itself registers in internal/cli/activity.go like the other capability commands.
- Scope amendment (2026-11-12, second): add internal/cli/activity_test.go, which carries the activity command's test suite.
