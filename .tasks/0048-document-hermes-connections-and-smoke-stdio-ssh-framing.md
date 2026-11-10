# Task 0048 - Document Hermes connections and smoke stdio SSH framing

State: todo
Owner: none
Mode: mvp
Lane: mcp
Scope: mcp
Risk: medium
Parallel: false
Depends on: 0038,0039,0040,0041,0047
Tags: planned,m5

## Goal

Provide usable same-host/remote-host Hermes instructions and a deterministic connection smoke.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/hermes.md

## Files allowed to edit

- docs/hermes.md
- tests/acceptance/hermes_test.go
- tests/fixtures/ssh-stdio/**
- docs/progress.md

- .tasks/0048-document-hermes-connections-and-smoke-stdio-ssh-framing.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Check installed Hermes config support or label example pending actual client validation
2. Exercise built binary initialization/list/create/search/progress/restart against temp DB
3. Test SSH no-PTY fixed-command framing in an isolated fixture; document host-key/account/secret boundaries

## Acceptance criteria

- Local MCP needs no HTTP/Node/provider
- Different-host instructions use verified SSH and NAS-local SQLite, not a DB mount
- Report real-client/NAS checks as unrun unless actually authorized/performed

## Invariants

- No real host deployment or arbitrary remote command construction

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./tests/acceptance -run \"TestHermes\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M5. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.

