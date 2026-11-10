# Task 0004 - Refine product architecture and implementation plan

State: doing
Owner: codex-media-plan-20261110
Mode: mvp
Lane: planning
Scope: docs,planning
Risk: medium
Parallel: false
Depends on: none
Tags: docs,planning,broad-scope

## Goal

Revise personal library documentation and shared CLI HTTP MCP architecture; verify a fine-grained unstarted implementation graph without application code.

## Context files

- AGENTS.md
- docs/project.md
- docs/scope.md
- docs/architecture.md
- docs/domain-model.md
- docs/decisions.md
- docs/task-system.md
- docs/context-system.md

## Files allowed to edit

- README.md
- docs/**
- .tasks/**
- scripts/check-planning.py

## Files forbidden to edit



## Steps

1. Inspect current contracts and policy
2. Revise documents with explicit superseding ADRs
3. Create narrow todo tasks and coverage matrix
4. Verify committed plan and obtain fresh-context review

## Acceptance criteria

- Both Web UI and MCP are MVP requirements
- Task dependencies are acyclic and requirements have owners
- No application code is implemented

## Invariants

- Preserve old task history and ownership
- Future implementation tasks remain unclaimed todo

## Review questions

- Do the complete docs and task graph describe one coherent MVP?

## Verification

- `{"id":"planning","type":"automated","required":true,"environment":"local","profile":"report","command":"python3 scripts/check-planning.py"}`
- `{"id":"apk-lint","type":"automated","required":true,"environment":"local","profile":"deterministic","apkOperation":"lint"}`
- `{"id":"apk-doctor","type":"automated","required":true,"environment":"local","profile":"deterministic","apkOperation":"doctor"}`
- `{"id":"apk-sync","type":"automated","required":true,"environment":"local","profile":"deterministic","apkOperation":"sync-check"}`
- `{"id":"whitespace","type":"automated","required":true,"environment":"local","profile":"deterministic","command":"git diff --check HEAD^ HEAD"}`

## Documentation updates

- docs/progress.md

## Notes


