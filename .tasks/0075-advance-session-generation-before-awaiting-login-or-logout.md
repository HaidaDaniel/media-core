# Task 0075 - Advance session generation before awaiting login or logout

State: done
Owner: opencode-media-fix-20261116
Mode: mvp
Lane: web
Scope: web
Risk: medium
Parallel: false
Depends on: 0065
Tags: planned,m5,final-audit,p2

## Goal

Close the narrow window in which a mount-time session inspection can still overwrite the token set by a login whose POST response has resolved but whose generation has not yet advanced.

## Context files

- AGENTS.md
- docs/task-system.md
- web/src/auth/AuthContext.tsx
- web/src/auth/AuthContext.test.tsx

## Files allowed to edit

- web/src/auth/AuthContext.tsx
- web/src/auth/AuthContext.test.tsx
- docs/progress.md
- .tasks/0075-advance-session-generation-before-awaiting-login-or-logout.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- web/src/api/client.ts
- web/src/api/client.test.ts

## Steps

1. Advance the generation synchronously at the start of login and logout, before the awaited network call
2. On a failed action, restore the previous generation only when no later action replaced it, so the initial inspection can still settle
3. Add a controlled-order test where the inspection resolves while the login promise is still pending, and a test where a failed login still lets the inspection settle
4. Keep the existing late-response guard tests green

## Acceptance criteria

- No stale GET response can apply or clear the token once a login/logout has started
- A failed login or logout does not leave the UI stuck in `inspecting`
- Existing generation-guard behavior and successful logout semantics are unchanged

## Invariants

- A session action owns the generation from its start, not from its response.

## Review questions

- Can a failed action restoring the generation undo a later successful action?

## Counterexample searches

- GET resolves between POST resolution and post-await code; failed login; failed logout; concurrent login/logout.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec vitest run src/auth src/api"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Narrow P2 race found in final reliability review: generation advanced after `await`.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
