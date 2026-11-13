# Task 0026 - Expose quick-add and catalog correction HTTP writes

State: done
Owner: opencode-media-20261110
Mode: mvp
Lane: http
Scope: http
Risk: medium
Parallel: true
Depends on: 0024,0012,0013
Tags: planned,m2,public-api

## Goal

Expose resolve/create/add-edition and narrow catalog corrections with explicit identity choice.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/application-contract.md

## Files allowed to edit

- internal/httpapi/catalog_writes.go
- internal/httpapi/catalog_writes_test.go
- internal/httpapi/routes.go
- docs/progress.md
- .tasks/0026-expose-quick-add-and-catalog-correction-http-writes.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Decode typed bodies and reject unknown fields; populate trusted actor and idempotency header
2. Bind documented catalog routes to shared services with no new validation policy
3. Test candidate round trip, existing identity, stale revision, replay and invalid fields

## Acceptance criteria

- No write occurs on duplicate-candidate response or failed confirmation
- Manual entry works without external calls
- Wrong owner session/CSRF cannot create or archive entries

## Invariants

- Transport never treats a title match as confirmed identity

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/httpapi -run \"TestHTTPCatalogWrite\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M2. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
