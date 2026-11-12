# Task 0024 - Secure HTTP lifecycle and single-owner session boundary

State: doing
Owner: opencode-media-20261110
Mode: mvp
Lane: http
Scope: http
Risk: high
Parallel: true
Depends on: 0006,0007,0009
Tags: planned,m2,security

## Goal

Create loopback serve mode with real session/CSRF/Origin/Host protections before library routes.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/application-contract.md

## Files allowed to edit

- internal/httpapi/server.go
- internal/httpapi/security.go
- internal/httpapi/session.go
- internal/httpapi/server_test.go
- internal/httpapi/security_test.go
- internal/httpapi/session_test.go
- internal/cli/owner.go
- internal/cli/owner_test.go
- internal/cli/cli.go
- internal/runtime/runtime.go
- docs/engineering/configuration.md
- docs/progress.md
- .tasks/0024-secure-http-lifecycle-and-single-owner-session-boundary.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Wire Go HTTP lifecycle/timeouts/body limits and explicit TLS non-loopback guard
2. Generate/read owner secret through init/local command and implement bounded login/logout/expiry
3. Test missing secret, CSRF, hostile Origin/Host, rate limit, cookie flags and graceful shutdown

## Acceptance criteria

- No unauthenticated writable library endpoint and no credentials in JS/logs
- Remote bind without explicit allow + HTTPS origin + TLS fails before listen
- Create security report evidence from focused tests; no real host deployment

## Invariants

- Local HTTP authentication is not bypassed by missing Origin

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/httpapi ./internal/cli -run \"Test(HTTPServer|Session|Security|Owner)\""}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M2. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
- Scope amendment (2026-11-12): the owner command joins the CLI usage text, so internal/cli/cli.go joins the allowed list; internal/cli/owner_test.go carries the owner-command tests.
