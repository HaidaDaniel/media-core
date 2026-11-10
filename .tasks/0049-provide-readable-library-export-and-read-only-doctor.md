# Task 0049 - Provide readable library export and read-only doctor

State: todo
Owner: none
Mode: mvp
Lane: reliability
Scope: reliability
Risk: medium
Parallel: true
Depends on: 0013,0015
Tags: planned,m1

## Goal

Provide portable readable state export and diagnostics without inventing repair or import behavior.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/application-contract.md

## Files allowed to edit
- internal/app/export.go
- internal/app/export_test.go
- internal/cli/export.go
- internal/cli/export_test.go
- internal/cli/doctor.go
- internal/cli/doctor_test.go
- internal/runtime/runtime.go
- docs/engineering/configuration.md
- docs/progress.md

- .tasks/0049-provide-readable-library-export-and-read-only-doctor.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Stream versioned JSON export using stable row ordering including IDs/provenance/history
2. Implement doctor read-only config/schema/permission checks without migrations or implicit state creation
3. Test missing state, newer schema and unknown values; document export is not executable restore input

## Acceptance criteria

- Export and backup have distinct formats; restore consumes only validated backup archive
- Doctor never mutates DB and diagnoses active/unsupported state clearly
- Export uses a consistent read snapshot and evolves to include later capability tables

## Invariants

- Task 0047 extends the initial export registry for later domain/import tables and checks full coverage

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/app ./internal/cli -run \"Test(Export|Doctor)\""}`

## Documentation updates
- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M1. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
