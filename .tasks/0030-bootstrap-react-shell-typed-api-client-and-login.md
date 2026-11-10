# Task 0030 - Bootstrap React shell typed API client and login

State: todo
Owner: none
Mode: mvp
Lane: web
Scope: web
Risk: medium
Parallel: true
Depends on: 0007,0024
Tags: planned,m2

## Goal

Create lightweight React/Vite routing shell with common fetch/session/error handling.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md
- docs/web-ui.md
- docs/application-contract.md

## Files allowed to edit

- web/package.json
- web/pnpm-lock.yaml
- web/tsconfig*.json
- web/vite.config.ts
- web/index.html
- web/src/main.tsx
- web/src/App.tsx
- web/src/api/**
- web/src/auth/**
- web/src/styles.css
- web/src/shell/**
- docs/decisions.md
- docs/progress.md

- .tasks/0030-bootstrap-react-shell-typed-api-client-and-login.md

- .tasks/0030-bootstrap-react-shell-typed-api-client-and-login.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Pin React/TS/Vite and minimal testing/router tooling with ADR dependency note
2. Implement typed fetch client against frozen fixtures plus session login/logout and in-memory CSRF
3. Create shell/client tests, explicit build/typecheck/test scripts and responsive navigation

## Acceptance criteria

- No business validation duplicated in React and no provider/cover prerequisite
- Unauthorized/retryable/conflict states preserve form input and show useful messages
- No credential in build env/storage; frontend scripts are newly created by this task

## Invariants

- Production server remains Go; Node only needed to develop/build

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"pnpm --dir web exec tsc --noEmit && pnpm --dir web test -- --run && pnpm --dir web build"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.
- Update docs/progress.md on lifecycle changes.


## Notes

- Milestone: M2. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.

