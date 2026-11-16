# Task 0060 - Freeze the QuickAdd retry payload with its request id

State: todo
Owner: none
Mode: mvp
Lane: web
Scope: web
Risk: medium
Parallel: true
Depends on: 0050,0032
Tags: planned,m5,review-fix

## Goal

Prevent the QuickAdd Retry action from reusing an idempotency key after the form payload has changed.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/web-ui.md
- web/src/add/QuickAddPage.tsx

## Files allowed to edit

- web/src/add/QuickAddPage.tsx
- web/src/add/add.test.tsx
- docs/progress.md
- .tasks/0060-freeze-the-quickadd-retry-payload-with-its-request-id.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- web/src/api/client.ts

## Steps

1. Store the exact submitted payload in the create action state when the create request is first issued
2. Make Retry resend that frozen payload with the same `request_id`; mint a new key only for a genuinely new attempt after an explicit user action
3. Either disable form editing while in the create-error state or present a distinct "edit and resolve again" path that creates a new key
4. Add a test that edits a field after an ambiguous failure and asserts Retry sends the original payload

## Acceptance criteria

- Retry always sends byte-equivalent payload and the original `request_id`
- A user-edited form cannot produce a same-key/different-payload conflict
- The existing idempotent-replay failure UX and retained input behavior are preserved

## Invariants

- An idempotency key is bound to one canonical payload for its whole lifetime.

## Review questions

- Does the fresh-attempt path reuse a stale candidate token that has since expired?

## Counterexample searches

- Simulate an ambiguous create failure, edit the title, click Retry, and confirm the request fingerprint is unchanged.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec vitest run src/add"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Post-implementation review remediation of client-side idempotency-key drift.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
