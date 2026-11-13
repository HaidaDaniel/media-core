# Task 0025 - Expose catalog search and detail HTTP reads

State: doing
Owner: opencode-media-20261110
Mode: mvp
Lane: http
Scope: http
Risk: medium
Parallel: true
Depends on: 0024,0013
Tags: planned,m2,public-api

## Goal

Expose the three core catalog read endpoints through shared library services.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/application-contract.md

## Files allowed to edit

- internal/httpapi/library.go
- internal/httpapi/library_test.go
- internal/httpapi/routes.go
- internal/httpapi/server.go
- internal/runtime/runtime.go
- internal/runtime/runtime_test.go
- docs/progress.md
- .tasks/0025-expose-catalog-search-and-detail-http-reads.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Add authenticated GET works/work/edition routes and bounded query decoding
2. Translate common results/errors using v1 DTOs and parameter validation
3. Create httptest coverage for aliases, filters, cursor mismatch, unknown ID and auth

## Acceptance criteria

- HTTP returns the same catalog IDs/current fields as CLI service calls
- API 404 returns JSON and never SPA content
- No HTTP-owned search/identity rules

## Invariants

- Read routes have no mutation side effects

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/httpapi -run \"TestHTTPLibrary\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M2. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
- Scope amendment (2026-11-12): the catalog routes join the server mux and the serve lifecycle opens the shared-lifetime database and passes the app service to the server, so internal/httpapi/server.go and internal/runtime/runtime.go join the allowed list.
- Scope amendment (2026-11-12, second): serve now fails before listen when the state directory has no initialized database, so its regression test joins the allowed list (internal/runtime/runtime_test.go).
