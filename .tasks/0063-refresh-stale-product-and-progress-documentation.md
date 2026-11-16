# Task 0063 - Refresh stale product and progress documentation

State: todo
Owner: none
Mode: mvp
Lane: planning
Scope: docs
Risk: medium
Parallel: false
Depends on: 0050,0052,0053,0054,0055,0056,0057,0058,0059,0060,0061,0062
Tags: planned,m5,review-fix

## Goal

Align the repository's top-level documentation with the implemented system and the remediation tasks.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/project.md
- docs/scope.md
- docs/architecture.md
- docs/progress.md
- README.md

## Files allowed to edit

- README.md
- docs/project.md
- docs/scope.md
- docs/architecture.md
- docs/progress.md
- docs/task-system.md
- .tasks/0063-refresh-stale-product-and-progress-documentation.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- docs/implementation-plan.md
- docs/decisions.md
- scripts/check-planning.py

## Steps

1. Remove the "planning only / no application runtime" claims and the APK 0.5.2 / "start with task 0005" instructions from README.md and docs/project.md
2. Move Streamable HTTP MCP out of the deferred list and reconcile the related architecture paragraph with task 0048 and the Origin-protection fix
3. Rewrite the progress sections that still describe an unstarted implementation or claim no tests were run; record the implemented milestones and the remediation tasks
4. Update the planning checks in README.md to name commands that pass after the remediation, and keep local links valid

## Acceptance criteria

- No tracked document claims the application, Web UI, MCP or tests do not exist
- Scope and architecture agree on the transport support that actually shipped
- Progress states the real task lifecycle and current verification evidence without inventing deployment or review results
- All local Markdown links resolve

## Invariants

- Documentation is source of truth, so it must not contradict the committed implementation or task graph.

## Review questions

- Are any remaining "future command" disclaimers still accurate for the task files they appear in?

## Counterexample searches

- Search the docs for the obsolete phrases and for links to files that were renamed or removed.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"python3 scripts/check-planning.py && git diff --check"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Post-implementation review remediation of stale documentation; this task follows all behavior fixes so its claims are final.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
