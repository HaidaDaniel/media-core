# Task 0065 - Prevent late session inspection from clearing the CSRF token

State: todo
Owner: none
Mode: mvp
Lane: web
Scope: web
Risk: medium
Parallel: false
Depends on: 0030,0050
Tags: planned,m5,audit,p2

## Goal

Make the initial `inspectSession()` result apply only to the request generation that started it, so it can never overwrite or clear a token established by a later successful login.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/web-ui.md
- web/src/api/client.ts
- web/src/auth/AuthContext.tsx

## Files allowed to edit

- web/src/api/client.ts
- web/src/api/client.test.ts
- web/src/auth/AuthContext.tsx
- web/src/auth/AuthContext.test.tsx
- docs/progress.md
- .tasks/0065-prevent-late-session-inspection-from-clearing-the-csrf-token.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- web/src/shell/Shell.tsx

## Steps

1. Reproduce with controlled promises: start inspect, complete `login()` successfully, then reject or resolve the inspection; show that the client clears or replaces the token after login
2. Add a generation guard: the inspection captures its generation and applies its CSRF/status result only if login/logout has not advanced it
3. Add tests for a late rejection and a late successful inspection after login, asserting the login token stays active and the status stays authenticated
4. Keep the existing first-load inspect, logout and failed-logout behavior unchanged

## Acceptance criteria

- A late GET `/session` response cannot clear or replace the CSRF token set by a completed POST `/session`
- A late GET failure cannot change status or token after a successful login or logout
- Normal initial inspection still authenticates, reports anonymous and refreshes the token

## Invariants

- Session state updates are ordered by request generation, not by response arrival.

## Review questions

- Does the guard also cover a logout that races the initial inspection?

## Counterexample searches

- Order GET-after-POST, failed GET-after-POST, GET-after-DELETE and failed GET-after-DELETE with deferred promises.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec vitest run src/auth src/api"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Confirmed P2: `inspectSession()` clears the in-memory CSRF token unconditionally on failure, and the client applies a late GET token without a generation check.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
