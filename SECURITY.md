# Security & static analysis

This SDK runs three Go static-analysis tools — `go vet`, `staticcheck`, and
`semgrep` — through a single shell script committed in this repository,
[`scripts/staticanalysis.sh`](./scripts/staticanalysis.sh). It is wired into both
`pre-commit` (staged-files mode) and `task` (full-repo mode), and the same
tools run again in CI.

## Canonical entry point

```bash
task staticanalysis
```

This invokes [`scripts/staticanalysis.sh`](./scripts/staticanalysis.sh) with `QG_FULL=1`
(scans `./...`). It writes per-tool reports and grouped fix-plans under
`.staticanalysis/`. Exit codes:

| Code | Meaning                                                                 |
|------|-------------------------------------------------------------------------|
| `0`  | Every enabled tool ran and none produced an `ERROR`-severity finding.  |
| `1`  | At least one `ERROR`-severity finding.                                  |
| `2`  | A tool failed: it crashed, exited with an unexpected code, produced no parsable output, or is not installed. A missing tool counts as a failure unless its `QG_SKIP_*` knob is set. `jq` is always required. |

The script never reports `OK` for a tool that did not run: a skipped tool is
printed as `skipped`, a failed one as `TOOL FAILURE` with the tail of its
stderr log.

The pre-commit hook calls the same script, but in **staged** mode — only the
`.go` files that `git diff --cached --diff-filter=ACMR` returns (added, copied,
modified or renamed) are scanned, and only their packages are vetted. With
nothing staged it skips, so `task precommit:run` also runs the manual-stage
hook `staticanalysis-full` (`QG_FULL=1`) to scan the whole repository:

```bash
pre-commit run --all-files --hook-stage manual staticanalysis-full
```

### Scan scope: tests included

Test files are in scope in both modes. [`.semgrepignore`](./.semgrepignore)
replaces semgrep's built-in ignore list, which skips `*_test.go`: without it
a full scan skipped every test while a staged scan of the same file reported
findings. Rules that must not apply to tests exclude them in `.semgrep.yml`
with `paths.exclude` (for example `defer-rows-close`, which matches test
fakes syntactically).

### What the script runs

| Step | Tool          | What it catches                                                 |
|------|---------------|-----------------------------------------------------------------|
| 1    | `go vet`      | Go-toolchain diagnostics (printf, lock copies, struct tags).    |
| 2    | `staticcheck` | Type-aware Go bugs (`SA*`), style (`ST*`), dead code (`U1000`). |
| 3    | `semgrep`     | Org-policy + community Go/security/secrets rules.               |

Semgrep is invoked via the `semgrep/semgrep:1.95.0` Docker image, pinned by
digest in the script, when Docker is available; otherwise it falls back to a
local `semgrep` CLI (`task tools:install` installs the same 1.95.0; the
runner warns when the CLI version differs). **No authentication is used** —
`SEMGREP_APP_TOKEN` is not set, no Semgrep Platform features are required,
and **there is no Semgrep MCP server configured**. Both paths use the same
rules: `.semgrep.yml` plus the community packs `p/golang`, `p/gosec` and
`p/secrets`. The packs are downloaded from the Semgrep registry on every run
and cannot be pinned, so they are the one input that can change without a
commit here; `QG_SEMGREP_PACKS=0` scans with `.semgrep.yml` only (offline,
reproducible). The fallback runs when Docker is unavailable or its run fails. The path that
produced the result is printed on the semgrep line and recorded as `runner`
in `report.semgrep.json` and in `plan.semgrep.md`.

### Outputs (`.staticanalysis/`)

| File                    | Purpose                                                          |
|-------------------------|------------------------------------------------------------------|
| `report.<tool>.json`    | Normalized findings (severity / path / line / check\_id / msg). |
| `plan.<tool>.md`        | Findings grouped by file as a fix-list.                          |
| `<tool>.stderr.log`     | Captured tool stderr (debugging); always written.                |

### Environment knobs

| Variable             | Effect                                                  |
|----------------------|---------------------------------------------------------|
| `QG_FULL=1`          | Scan whole repo (set by the `task` runner).               |
| `QG_SKIP_VET=1`      | Skip `go vet`.                                          |
| `QG_SKIP_SC=1`       | Skip `staticcheck`.                                     |
| `QG_SKIP_SEMGREP=1`  | Skip `semgrep`.                                         |
| `QG_SEMGREP_PACKS=0` | Semgrep with `.semgrep.yml` only (no registry packs).   |
| `QG_SEMGREP_IMAGE=…` | Override the pinned semgrep Docker image.               |
| `QG_VERBOSE=1`       | Also print tool stderr to the terminal after each tool. |
| `QG_OUT_DIR=path`    | Override output directory (default `.staticanalysis`).  |

## Why three tools

`go vet` ships with the toolchain — no excuse to skip it; catches the
obvious-but-easy-to-miss stuff (printf format mismatches, lock copies,
unreachable code, struct-tag typos).

**Semgrep** is **syntax-pattern** based — fast, language-agnostic, easy to
author custom org-specific rules (banning `fmt.Println` in lib code, requiring
`%w` wrapping, etc.). It cannot resolve Go types.

**Staticcheck** is **type-aware** — built on `go/types`. Catches things Semgrep
can't reliably detect:

- `SA1012` — passing `nil` as `context.Context` (we previously had a buggy
  Semgrep rule for this; it false-positived on `(*T)(nil)` type conversions
  and `var _ Iface = (*Impl)(nil)` interface checks. Removed.)
- `SA4006` — unused assignments
- `SA1029` — illegal context key types
- `ST1003` — Go style violations
- `U1000` — dead code

Use **semgrep** for org policy. Use **staticcheck** for type-correct Go bugs.
`go vet` is the cheap baseline.

## One-time setup (per developer)

```bash
# 1. Install tools (versions pinned in .prototools)
proto trust .prototools
proto install                    # go, python, task, staticcheck, skillshare, sentrux
task tools:install               # pinned goimports, pre-commit, semgrep, git-cliff

# 2. Install git hooks
task precommit:install
# (equivalent to: pre-commit install --install-hooks)

# 3. Sanity check on the whole tree
task staticanalysis
task precommit:run
```

Docker is optional but recommended — without it the script falls back to the
host `semgrep` binary with the project rule pack only.

## What runs on every `git commit`

Order matters — earlier failures abort the commit (see `.pre-commit-config.yaml`):

1. `trailing-whitespace`, `end-of-file-fixer`, `check-yaml`,
   `check-merge-conflict`, `check-added-large-files` (max 512 KB),
   `detect-private-key`.
2. `gofmt -w` and `goimports -w` (auto-fix; re-stage if the hook rewrites a
   file).
3. `go test -short ./...`.
4. `bash scripts/staticanalysis.sh` — `go vet` + `staticcheck` + `semgrep` against
   staged `.go` files.

If any step exits non-zero (including an `ERROR`-severity finding from any
tool), the commit is blocked. Warnings are reported but do not block — fix
them in the same PR if related.

## Rule severities (project rules in `.semgrep.yml`)

| Severity  | Behavior                          | Example rule                  |
|-----------|-----------------------------------|-------------------------------|
| `ERROR`   | Blocks commit + blocks CI         | `no-hardcoded-credentials`    |
| `WARNING` | Reported, does not block          | `error-wrapping-required`     |
| `INFO`    | Reported, does not block          | (none currently)              |

Promote a rule from `WARNING` → `ERROR` only when the codebase is clean of it.

## CI gate (`.github/workflows/semgrep.yml`)

The `code-quality` workflow runs two jobs:

- **`staticcheck`** — installs `staticcheck` and runs `staticcheck ./...` on
  every push/PR to `main` / `develop`.
- **`semgrep`** —
  - **Pull requests**: `semgrep ci` runs in baseline-diff mode → only new
    findings on changed lines block the PR.
  - **Push to `main` / `develop`**: full-repo scan with `--error`.
  - **Weekly cron** (Mon 06:00 UTC): full sweep; results uploaded as SARIF to
    the GitHub Security tab and as a JSON artifact (30-day retention).

`go vet` is not a separate CI job — it runs as part of the staged pre-commit
hook and as step 1 of the local `task staticanalysis` task.

## "Plan before commit" workflow

```
edit code
   │
git add .
   │
pre-commit fires
   │
   ├── auto-fixers (gofmt, goimports)
   ├── go test -short
   └── scripts/staticanalysis.sh
        ├── go vet           → .staticanalysis/plan.go-vet.md
        ├── staticcheck      → .staticanalysis/plan.staticcheck.md
        └── semgrep          → .staticanalysis/plan.semgrep.md
                │
        ┌───────┴───────┐
   no errors          errors
        │                │
   commit lands     review the plan files
                    fix → re-stage → re-run
```

Re-generate the plan files manually at any time:

```bash
task staticanalysis            # whole repo
bash scripts/staticanalysis.sh               # staged files only
QG_VERBOSE=1 bash scripts/staticanalysis.sh  # also print tool stderr
```

## Bypassing hooks (don't)

`git commit --no-verify` is never the answer. If a rule is wrong, fix the rule
in `.semgrep.yml` in the same PR and justify it in the commit body.

## Adding a new rule

1. Add to `.semgrep.yml` with `severity: WARNING` first.
2. Run `task staticanalysis` and review
   `.staticanalysis/plan.semgrep.md`.
3. Fix all existing offenders.
4. Promote to `severity: ERROR` in a follow-up commit.
