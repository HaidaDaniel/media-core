# Task 0028 - Expose personal annotation HTTP adapter group

State: todo
Owner: none
Mode: mvp
Lane: http
Scope: http
Risk: medium
Parallel: false
Depends on: 0024,0016,0017,0018,0019
Tags: planned,m4,public-api

## Goal

Adapt the existing annotation services to bounded typed HTTP routes without new domain logic.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/application-contract.md

## Files allowed to edit

- internal/httpapi/annotations.go
- internal/httpapi/annotations_test.go
- internal/httpapi/routes.go
- docs/progress.md

- .tasks/0028-expose-personal-annotation-http-adapter-group.md

- .tasks/0028-expose-personal-annotation-http-adapter-group.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Add intention/tag/note/rating/reference route bindings and annotation correction/retraction dispatch
2. Share request decoding/error mapping already introduced in HTTP foundation
3. Create table-driven valid/invalid adapter tests including notes at work scope

## Acceptance criteria

- All documented personal-panel operations reach their existing shared service
- No annotation route bypasses session/CSRF or idempotency/revision
- Notes/reference retirement and superseded ratings remain retrievable

## Invariants

- This task is thin annotation binding only, not annotation persistence or UI

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/httpapi -run \"TestHTTPAnnotation\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M4. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
