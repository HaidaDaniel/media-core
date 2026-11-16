# Task 0058 - Make the release manifest and cross-build report truthful

State: done
Owner: opencode-media-fix-20261116
Mode: mvp
Lane: packaging
Scope: packaging
Risk: medium
Parallel: true
Depends on: 0047
Tags: planned,m5,review-fix

## Goal

Fix the self-referential web-asset manifest and either implement or retract the claimed cross-architecture format check.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/architecture.md
- docs/engineering/deployment.md
- scripts/build-release.sh

## Files allowed to edit

- scripts/build-release.sh
- tests/packaging/packaging_test.go
- docs/engineering/deployment.md
- docs/progress.md
- .tasks/0058-make-the-release-manifest-and-cross-build-report-truthful.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- internal/webassets/dist/**

## Steps

1. Generate the asset manifest into a path outside the staged tree or exclude `manifest.json` from the enumeration so it cannot describe itself
2. Implement an actual ELF format check for the non-host artifact and report its result, or change both the script message and deployment doc to say the artifact is linked but not executed/verified
3. Add a packaging regression that validates every manifest entry resolves to a staged file with matching size and hash

## Acceptance criteria

- Every manifest entry names a real staged file with its final size and SHA-256; there is no self-entry
- The release output and docs/engineering/deployment.md make the same claim about non-host artifacts
- All failure paths still restore the tracked embed placeholder and leave the tree clean

## Invariants

- A manifest never includes itself, and build output never overstates verification it did not perform.

## Review questions

- Which option was chosen for the cross-build claim, and do script output and documentation agree?

## Counterexample searches

- Run the build in a temp checkout and verify each manifest hash against the staged file, including after an intentional mid-build failure.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"bash -n scripts/build-release.sh && go test ./tests/packaging -run TestPackageReleaseArtifacts"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Post-implementation review remediation of release artifact integrity and truthfulness.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
