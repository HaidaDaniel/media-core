# Task 0006 - Resolve explicit configuration and local state paths

State: done
Owner: opencode-media-20261110
Mode: mvp
Lane: foundation
Scope: foundation
Risk: medium
Parallel: true
Depends on: 0005
Tags: planned,m1

## Goal

Implement deterministic flag/environment/default resolution and secure state path handling.

## Context files

- AGENTS.md
- docs/project.md
- docs/architecture.md
- docs/task-system.md

## Files allowed to edit

- internal/config/**
- internal/runtime/runtime.go
- docs/engineering/configuration.md
- docs/progress.md
- .tasks/0006-resolve-explicit-configuration-and-local-state-paths.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md

## Steps

1. Define state directory and listener/public-origin configuration values
2. Validate canonical local paths and redact owner/TLS secrets in diagnostics
3. Create configuration tests for precedence, XDG fallback, invalid and symlinked paths; document settings

## Acceptance criteria

- Explicit flags override env and defaults without writing state for version/doctor
- Invalid listener/TLS combinations are rejected by validation, with enforcement in 0024
- No checkout-relative default or secret logging

## Invariants

- Configuration cannot silently select a public listener

## Review questions

- Does failure leave the prior state intact and the error distinguishable?

## Counterexample searches

- Exercise each invalid, stale, retry and correction path listed above using a temporary state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"go test ./internal/config"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M1. See docs/implementation-plan.md for requirements and graph.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
