# Security & static analysis

This SDK runs three Go static-analysis tools — `go vet`, `staticcheck`, and
`semgrep` — through a single shell script. The script is wired into both
`pre-commit` (staged-files mode) and `moon` (full-repo mode), and the same
tools run again in CI.

## Canonical entry point

```bash
moon run repo:staticanalysis
```

This invokes [`staticanalysis.sh`](./staticanalysis.sh) with `QG_FULL=1`
(scans `./...`). It writes per-tool reports and grouped fix-plans under
`.staticanalysis/`. Exit code is `1` if any tool produces an `ERROR`-severity
finding, `0` otherwise.

The pre-commit hook calls the same script, but in **staged** mode — only files
returned by `git diff --cached` are scanned, and only their packages are vetted.

### What the script runs

| Step | Tool          | What it catches                                                 |
|------|---------------|-----------------------------------------------------------------|
| 1    | `go vet`      | Go-toolchain diagnostics (printf, lock copies, struct tags).    |
| 2    | `staticcheck` | Type-aware Go bugs (`SA*`), style (`ST*`), dead code (`U1000`). |
| 3    | `semgrep`     | Org-policy + community Go/security/secrets rules.               |

Semgrep is invoked via the `semgrep/semgrep` Docker image when Docker is
available; otherwise it falls back to a local `semgrep` CLI binary. **No
authentication is used** — `SEMGREP_APP_TOKEN` is not set, no Semgrep Platform
features are required, and **there is no Semgrep MCP server configured**. The
Docker variant uses the configured rule packs (`.semgrep.yml`, `p/golang`,
`p/gosec`, `p/secrets`); the local CLI fallback uses `.semgrep.yml` only.

### Outputs (`.staticanalysis/`)

| File                    | Purpose                                                          |
|-------------------------|------------------------------------------------------------------|
| `report.<tool>.json`    | Normalized findings (severity / path / line / check\_id / msg). |
| `plan.<tool>.md`        | Findings grouped by file as a fix-list.                          |
| `<tool>.stderr.log`     | Captured tool stderr (debugging).                                |

### Environment knobs

| Variable             | Effect                                                  |
|----------------------|---------------------------------------------------------|
| `QG_FULL=1`          | Scan whole repo (set by the `moon` task).               |
| `QG_SKIP_VET=1`      | Skip `go vet`.                                          |
| `QG_SKIP_SC=1`       | Skip `staticcheck`.                                     |
| `QG_SKIP_SEMGREP=1`  | Skip `semgrep`.                                         |
| `QG_VERBOSE=1`       | Stream tool stderr to terminal instead of log files.    |
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
# 1. Install tools
pipx install semgrep==1.95.0     # or: brew install semgrep
pipx install pre-commit          # or: brew install pre-commit
go install golang.org/x/tools/cmd/goimports@latest
go install honnef.co/go/tools/cmd/staticcheck@latest

# 2. Install git hooks
moon run repo:precommit-install
# (equivalent to: pre-commit install --install-hooks)

# 3. Sanity check on the whole tree
moon run repo:staticanalysis
moon run repo:precommit-run
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
4. `bash staticanalysis.sh` — `go vet` + `staticcheck` + `semgrep` against
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
hook and as step 1 of the local `moon run repo:staticanalysis` task.

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
   └── staticanalysis.sh
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
moon run repo:staticanalysis            # whole repo
bash staticanalysis.sh                  # staged files only
QG_VERBOSE=1 bash staticanalysis.sh     # stream tool stderr live
```

## Bypassing hooks (don't)

`git commit --no-verify` is never the answer. If a rule is wrong, fix the rule
in `.semgrep.yml` in the same PR and justify it in the commit body.

## Adding a new rule

1. Add to `.semgrep.yml` with `severity: WARNING` first.
2. Run `moon run repo:staticanalysis` and review
   `.staticanalysis/plan.semgrep.md`.
3. Fix all existing offenders.
4. Promote to `severity: ERROR` in a follow-up commit.
