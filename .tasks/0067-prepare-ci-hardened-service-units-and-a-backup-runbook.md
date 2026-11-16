# Task 0067 - Prepare CI, hardened service units and a backup runbook

State: todo
Owner: none
Mode: mvp
Lane: reliability
Scope: reliability,packaging,docs
Risk: high
Parallel: false
Depends on: 0047,0058,0066
Tags: planned,m5,audit,p2

## Goal

Provide hosted CI, a separate release gate, safe example systemd units, a documented first-run setup and a regular backup with restore smoke — preparation only, with no deployment or publication.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/decisions.md
- docs/engineering/deployment.md
- docs/engineering/backup-restore.md
- scripts/build-release.sh
- package.json
- web/package.json

## Files allowed to edit

- .github/workflows/ci.yml
- .github/workflows/release-gate.yml
- deploy/media-core.service
- deploy/media-core-backup.service
- deploy/media-core-backup.timer
- scripts/backup-smoke.sh
- docs/decisions.md
- docs/engineering/deployment.md
- docs/progress.md
- .tasks/0067-prepare-ci-hardened-service-units-and-a-backup-runbook.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- release/**
- internal/webassets/dist/**
- web/dist/**

## Steps

1. Add a CI workflow running Go build, vet, tests, race tests, shell syntax check, TypeScript, Vitest, Playwright and the release-artifact gate; pin action versions and use least-privilege permissions
2. Add a separate release-gate workflow that re-runs the checks and produces local artifacts only: no tag, push, GitHub Release, registry upload or secret use
3. Add hardened example units for `serve` and for scheduled backup with a timer: explicit user/group, no new privileges, read-only system where possible, writable paths limited to the state and backup directories, restart policy, no secret values
4. Add a backup smoke script that creates an archive and restores it into a fresh temporary state directory with `restore --force`, then add a documented first-run runbook (`init`, `owner generate`, `serve`) and the backup/timer procedure to deployment.md
5. Record the hosted-CI decision in docs/decisions.md as a new ADR without rewriting accepted history

## Acceptance criteria

- CI declares every required check and can be parsed as valid YAML; Playwright installs its browser before running
- The release gate is a distinct workflow and contains no publish/tag/upload step
- Units contain no secret material and restrict filesystem write access to explicit paths
- Backup smoke never touches a live state directory and proves a restored database is readable
- Documentation matches the actual flags and secret-generation command

## Invariants

- CI and deployment preparation never deploy to a NAS, create a release, or modify operator data.

## Review questions

- Could the release-gate workflow accidentally publish an artifact or require repository write permission?

## Counterexample searches

- Inspect workflow triggers/permissions/steps for publish actions; run the backup smoke script against a scratch state directory.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"python3 -c \"import yaml,sys; [yaml.safe_load(open(p)) for p in sys.argv[1:]]\" .github/workflows/ci.yml .github/workflows/release-gate.yml && bash -n scripts/build-release.sh scripts/backup-smoke.sh && go test ./internal/backup -run TestRestore"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Confirmed P2: no CI, no service units, no automated backup/restore smoke and no consolidated first-run runbook. Hosted CI is explicitly authorized by the operator for this audit task.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
