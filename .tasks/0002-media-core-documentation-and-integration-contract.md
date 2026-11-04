# Task 0002 - Media Core documentation and integration contract

State: done
Owner: opencode-primary
Mode: mvp
Lane: planning
Scope: docs,product,architecture,integration
Risk: low
Parallel: false
Depends on: none
Tags: none

## Goal

Create the minimal accepted documentation for Media Core: personal media history, progress, statuses, ratings and notes, WN/LN/manga/anime links, agent-first local CLI/MCP, Go + SQLite, zero idle resources, and future content delivery concepts. Documentation only; no business logic or code.

## Context files

- AGENTS.md
- docs/project.md
- docs/scope.md
- docs/architecture.md
- docs/decisions.md

## Files allowed to edit

- README.md
- docs/project.md
- docs/scope.md
- docs/architecture.md
- docs/domain-model.md
- docs/roadmap.md
- docs/integration-contract.md
- docs/decisions.md
- docs/progress.md

## Files forbidden to edit



## Steps

1. Write README, project, scope, architecture, domain model, roadmap, decisions, progress and integration contract.
2. Define ContentReference, ContentAsset, ContentProvider, DeliveryTarget, original/translation links and the external translation harness boundary without implementing them.
3. Align the future Media Core event import with the published Release Watcher schema_version 1.0 contract.

## Acceptance criteria

- Documents define purpose, users, non-goals, domain concepts, local-first/agent-first architecture, and a dependency-ordered roadmap.
- No business logic, runtime code, second database or shared SQLite file is introduced.
- The integration contract is compatible with Release Watcher event_id/dedupe/schema_version semantics.

## Verification

- `{"id":"apk-lint","type":"automated","required":true,"environment":"local","profile":"deterministic","apkOperation":"lint"}`

## Documentation updates

- Update docs/progress.md.

## Notes

- This is documentation only. Content delivery, translation execution and provider integration are future modules.
