# Task 0029 - Expose relation origin and recent-activity HTTP reads and writes

State: doing
Owner: opencode-media-20261110
Mode: mvp
Lane: http
Scope: http
Risk: medium
Parallel: false
Depends on: 0024,0020,0021
Tags: planned,m4,public-api

## Goal

Adapt relation/origin lifecycle and existing activity projection for Web navigation.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/application-contract.md

## Files allowed to edit

- internal/httpapi/relations.go
- internal/httpapi/activity.go
- internal/httpapi/relations_test.go
- internal/httpapi/activity_test.go
- internal/httpapi/routes.go
- docs/progress.md
- .tasks/0029-expose-relation-origin-and-recent-activity-http-reads-and-writes.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Wire bounded related/activity queries and typed propose/confirm/revoke/origin routes
2. Preserve proposal state and provenance in response DTOs
3. Test reverse source navigation, cycle error, revoked history and activity pagination

## Acceptance criteria

- Confirmation is explicit and no confidence threshold confirms an edge
- Related reads display source direction and unknown origin honestly
- No transport graph logic or second activity store

## Invariants

- Confirmed and proposed links remain distinguishable

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/httpapi -run \"TestHTTP(Relation|Origin|Activity)\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M4. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
