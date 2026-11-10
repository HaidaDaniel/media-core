# Task 0047 - Embed Web assets and verify complete deployable backup round trip

State: todo
Owner: none
Mode: mvp
Lane: packaging
Scope: packaging
Risk: high
Parallel: false
Depends on: 0023,0024,0033,0035,0036,0041,0043,0044,0045,0046,0049
Tags: planned,m5

## Goal

Produce local release artifacts with embedded UI and verify complete-state restore without deploying.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/product/requirements.md

## Files allowed to edit

- internal/webassets/**
- internal/httpapi/assets.go
- internal/httpapi/assets_test.go
- internal/runtime/runtime.go
- scripts/build-release.sh
- tests/packaging/**
- tests/acceptance/backup_test.go
- docs/engineering/deployment.md
- docs/engineering/backup-restore.md
- .gitignore
- internal/app/export.go
- internal/app/export_test.go
- docs/progress.md

- .tasks/0047-embed-web-assets-and-verify-complete-deployable-backup-round-trip.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Build frontend then stage embed assets; build CGO-disabled Linux amd64/arm64 binaries with truthful identity
2. Test static fallback/cache/CSP and CLI/MCP operation without Node; document local service/TLS/SSH setup
3. Extend scenario F to all data tables including relations/notes/imports; restore isolated DB and compare Web/MCP views
4. Register all M5 tables in readable export and test complete export coverage as well as archive restore

## Acceptance criteria

- Artifact contains API/UI and CLI/MCP in one binary; no runtime Node/Docker
- Complete application-visible state/history survives backup/restore and destination secrets remain intact
- No real host deployment, publish/tag/push or invented license; unsupported architecture smoke limits reported

## Invariants

- New build script must build/test local artifacts only and never publish

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"bash scripts/build-release.sh && go test ./internal/httpapi ./tests/packaging ./tests/acceptance -run \"Test(Assets|Package|ScenarioF)\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M5. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
