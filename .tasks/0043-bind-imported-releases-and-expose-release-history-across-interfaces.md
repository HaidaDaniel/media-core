# Task 0043 - Bind imported releases and expose release history across interfaces

State: todo
Owner: none
Mode: mvp
Lane: integration
Scope: integration
Risk: high
Parallel: false
Depends on: 0042,0019,0021,0024,0037,0036,0033
Tags: planned,m5,integration,migration

## Goal

Add the small release-binding vertical slice: explicit map, projection, pending controls and visible history.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/integration-contract.md
- docs/product/requirements.md

## Files allowed to edit

- internal/app/import_binding.go
- internal/app/import_binding_test.go
- internal/store/releases.go
- internal/store/releases_test.go
- internal/store/activity.go
- internal/store/migrations/0013_release_bindings.sql
- internal/cli/releases.go
- internal/runtime/runtime.go
- internal/httpapi/imports.go
- internal/httpapi/imports_test.go
- internal/httpapi/routes.go
- internal/mcp/imports.go
- internal/mcp/imports_test.go
- internal/mcp/server.go
- web/src/activity/imports/**
- web/src/work/releases/**
- tests/acceptance/releases_test.go
- docs/progress.md

- .tasks/0043-bind-imported-releases-and-expose-release-history-across-interfaces.md

- .tasks/0043-bind-imported-releases-and-expose-release-history-across-interfaces.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Persist audited publication→edition bindings and immutable release projections; handle rebind/revoke
2. Add bounded HTTP/MCP JSON batch/binding/release adapters and CLI releases
3. Extend existing activity/work panels with pending binding and release list; implement scenario E fixtures

## Acceptance criteria

- Exact confirmed publication map supports future events; no title-based map
- Replay/rebind preserves raw/old history without duplicate active projections
- Count-only/RSS/removal/status semantics preserved; personal progress/intention unchanged

## Invariants

- Only this narrow integration slice crosses all adapters; no new independent UI screen

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/app ./internal/store ./internal/httpapi ./internal/mcp ./tests/acceptance -run \"Test(ImportBinding|Release|ScenarioE)\" && pnpm --dir web exec tsc --noEmit && pnpm --dir web test -- --run src/activity/imports src/work/releases"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M5. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
