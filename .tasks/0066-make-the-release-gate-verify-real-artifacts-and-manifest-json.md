# Task 0066 - Make the release gate verify real artifacts and manifest.json

State: todo
Owner: none
Mode: mvp
Lane: packaging
Scope: packaging
Risk: medium
Parallel: false
Depends on: 0047,0058
Tags: planned,m5,audit,p2

## Goal

Fail the release gate when artifacts or the web-asset manifest are absent, and verify the manifest against real staged files at runtime.

## Context files

- AGENTS.md
- docs/task-system.md
- docs/engineering/deployment.md
- scripts/build-release.sh
- tests/packaging/packaging_test.go

## Files allowed to edit

- scripts/build-release.sh
- tests/packaging/packaging_test.go
- docs/engineering/deployment.md
- docs/progress.md
- .tasks/0066-make-the-release-gate-verify-real-artifacts-and-manifest-json.md

## Files forbidden to edit

- AGENTS.md
- .agentic/config.json
- CLAUDE.md
- GEMINI.md
- release/**
- internal/webassets/dist/**
- web/dist/**

## Steps

1. Replace the static Bash-string assertion with a Go verifier that parses the actual manifest.json in a staging directory: format marker, complete file set, no self-entry, size and SHA-256 for every entry
2. Invoke that verifier from build-release.sh while the real staged files exist, and set a gate flag only for the script's own verification run
3. Make TestPackageReleaseArtifacts fail (not skip) when the gate flag is set and either release artifact is missing; ordinary local `go test ./...` may still skip when artifacts are absent
4. Run the gate with artifacts hidden to prove it fails, then restore them and prove it passes

## Acceptance criteria

- With no release artifacts, the release gate exits non-zero
- With a missing, malformed, self-referential or hash-mismatched manifest, the verifier exits non-zero
- With real artifacts and a correct manifest, all checks pass
- No runtime check is replaced by searching for literal strings in the shell script

## Invariants

- A release gate proves the presence and integrity of real local artifacts; it never publishes them.

## Review questions

- Does the manifest verifier cover extra on-disk files as well as missing manifest entries?

## Counterexample searches

- Hide both binaries; delete manifest.json; add a self-entry; corrupt a size and a hash; add an extra staged file.

## Verification

- `{"id":"focused","type":"automated","required":true,"environment":"local","profile":"report","command":"bash -n scripts/build-release.sh && go test ./tests/packaging -run TestPackageReleaseArtifacts -count=1"}`

## Documentation updates

- Update docs/progress.md on lifecycle changes.

## Notes

- Milestone: M5. Confirmed P2: the packaging gate skipped when artifacts were absent, and the previous manifest test only searched Bash source text.
- Unstarted implementation contract. Tests/commands above are to be created or made runnable by this task; none is claimed to pass today.
- Before editing, read existing allowed source files supplied by completed dependencies. Missing paths are new files owned by this task, not existing context.
- Only listed paths plus this task file may change. Do not edit other task contracts or generated agent instructions; task lifecycle is managed by APK.
- Keep contract behavior fixed; if a counterexample requires expanding scope, update this task before source edits. No actual host deployment, push or publication is authorized.
