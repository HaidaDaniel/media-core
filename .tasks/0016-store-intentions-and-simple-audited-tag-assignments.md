# Task 0016 - Store intentions and simple audited tag assignments

State: done
Owner: opencode-media-20261110
Mode: mvp
Lane: application
Scope: application
Risk: medium
Parallel: false
Depends on: 0013,0014,0015
Tags: planned,m4,migration

## Goal

Add saved/planned/interested intentions and simple work/edition tags with query predicates.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/domain-model.md
- docs/application-contract.md

## Files allowed to edit

- internal/app/intention.go
- internal/app/tags.go
- internal/app/intention_test.go
- internal/app/tags_test.go
- internal/app/library.go
- internal/app/mutation.go
- internal/store/intention.go
- internal/store/tags.go
- internal/store/intention_test.go
- internal/store/tags_test.go
- internal/store/library.go
- internal/store/editions.go
- internal/store/works.go
- internal/store/migrations/0007_intention_tags.sql
- internal/store/migrations_test.go
- internal/cli/intention.go
- internal/cli/tags.go
- internal/cli/library.go
- internal/cli/cli.go
- internal/cli/intention_test.go
- internal/cli/tags_test.go
- internal/runtime/runtime.go
- docs/progress.md
- .tasks/0016-store-intentions-and-simple-audited-tag-assignments.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Implement intention history and normalized unique tags with audited attach/detach
2. Wire CLI intention/tag and shared library intention/tag filters
3. Test manga saved while anime completed and favorite/translation-candidate queries

## Acceptance criteria

- Saved/planned filters use intention, not read-later tag inference
- Repeated same assignment is a no-op; detach retains audit
- Changing intention/tag never drops an edition or changes publication

## Invariants

- Consumption and intention are independent

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/app ./internal/store ./internal/cli -run \"Test(Intention|Tag)\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M4. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
