# AgenticProjectKit tooling

Media Core uses [AgenticProjectKit](https://github.com/HaidaDaniel/AgenticProjectKit) only as
development tooling for task lifecycle, evidence and generated instructions. It is not a Go
module, is not imported by application code, and is not part of the runtime, the embedded web
bundle or any release artifact.

## Pinned version

- `package.json` declares `agentic-project-kit` as `github:HaidaDaniel/AgenticProjectKit#v0.6.2`
  (previously `#v0.6.0`).
- The tag `v0.6.2` is an annotated tag object `bedff6e5b8b83794e8614b0dca1c0ca674adc2ed` that
  peels to commit `b4df405205e6ea66ae88c1093ea9bd6ae28080a0`.
- `pnpm-lock.yaml` resolves the package from the codeload tarball at exactly that commit, so the
  install is pinned to the reviewed commit rather than floating `main`.
- Upgrade command used with the existing package manager:
  `pnpm add -D agentic-project-kit@git+https://github.com/HaidaDaniel/AgenticProjectKit.git#v0.6.2`.
  pnpm normalizes the specifier to the same `github:...#v0.6.2` form as before.
- `pnpm install --frozen-lockfile` succeeds and leaves `pnpm-lock.yaml` unchanged.
- The repository-local executable `node_modules/.bin/apk` reports `agentic-project-kit 0.6.2`.
- The 0.6.2 package ships `handlebars` as its only runtime dependency; the previous 0.6.0
  transitive `@types/node`, `typescript` and `undici-types` entries are removed. This is the
  documented 0.6.1 dependency-narrowing change, not an unrelated dependency edit.

## Upgrade record (2026-11-28)

- Starting point: `main` / `origin/main` at `b62b2d136788b3992bef92b057222357d475dfea`.
- The upgrade was performed on branch `chore/apk-0.6.2` in a separate Git worktree, so the
  existing `feature/dark-ui-covers-0079` checkout, its tasks 0079/0080 and its local journals
  were not modified. Runtime code is untouched.
- Backups were written outside the repository (refs, bundles, tasks, journals, manifests and
  generated instructions).
- `apk sync` stayed in sync before and after the upgrade; no generated instruction file needed
  rewriting, so project-specific guidance in `docs/` and the generated `AGENTS.md`, `CLAUDE.md`
  and `GEMINI.md` are preserved.
- `apk adopt --apply` added the two canonical operational ignore entries introduced by 0.6.2
  (`.agentic/task-reconciliations.jsonl`, `.agentic/task-recoveries.jsonl`) to `.gitignore`.

## Diagnostics before and after

Captured in the isolated worktree with the upgrade task claimed (79 task files). Wall-clock
timings are single observations on this host, not a benchmark.

| command | 0.6.0 exit | 0.6.0 ms | 0.6.2 exit | 0.6.2 ms |
| --- | --- | --- | --- | --- |
| `apk --version` | 0 | 417 | 0 | 406 |
| `apk doctor` | 0 | 795 | 0 | 647 |
| `apk sync` | 0 | 653 | 0 | 556 |
| `apk lint --json` | 0 | 720 | 0 | 716 |
| `apk audit` | 0 | 526 | 0 | 608 |
| `apk status` | 0 | 661 | 0 | 778 |
| `apk status --detail` | 0 | 816 | 0 | 776 |
| `apk workers --json` | 0 | 498 | 0 | 467 |
| `apk quality detect --json` | 0 | 506 | 0 | 479 |

Doctor keeps the same four advisory warnings before and after (`application-discovery`,
`quality 4/7`, `license`, `env`) and adds a passing `ignore-entries` check after adoption.
`apk lint`, `apk doctor`, `apk sync` and `apk status` exit 0 in both versions. 0.6.2 lint
reports zero findings; the 0.6.0 lint capture additionally emitted one info-level
`context-reference-missing` note for the generated `docs/project-map.md` that was not present
at capture time, which is a capture-order artifact rather than an upgrade regression. With two
worktrees present, 0.6.2 emits a bounded diagnostic that lifecycle metadata
was read from two linked worktrees while task identity stayed bound to its canonical worktree,
baseline and candidate. That warning is expected while the isolated upgrade worktree exists.

## Historical task re-validation

The completed tasks 0079 and 0080 were re-read read-only with the 0.6.2 binary against the
untouched feature worktree. Their subject hashes and verdicts are unchanged:

- 0080 gate: `pass` under both 0.6.0 and 0.6.2.
- 0079 gate: `blocked` under both. The blockers are later 0080 files that are outside the 0079
  scope and verification/review evidence bound to a different candidate revision. This is the
  documented historical artifact of completing 0079 before 0080; it is not a live failure. The
  tasks were not reopened and their evidence was not rewritten.
- Task files, `evidence.jsonl` and `task-baselines.jsonl` in the feature worktree are
  byte-identical to the pre-upgrade backup.

## Isolated lifecycle and multi-worktree fixture

A throwaway Git repository was initialized with the 0.6.2 binary and exercised:
`init` → `agent register` → `task create` → `claim` → `verify` → `review` (different identity)
→ `gate pass` → `done`. A linked `git worktree` was created for the same fixture. From the
linked checkout, `apk status` read lifecycle metadata read-only and warned that identity stayed
bound to the canonical worktree, and both `apk release` and `apk done` failed closed (the claim
could not be mutated from the sibling checkout). No fixture failure indicated an APK defect.

## Upstream observation

The `v0.6.2` tag and peeled commit requested for this upgrade exist and match. At the time of
this upgrade the upstream release index still marks `v0.6.1` as the last validated release and
the `v0.6.2` note still says post-tag validation is pending. Media Core pins the requested tag
commit; the upstream validation bookkeeping is an upstream concern and is not a blocker here.

## Verification

Go build, vet and tests ran from the upgrade worktree after the change and passed. Canonical,
candidate-bound evidence is recorded by `apk task verify` through the task gate. Web checks
were not rerun here because no web or runtime file changed; hosted CI on `main` runs the full
Go/web/release suite after integration. No deployment, tag, release or NAS change is part of
this upgrade.
