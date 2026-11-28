# Task 0082 - Fix planning contract for the APK upgrade task

State: done
Owner: opencode-apk-upgrade-20261128
Mode: mvp
Lane: maintenance
Scope: docs,tasks,planning
Risk: low
Parallel: false
Depends on: 0081
Tags: planning,contract,apk,tooling,ci

## Goal

Restore the repository planning contract after the APK v0.6.2 upgrade by completing task 0081 metadata and adding the required inventory and requirement coverage for tasks 0081 and 0082.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/implementation-plan.md
- docs/product/requirements.md
- scripts/check-planning.py
- .tasks/0081-upgrade-agenticprojectkit-to-v062.md

## Files allowed to edit

- .tasks/0081-upgrade-agenticprojectkit-to-v062.md
- .tasks/0082-fix-planning-contract-for-the-apk-upgrade-task.md
- docs/implementation-plan.md
- docs/progress.md
- docs/engineering/apk-tooling.md

## Files forbidden to edit

- cmd/**
- internal/**
- web/**
- deploy/**
- scripts/**
- go.mod
- go.sum
- package.json
- pnpm-lock.yaml
- .github/workflows/**
- release/**
- tmp/**

## Steps

1. Set task 0081 Mode to mvp and add the Milestone M5 line and the Invariants, Review questions and Counterexample searches sections without changing its done state or evidence.
2. Add inventory rows for tasks 0081 and 0082 to docs/implementation-plan.md with matching title, dependencies, risk and milestone.
3. Add tasks 0081 and 0082 to a requirement coverage owner list in docs/implementation-plan.md.
4. Add the same required sections, mode and Milestone M5 line to task 0082.
5. Run python3 scripts/check-planning.py and apk lint, doctor, sync and status until all pass.
6. Commit the planning fix, run candidate-bound verification, obtain a fresh independent review, gate and complete task 0082.
7. Push the fix to main and confirm CI succeeds.

## Acceptance criteria

- python3 scripts/check-planning.py passes with all task IDs, inventory rows, requirement owners and local links valid.
- Task 0081 remains done with its evidence unchanged; only missing contract metadata is completed.
- Task 0082 carries mode mvp, the M5 milestone and the required Invariants, Review questions and Counterexample searches sections.
- No runtime, dependency, lockfile or generated-instruction file changes.
- apk lint, doctor, sync and status pass and the task gate passes.
- Hosted CI on main succeeds after the fix is pushed.

## Invariants

- Task 0081 stays done with its gate subject and evidence unchanged; only missing structural contract metadata is completed.
- No runtime, dependency, lockfile, generated instruction, release or deployment file changes.
- Inventory and requirement coverage stay consistent with the actual task set present on main.
- The planning check remains a static structural check, not an application or deployment claim.

## Review questions

- Does `scripts/check-planning.py` pass for every task in this checkout, including 0081 and 0082?
- Are the inventory title, dependencies, risk and milestone for 0081 and 0082 accurate?
- Does every task in the inventory have a requirement owner, and does every requirement owner exist?
- Did the fix avoid changing 0081 evidence, state or runtime files?
- Could the fix hide a real contract problem instead of correcting it?

## Counterexample searches

- Run `python3 scripts/check-planning.py` before and after the fix; require the specific 0081 mode/section/inventory/ownership errors to disappear.
- Compare the inventory row values against the task files field by field.
- Diff `cmd/**`, `internal/**`, `web/**`, `scripts/**`, `package.json`, `pnpm-lock.yaml` and `go.mod`; require no change.
- Confirm task 0081 remains `done` and its recorded gate evidence still refers to the reviewed candidate.

## Verification

- `{"id":"planning-links","type":"automated","required":true,"environment":"local","profile":"report","command":"python3 scripts/check-planning.py"}`
- `{"id":"apk-lint","type":"automated","required":true,"environment":"local","profile":"report","apkOperation":"lint"}`
- `{"id":"apk-doctor","type":"automated","required":true,"environment":"local","profile":"report","apkOperation":"doctor"}`
- `{"id":"apk-sync","type":"automated","required":true,"environment":"local","profile":"report","apkOperation":"sync-check"}`
- `{"id":"apk-status","type":"automated","required":true,"environment":"local","profile":"report","apkOperation":"status"}`

## Documentation updates

- docs/implementation-plan.md: inventory and requirement coverage for tasks 0081 and 0082
- docs/progress.md: record the planning-contract remediation

## Notes

- Milestone: M5. Planning-contract remediation after the APK v0.6.2 upgrade.
- This fixes a task-contract error: the generated upgrade task was created in maintenance mode without the repository-required planning sections and did not include scripts/check-planning.py in its verification, so hosted CI structural checks failed after integration.
- None is claimed to pass today before the candidate is verified.
- Do not alter task 0081 state, gate subject or evidence; complete only its structural contract metadata.
- No runtime, dependency, lockfile, release or deployment change.
