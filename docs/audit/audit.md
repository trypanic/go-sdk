# go-sdk — Senior Go Engineering Audit & Remediation

Self-contained, resumable work order for any coding agent (Claude Code, Codex,
Cursor, …). It replaces a generic audit brief with one fitted to this
repository, split into tasks that can run in parallel, with a ledger (§9) that
records what is done so a new session — even with a different agent — can
pick up where the last one stopped.

> **Start here every session:** read §0, then §9 (ledger), then continue with
> the first task whose dependencies are `DONE` and whose status is `TODO`
> (or a stale `IN_PROGRESS`, see §0.3).

---

## 0. Resume protocol

### 0.1 Role

Act as a Staff Go engineer, software architect and security engineer who
will own this SDK in production. Understand first, then audit, then
prioritize, then fix. Do not chase theoretical perfection or apply patterns
for fashion; every change must solve a demonstrated problem.

### 0.2 On session start

1. `git fetch --all` (read-only) and check out the audit branch
   `audit/2026-11` (create it in task `P0.1` if it does not exist).
2. Read §9. Tasks are `TODO`, `IN_PROGRESS`, `IN_REVIEW`, `CHANGES_REQUESTED`,
   `DONE`, `BLOCKED` or `WAITING_APPROVAL`. A `CHANGES_REQUESTED` task goes
   back to its owner (or to whoever reclaims it) with the review notes in
   `Notes`; it takes priority over new `TODO` tasks.
3. Read the outputs of every `DONE` task you depend on (paths in §9).
4. Pick a task: status `TODO` (or stale, see 0.3) and all `Depends on` tasks
   `DONE`. If several are eligible and you can run parallel workers, take
   them all (§8).
5. Claim it: set `IN_PROGRESS`, `Owner` = `<agent>/<session-id or date>`,
   `Updated` = ISO timestamp, then commit (§0.4) before doing the work.

### 0.3 Stale claims

An `IN_PROGRESS` row whose `Updated` is older than 12 hours and whose output
file is missing or incomplete may be reclaimed. Note the takeover in `Notes`.
If a partial output exists, continue it instead of starting over.

### 0.4 Ledger commits

- Only edit your own row(s). Re-read the file immediately before each edit.
- Commit ledger and audit-document changes as
  `chore(audit): <task-id> <status>` so they stay out of the release
  changelog (Conventional Commits are enforced by a `commit-msg` hook).
- Never push, tag, or create releases (see §7).

### 0.5 On task completion

Set `IN_REVIEW` with the output path and the commit SHA, or `BLOCKED` with
the reason. Only a reviewer sets `DONE` (§8.2), and the reviewer is never the
task's owner. P0.1, P0.2 and P5 need no review and go straight to `DONE`. A
task that needs a human decision is `WAITING_APPROVAL`; stop working on it
and say so in your final message.

---

## 1. Project facts (re-verified in P1 on 2026-11-04 — see `discovery.md` §9)

| Fact | Value |
| --- | --- |
| Module | `github.com/trypanic/go-sdk` — **single module**, one `go.mod`, one version for all packages |
| Released version | `v1.0.0` (semver v1: public API is a compatibility contract) |
| Go | `go 1.26.1` directive in `go.mod` (no `toolchain` line); `.prototools` pins `go = "1.27.3"`; Go on `PATH` here is 1.26.2; CI uses `go-version-file: go.mod` |
| Size | 22 packages (21 importable), 13,979 lines = 9,659 non-test + 4,320 test; per-package totals incl. tests: errorkit 2.3k, messaging 1.5k, logger 1.4k, httprequest 1.3k, grpc 1.3k, mongo 1k, postgres 0.9k, storage 0.9k, … |
| Package name ≠ directory | `postgres` → `package database`, `mongo` → `package mongodb` |
| Contracts | `AGENTS.md` §2 (errorkit errors, tracing constructor triplet, no silent env reads, body redaction, concurrency/lifecycle) and each package `README.md` |
| Task runner | Task (`Taskfile.yml`); tools pinned via proto (`.prototools`) |
| Quality gates | `task test`, `task staticanalysis` (vet + staticcheck + semgrep), `task precommit:run`, `task release:check` (gorelease) |
| CI | Only `.github/workflows/release.yml` (release-please + SBOM). `SECURITY.md` describes semgrep CI jobs that are **not present** — verify and report |
| Tests | No `//go:build integration` tags and no `testing.Short()` gates; connection paths of postgres/mongo/messaging are untested (fakes only for postgres `Querier`, mongo `ClientPort`/`Collection`, llmclient requester, gRPC bufconn, `httptest`) |
| Project skills | `.skillshare/skills/{go-code-quality-check,go-design-principles,go-testing-strategy}` (gitignored; install with `task skills:install`) |
| Known pre-existing | See `baseline.md`: staticcheck SA1019 ×4 in `logger`, stdlib vulns on go ≤ 1.26.2, 25 golangci-lint issues. (The semgrep `defer-rows-close` hits at `postgres/stored_procedure_test.go:47` appear only when test files are passed explicitly, as the staged pre-commit mode does; full mode skips `*_test.go` — see X2-006.) |
| Gotcha | `task release:check` refuses a dirty tree (Taskfile precondition), including the untracked `.moon/` leftover from `main`; run it from a clean worktree |

---

## 2. Evaluation sources

Read the real sources. Never claim to have applied a skill you could not
read; if a source is unreachable, mark the task `BLOCKED`.

### 2.1 Precedence (highest first)

1. **Project contracts**: `AGENTS.md`, package `README.md`s, exported API at
   `v1.0.0`. A finding that contradicts a contract is a *proposal* needing
   approval, not a defect.
2. **Project skills**: `go-code-quality-check`, `go-design-principles`,
   `go-testing-strategy`.
3. **Eduardo Spinelli — `eduardo-sl/go-agent-skills`**.
4. **JetBrains — `JetBrains/go-modern-guidelines`**, limited to features
   available in Go 1.26 (the `go` directive), never newer.

### 2.2 Fetch (task P0.1)

Clone outside the repository and do not copy, vendor or modify them:

```bash
SRC="${XDG_CACHE_HOME:-$HOME/.cache}/go-audit-sources"
mkdir -p "$SRC"
git clone https://github.com/eduardo-sl/go-agent-skills "$SRC/go-agent-skills"
git -C "$SRC/go-agent-skills" checkout e25b517bbd0c11db35f75a83a14f18848c1abdb0
git clone https://github.com/JetBrains/go-modern-guidelines "$SRC/go-modern-guidelines"
git -C "$SRC/go-modern-guidelines" checkout 155dc7ca10da5e1f6c841503086957b1b37f5815
```

The SHAs are the reviewed revisions (2026-11-03). If you deliberately use
newer ones, record them in the P0.1 `Notes`.

- Eduardo: start with `skills/(workflow)/go-skills-router/SKILL.md`, then
  read each mapped `SKILL.md` **and its `references/`** before auditing its
  area.
- JetBrains: follow `plugin/skills/use-modern-go/SKILL.md`. Run its CLI per
  file you audit or edit, and read the full, untruncated output:
  `sh "$SRC/go-modern-guidelines/plugin/skills/use-modern-go/scripts/run-tool.sh" list --file-path <file.go>`
  Use `explain <id>` before skipping a guideline that looks relevant.

### 2.3 Area → skills map

| Area | Eduardo skills | Other sources |
| --- | --- | --- |
| A. Architecture & design | go-architecture-review, go-project-layout, go-interface-design, go-dependency-injection, go-design-patterns | go-design-principles |
| B. Idiomatic & modern Go | go-coding-standards, go-naming, go-error-handling, go-context, go-data-structures, go-modernize | JetBrains use-modern-go |
| C. Concurrency | go-concurrency-review, go-context | — |
| D. Security | go-security-audit, go-defensive-coding, go-dependency-audit | govulncheck, semgrep (`.semgrep.yml`) |
| E. Testing | go-test-quality, go-test-table-driven | go-testing-strategy |
| F. Performance | go-performance-review, go-data-structures | benchmarks/pprof, only with evidence |
| G. Public API & DX | go-api-design, go-interface-design, go-documentation, go-observability, go-grpc (grpc), go-database (postgres/mongo/storage) | gorelease, `AGENTS.md` §2 |
| H. Maintainability & DevEx | go-ci, go-build-release, go-dependency-audit, go-refactoring, go-code-review, go-documentation, git-commit | go-code-quality-check |

---

## 3. Scope & rules

- Work on branch `audit/2026-11`, created from the branch that contains
  `Taskfile.yml` (`main` once `chore/migrate-moon-to-taskfile` is merged,
  that branch otherwise).
- **Never push, tag, release, or edit `.release-please-manifest.json` or
  `CHANGELOG.md`.**
- Do not break exported APIs. `task release:check` must not report
  incompatible changes unless the item is approved as breaking (it would
  force `v2`, a new module path).
- Do not remove features, weaken tests to hide defects, or add abstractions
  without a demonstrated need.
- Only the `R-DEPS` item may change `go.mod`/`go.sum` (all packages share
  them; parallel edits corrupt them). Evaluate every dependency change.
- Do not introduce new tooling configuration (e.g. `.golangci.yml`, new CI
  jobs) without approval; propose it in the plan instead.
- Never invent findings. Every finding needs evidence: file:line, a failing
  test, tool output, or a concrete execution path. Speculative issues go to
  "Open questions", not the findings list.
- No destructive tests and no traffic to external services.
- All artifacts are written in English. One logical change per commit,
  Conventional Commits (`fix(httprequest): …`, `test(logger): …`,
  `refactor(errorkit): …`), finding IDs in the commit body.

---

## 4. Outputs

All under `docs/audit/`:

| File | Produced by | Content |
| --- | --- | --- |
| `audit.md` | — | This work order and ledger |
| `baseline.md` | P0.2 | Results of every §6 check **before** any change (pre-existing issues) |
| `discovery.md` | P1 | Architecture, package roles, internal dependency graph, critical flows, exported API inventory, tests and tooling state |
| `findings/<task-id>.md` | P2.x | Raw findings of one audit task |
| `AUDIT_REPORT.md` | P2.Z | Consolidated report (§5.2) |
| `REMEDIATION_PLAN.md` | P3 | Prioritized items `R-NNN` with their own status table |
| `reviews/<task-id>.md` | reviewers | Review notes that do not fit in the ledger (§8.2) |
| `REMEDIATION_REPORT.md` | P5 | What changed, tests added, validation results, residual risk |

---

## 5. Phases and tasks

### P0 — Setup (sequential)

- **P0.1** Create `audit/2026-11`; fetch sources (§2.2); `task tools:install`
  and `task skills:install` if tools are missing. Record tool versions.
- **P0.2** Run every check in §6 on the untouched tree and write
  `baseline.md` (command, tool version, exit code, summary, notable output).
  Unavailable tools are recorded as such, never as passing.

### P1 — Discovery (sequential, read-only)

Write `discovery.md`: purpose; package roles; internal import graph
(`go list -f '{{.ImportPath}}: {{join .Imports " "}}' ./...`); exported API
inventory per package (`go doc -all`); constructors, lifecycle and
concurrency guarantees as documented vs. as implemented; critical flows
(HTTP out with retries, inbound HTTP/gRPC middleware, Postgres stored
procedures, RabbitMQ publish/consume with topology, OTLP logging, LLM calls);
test inventory per package; existing tooling and CI. Re-verify §1 and fix it
if a fact is wrong.

### P2 — Audit (parallel, read-only)

Code-level tasks cover areas **B, C, D, E, F and the package-level part of
G** for their packages. Cross-cutting tasks cover the rest.

| Task | Scope | Focus beyond the default areas |
| --- | --- | --- |
| P2.C1 | `errorkit`, `logger`, `telemetry` | Registry concurrency, error wrapping/`errors.Is/As`, stack capture cost, global logger mutation, OTLP setup/shutdown order, sensitive data in logs |
| P2.C2 | `httpclient`, `httprequest`, `httpserver`, `httpserver/hertz`, `llmclient` | Timeouts, retries vs. idempotency, body close/drain, connection reuse, redaction defaults, context propagation, credential handling (`LLM_API_KEY`), response size limits |
| P2.C3 | `grpc` (incl. `internal/testsvc`) | Interceptor order, panic recovery, deadlines, streaming lifecycle/leaks, TLS/insecure defaults, metadata leaks |
| P2.C4 | `postgres`, `mongo`, `storage` | Pool lifecycle, `rows.Close`/`rows.Err`, SQL/NoSQL injection in stored-procedure helpers, cancellation, file permissions and path handling in `storage` |
| P2.C5 | `messaging` | Channel/connection recovery, consumer goroutine lifecycle, ack/nack semantics, prefetch, publisher confirms, topology file handling, shutdown |
| P2.C6 | `envs`, `urlkit`, `validators`, `marshal`, `slices`, `stringutils`, `algorithms`, `ioutils` | Input validation edge cases, fuzz candidates, generics, `ioutils` must stay dev-only |
| P2.X1 | Whole repo: areas **A** and **G** | Dependency direction, coupling between packages, API consistency (naming, options, constructor triplet), GoDoc and examples, consumer testability; `gorelease` against `v1.0.0` |
| P2.X2 | Whole repo: area **H** | Taskfile, proto, pre-commit, release workflow, missing CI for tests/lint/vuln, docs accuracy (README/AGENTS/SECURITY vs. reality), reproducibility |
| P2.X3 | Whole repo: dependencies & vulnerabilities (area D, supply chain) | `govulncheck` (call-graph reachable vs. not), outdated/abandoned modules (`task deps:outdated`), dependency weight imposed on consumers by the single module |

Each `findings/<task-id>.md` lists findings with stable IDs
`<TASK>-<NNN>` (e.g. `C2-004`, `X1-002`) and these fields:

```markdown
### C2-004 — <title>
- Area: C (Concurrency) · Severity: HIGH · Effort: S/M/L
- Location: httprequest/client.go:120-148
- Problem: …
- Evidence: <code excerpt, tool output, or reproducing test>
- Impact: …
- Source: eduardo/go-concurrency-review §… · JetBrains <guideline-id> · AGENTS.md §2.6
- Recommendation: …
- Breaking: no | yes (<what breaks>) · Needs approval: no | yes (<why>)
```

Severity: **CRITICAL** — exploitable vulnerability or defect that breaks core
behavior; **HIGH** — demonstrable security, correctness, concurrency or
architecture problem; **MEDIUM** — maintainability, design, testing or
performance; **LOW** — style, readability, modernization.

End each findings file with *Strengths* (what to keep) and *Open questions*
(suspicions without proof).

- **P2.Z** Consolidate (sequential, after all P2 tasks): dedupe across files
  (keep the first ID, list duplicates), verify every CRITICAL/HIGH finding
  yourself, and write `AUDIT_REPORT.md`: executive summary, overall state,
  architecture, strengths, findings by area and severity with evidence,
  sources used (with SHAs), residual risks.

### P3 — Remediation plan (sequential) → **human gate**

Write `REMEDIATION_PLAN.md`, grouped in this order: 1) security and
correctness, 2) concurrency and stability, 3) architecture and design,
4) idiomatic Go and modernization, 5) testing, 6) performance,
7) documentation and maintainability.

Each item `R-NNN`: findings resolved, solution, files, regression risk,
compatibility impact, tests required, effort, dependencies on other items,
**cluster** (C1…C6, X), and **class**:

- `auto` — non-breaking, no architectural decision: may be implemented once
  the gate is approved.
- `approval` — breaking change, new dependency or tooling, architecture
  change (e.g. splitting into multiple modules), behavior change visible to
  consumers. Needs an explicit per-item decision.

Include a status table (`R-NNN | class | cluster | status | commit`).
Once P3 passes review (§8.2) and is `DONE`, set **G1** to
`WAITING_APPROVAL` and stop. The human
approves by setting G1 to `DONE` and marking each `approval` item
`APPROVED` or `REJECTED` in the plan.

### P4 — Remediation (parallel per cluster, after G1)

- One task per cluster with eligible items (`auto`, or `approval` marked
  `APPROVED`). Each worker uses its own worktree outside the repo:
  `git worktree add ../go-sdk-wt/<cluster> -b audit/2026-11-<cluster> audit/2026-11`.
- Order inside a worker: write a failing regression test → fix → run the
  package checks (§6, scoped with `task test -- <pkg>` and
  `go test -race -short ./<pkg>/...`) → commit. Update the item in the plan's
  status table.
- `R-DEPS` (any `go.mod`/`go.sum` change) runs alone, after the clusters are
  merged.
- **P4.M** Merge (sequential): merge each cluster branch whose task is
  `DONE` (i.e. approved by the reviewer) into
  `audit/2026-11` one at a time, run §6 after each merge, remove the
  worktrees.

### P5 — Validation & report (sequential)

Run every check in §6 on the final tree, compare with `baseline.md`, and
write `REMEDIATION_REPORT.md`: per item the original problem, the solution,
files, tests added, checks run and their results; plus limitations, items
deferred or rejected, and residual risks. Distinguish pre-existing failures
from ones introduced by the remediation.

---

## 6. Validation checks

Run from the repository root (`gorelease` from a clean worktree). Report
each command's real exit code; never mark a check as passing if it did not
run.

| # | Check | Command |
| --- | --- | --- |
| V1 | Format | `gofmt -l .` (must print nothing) |
| V2 | Build | `go build ./...` |
| V3 | Vet | `go vet ./...` |
| V4 | Unit tests | `task test` |
| V5 | Race | `go test -race -short ./...` (needs cgo + gcc) |
| V6 | Coverage | `go test -short -coverprofile=docs/audit/coverage.out ./... && go tool cover -func=docs/audit/coverage.out \| tail -1` (do not commit `coverage.out`) |
| V7 | Static analysis | `task staticanalysis` (needs `task skills:install`) |
| V8 | Vulnerabilities | `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` |
| V9 | Lint (informational) | `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run ./...` — default linters, no config committed without approval |
| V10 | API compatibility | `task release:check` |
| V11 | Hooks | `task precommit:run` |

---

## 7. Hard limits

- No `git push`, tags, GitHub releases or edits to release files.
- No breaking API change, module split, new dependency or new tool config
  unless approved in `REMEDIATION_PLAN.md`.
- No changes to the external skill repositories and no copying them into
  this repository.
- Stop at gate G1. Do not start P4 until G1 is `DONE`.

---

## 8. Parallel execution, review and models

### 8.1 Execution graph

Tasks in the same braces can run at the same time:

```
P0.1 → P0.2 → P1 ⟲ → { P2.C1 … P2.C6, P2.X1, P2.X2, P2.X3 } ⟲ → P2.Z ⟲ → P3 ⟲ → G1
     → { P4.C1 … P4.C6, P4.X } ⟲ → P4.M → R-DEPS ⟲ → P5
```

`⟲` = review loop (§8.2): each task must pass review before its dependents
start.

- **Coordinator mode (preferred).** One session (the orchestrator) owns this
  file and the ledger, dispatches each parallel task to a worker subagent,
  reviews or dispatches the review of every result (§8.2), and updates the
  ledger. Workers never edit `audit.md`; they write only their output file
  (P2) or their worktree (P4) and report back.
- **Independent sessions mode.** Several sessions each claim rows per §0.2
  and follow §0.4. A session may review any `IN_REVIEW` task it did not
  own. P2 tasks only write their own findings file, so they never conflict.
  P4 workers only touch their worktree branch.

### 8.2 Review loop

Nothing reaches `DONE` on its author's word. The reviewer runs with fresh
context (a different subagent or session than the author), records its name
in `Reviewed by`, and sets `DONE` or `CHANGES_REQUESTED` with concrete
reasons in `Notes` (or in a `docs/audit/reviews/<task-id>.md` file when they
do not fit). A task may loop at most 3 times; after that it goes
`BLOCKED` for a human.

| Task | Reviewer | What the review checks |
| --- | --- | --- |
| P1 | orchestrator | Facts in §1 re-verified; API inventory complete (`go doc` per package); graph matches `go list` |
| P2.* | orchestrator | Format of §5 P2; every finding has evidence and a source; re-verify every CRITICAL/HIGH and at least 3 random MEDIUM/LOW findings against the code; no finding contradicts a project contract without being marked `Needs approval` |
| P2.Z | `go-reviewer` agent | No finding lost or merged wrongly; severities consistent; duplicates listed |
| P3 | `go-reviewer` agent | Every CRITICAL/HIGH has an item or an explicit deferral; `auto`/`approval` classes correct (anything breaking, new dependency or new tooling must be `approval`) |
| P4.* | `go-reviewer` agent | Review of `git diff audit/2026-11...audit/2026-11-<cluster>` (Claude Code: `/code-review high` on that range): correctness, regression test fails without the fix, no weakened tests, no API break (`task release:check` from a clean worktree), scoped §6 checks green |
| R-DEPS | `go-reviewer` agent | Each version bump justified; changelogs of major/minor bumps read; §6 V2–V8 green |

### 8.3 Models and effort

Configure workers explicitly; in Claude Code subagents inherit the
orchestrator's model unless told otherwise. The agent definitions in
`.claude/agents/` carry these defaults, and the orchestrator passes `model`
and `effort` overrides where this table differs from them.

| Role / task | Claude Code agent | Model | Effort |
| --- | --- | --- | --- |
| Orchestrator (this file, ledger, P2 reviews) | main session | Opus 5.5 | `high` |
| P1 discovery | `go-auditor` | Opus 5.5 | `high` |
| P2.C1–C5, P2.X1, P2.X3 | `go-auditor` | Opus 5.5 | `high` |
| P2.C6, P2.X2 | `go-auditor` | Sonnet 5.5 | `high` |
| P2.Z consolidation, P3 plan | main session | Opus 5.5 | `xhigh` |
| Reviews of P2.Z, P3, P4, R-DEPS | `go-reviewer` | Opus 5.5 | `high` |
| P4 remediation (concurrency/security items) | `go-remediator` | Opus 5.5 | `high` |
| P4 remediation (other items) | `go-remediator` | Opus 5.5 | `medium` |
| P5 validation and report | `go-remediator` | Sonnet 5.5 | `medium` |

Other agents (Codex, Cursor, …): use your most capable reasoning model at
high effort for discovery, audit, consolidation, planning and review, and a
faster model for P2.C6, P2.X2 and P5. Never use a small/fast tier model for
audit or review tasks.

### 8.4 Worker brief templates

Auditor (P1, P2.*):

```text
You are auditing github.com/trypanic/go-sdk, task <TASK-ID>: <scope from §5>.
Read docs/audit/audit.md §1–§3 and §5 (your task), docs/audit/discovery.md,
docs/audit/baseline.md, AGENTS.md and the README.md of each package in scope.
Sources are cloned at <SRC>; read the skills mapped to your areas in §2.3
(SKILL.md and references/) before auditing. Read-only: write only
docs/audit/findings/<TASK-ID>.md in the format of §5 P2. Do not edit
audit.md, do not commit, do not push. Report: number of findings by
severity, the file path, and anything you could not verify.
<If CHANGES_REQUESTED: the review notes to address.>
```

Remediator (P4.*, R-DEPS):

```text
You are fixing github.com/trypanic/go-sdk, task <TASK-ID>, in the worktree
<path> on branch <branch>. Implement only these items from
docs/audit/REMEDIATION_PLAN.md: <R-NNN list>. For each: failing regression
test first, then the fix, then `task test -- <pkg>` and
`go test -race -short ./<pkg>/...`, then one Conventional Commit with the
item and finding IDs in the body. Follow §3 and §7 of docs/audit/audit.md.
Do not edit audit.md, do not push. Report per item: commit SHA, tests
added, checks run with exit codes, anything left undone and why.
<If CHANGES_REQUESTED: the review notes to address.>
```

Reviewer:

```text
You are reviewing task <TASK-ID> of docs/audit/audit.md; you did not author
it. Apply the checks for this task in §8.2 to <output file or diff range>.
Re-verify claims against the code yourself; do not trust the author's
report. Read-only: do not edit files or commit. Return a verdict
APPROVE or CHANGES_REQUESTED with a numbered list of concrete, verifiable
reasons (file:line, command and output).
```

---

## 9. Progress ledger

Status: `TODO` · `IN_PROGRESS` · `IN_REVIEW` · `CHANGES_REQUESTED` · `DONE` ·
`BLOCKED` · `WAITING_APPROVAL`. `Reviewer` is who must review (§8.2);
`Reviewed by` is filled in by that reviewer.

| Task | Phase | Depends on | Status | Owner | Updated | Output | Commit | Reviewer | Reviewed by | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| P0.1 | Setup | — | DONE | claude-opus-5-5/2026-11-03 | 2026-11-04T00:15Z | sources + branch | | — | — | Branch from `chore/migrate-moon-to-taskfile` (d966f55; contains main). Sources at pinned SHAs in `~/.cache/go-audit-sources`. Tools: go on PATH 1.26.2 (proto resolves 1.27.3 from `.prototools`), proto 0.63.2, task 3.54.0, staticcheck on PATH 2026.1 (proto pins 2026.2.2), semgrep 1.95.0, pre-commit 4.7.0, gcc 15.3.0, skillshare 0.28.2 (pin 0.25.5); gorelease/govulncheck/golangci-lint via `go run`. Skills already installed in `.skillshare/skills`. |
| P0.2 | Setup | P0.1 | DONE | claude-opus-5-5/2026-11-03 | 2026-11-04T00:25Z | `baseline.md` | | — | — | V7 FAIL (4× SA1019 logger), V8 FAIL on go1.26.2 (19 stdlib vulns; 0 on go1.27.3), V9 25 issues; rest PASS. Coverage 47.7%. |
| P1 | Discovery | P0.2 | DONE | go-auditor via orchestrator claude-opus-5-5/2026-11-03 | 2026-11-04T00:45Z | `discovery.md` | | orchestrator | orchestrator claude-opus-5-5 | Reviewed: 22 packages in API inventory, 30 intra-module edges match `go list`, §1 corrected per discovery §9; DN-01 and DN-15 spot-checked against code. |
| P2.C1 | Audit | P1 | DONE | go-auditor via orchestrator claude-opus-5-5/2026-11-03 | 2026-11-04T04:20Z | `findings/P2.C1.md` | | orchestrator | orchestrator claude-opus-5-5 | 25 findings (H2/M12/L11). Re-verified C1-001 (=DN-01, config.go:303-310 global level/time format), C1-002 (PrettyJSON serializes stack_trace, error.go:38,192), C1-008 (direct type assertion instance.go:82, wrapper.go:33), C1-004 (redaction only password variants, config.go:264), C1-009 (no nil check in WithErrorKit); temp tests removed; format OK. |
| P2.C2 | Audit | P1 | DONE | go-auditor via orchestrator claude-opus-5-5/2026-11-03 | 2026-11-04T04:00Z | `findings/P2.C2.md` | | orchestrator | orchestrator claude-opus-5-5 | 27 findings (H5/M15/L7). Re-verified C2-001 (Run calls Spin and returns nil, wrapper.go), C2-002 (retry loop ignores method), C2-003 (MaxRetries<0 skips loop, returns nil lastErr, httprequest.go:187), C2-004 (out.(*string) assertion panics on named types, decode.go), C2-005 (=DN-15); format OK. |
| P2.C3 | Audit | P1 | DONE | go-auditor via orchestrator claude-opus-5-5/2026-11-03 | 2026-11-04T03:30Z | `findings/P2.C3.md` | | orchestrator | orchestrator claude-opus-5-5 | 16 findings (H4/M8/L4). Re-verified C3-001/002 (panic value and Reason sent to client, interceptors.go:45, status.go ToStatus), C3-003 (codeFor default Internal), C3-005 (insecure default, client.go:33), C3-009; format OK. |
| P2.C4 | Audit | P1 | DONE | go-auditor via orchestrator claude-opus-5-5/2026-11-03 | 2026-11-04T03:50Z | `findings/P2.C4.md` | | orchestrator | orchestrator claude-opus-5-5 | 24 findings (H3/M13/L8). Re-verified C4-001 (pgx error with ConnString stored via WithPayload, postgres.go:41), C4-002 (only ERR_DB_POSTGRES_ERROR is ever constructed), C4-012 (fixed .tmp name, in-process mutex only), C4-005, C4-015; format OK. Author notes local semgrep CLI crashes on Python 3.15 (protobuf TypeError). |
| P2.C5 | Audit | P1 | DONE | go-auditor via orchestrator claude-opus-5-5/2026-11-03 | 2026-11-04T03:30Z | `findings/P2.C5.md` | | orchestrator | orchestrator claude-opus-5-5 | 18 findings (H2/M10/L6). Re-verified C5-001 (retry ignores ctx then Nack requeue=false, messaging.go:331,381), C5-002 (mandatory=false, no confirms), C5-003, C5-015; format OK. |
| P2.C6 | Audit | P1 | DONE | go-auditor via orchestrator claude-opus-5-5/2026-11-03 | 2026-11-04T03:40Z | `findings/P2.C6.md` | | orchestrator | orchestrator claude-opus-5-5 | 23 findings (H1/M9/L13). Re-verified C6-001 (BuildURL parses base+path), C6-006 (Chunk without capacity clip), C6-004 (no Reset after setting InitialInterval); ioutils not imported by production code; format OK. |
| P2.X1 | Audit | P1 | DONE | go-auditor via orchestrator claude-opus-5-5/2026-11-03 | 2026-11-04T04:10Z | `findings/P2.X1.md` | | orchestrator | orchestrator claude-opus-5-5 | 18 findings (H1/M15/L2). Re-verified X1-001 (httprequest imports logger, logger imports otlploggrpc, httprequest links grpc), route interface only GET/POST/PUT/DELETE (contracts.go:122-159), ServerOptions bool zero values (options.go:45-50), 15 packages without package doc; gorelease rerun = v1.1.0 compatible; format OK. |
| P2.X2 | Audit | P1 | DONE | go-auditor via orchestrator claude-opus-5-5/2026-11-03 | 2026-11-04T03:10Z | `findings/P2.X2.md` | | orchestrator | orchestrator claude-opus-5-5 | 19 findings (H3/M8/L8). Re-verified X2-001/002/003 (HIGH) and X2-007/009/011; format OK. Side effect noted by author: pre-commit fetched remote hook repos in a scratch clone. |
| P2.X3 | Audit | P1 | DONE | go-auditor via orchestrator claude-opus-5-5/2026-11-03 | 2026-11-04T03:30Z | `findings/P2.X3.md` | | orchestrator | orchestrator claude-opus-5-5 | 11 findings (H1/M5/L5). Re-verified X3-001 (baseline V8), X3-006 (logger links x/net/trace), X3-008 (backoff v4+v5, v4 in algorithms API); go.mod untouched; format OK. |
| P2.Z | Audit | all P2.* | DONE | orchestrator claude-opus-5-5/2026-11-03 | 2026-11-04T05:40Z | `AUDIT_REPORT.md` | | go-reviewer | go-reviewer (loop 2 APPROVE) | Loop 2 after CHANGES_REQUESTED (6 blocking + 6 notes, all addressed): 170 unique (11 merged): H21/M87/L62; C6-009 and C2-017 un-merged; C2-006, C2-017 → HIGH; Needs approval parsed from its own line and unioned on merges; severity criteria in §7.2. Loop-2 non-blocking notes applied: multi-line locations, C4-009/C5-012 partial, C2-017 Hertz wording, disclosure criterion. |
| P3 | Plan | P2.Z | DONE | orchestrator claude-opus-5-5/2026-11-03 | 2026-11-04T08:00Z | `REMEDIATION_PLAN.md` | | go-reviewer | go-reviewer (loop 2 APPROVE) | Loop 2 after CHANGES_REQUESTED (5 blocking + notes addressed): 139 items (63 auto / 76 approval); 8 auto items reclassified or split; AGENTS.md edits centralized in R-153; rejected-dependency fallbacks; P4.XP step for X post-merge items; all fields present. Loop-2 notes applied (R-159 fields, after P4.XP, R-153 fallback, C1-018 deferral). |
| G1 | Gate | P3 | DONE | human (owner, via session chat) | 2026-11-04T08:30Z | approvals in plan | 39af5b6 | — | — | Owner approved all approval items incl. breaking changes; release will be 2.0.0. R-152 = MIT licence (owner's answer), R-159 DEFERRED. See REMEDIATION_PLAN.md §12. |
| P4.C1 | Remediation | G1 | IN_REVIEW | go-remediator via orchestrator claude-opus-5-5/2026-11-04 | 2026-11-04T15:30Z | branch `audit/2026-11-C1` | f901d61 | go-reviewer | | |
| P4.C2 | Remediation | G1 | IN_PROGRESS | go-remediator via orchestrator claude-opus-5-5/2026-11-04 | 2026-11-04T08:30Z | branch `audit/2026-11-C2` | | go-reviewer | | |
| P4.C3 | Remediation | G1 | DONE | go-remediator via orchestrator claude-opus-5-5/2026-11-04 | 2026-11-04T14:15Z | branch `audit/2026-11-C3` | f58b77b | go-reviewer | go-reviewer (APPROVE) | Follow-ups: N2 FromContextError sends wrapped ctx error text (consider in R-020); N4 grpc registry-walk tests import C2/C4/C5 packages — re-run after P4.M; N1 keep BREAKING CHANGE adjacent to trailers in future commits. |
| P4.C4 | Remediation | G1 | IN_REVIEW | go-remediator via orchestrator claude-opus-5-5/2026-11-04 | 2026-11-04T16:00Z | branch `audit/2026-11-C4` | e7465c1 | go-reviewer | | |
| P4.C5 | Remediation | G1 | IN_REVIEW | go-remediator via orchestrator claude-opus-5-5/2026-11-04 | 2026-11-04T16:30Z | branch `audit/2026-11-C5` | e366890 | go-reviewer | | |
| P4.C6 | Remediation | G1 | DONE | go-remediator via orchestrator claude-opus-5-5/2026-11-04 | 2026-11-04T13:30Z | branch `audit/2026-11-C6` | 5d5c63d | go-reviewer | go-reviewer (APPROVE) | Follow-ups: N1 messaging executeWithRetry untested and lost 15-min cap → covered by R-026 (C5) + add MaxElapsedTime in P4.XP; N2 urlkit %2F/opaque-base edge cases; N5 4 lint issues in new tests; N6 R-160 must rewrite string constant in ioutils/imports_test.go. |
| P4.X | Remediation | G1 | IN_REVIEW | go-remediator via orchestrator claude-opus-5-5/2026-11-04 | 2026-11-04T16:15Z | branch `audit/2026-11-X` | 1aa63f3 | go-reviewer | go-reviewer (loop 2 CHANGES_REQUESTED) | docs, tooling, cross-cutting ; R-148: sentrux removed (binary needs OpenSSL 3). Owner action: enable GitHub private vulnerability reporting. ; loop 1: CI never triggers on release-please PR (GITHUB_TOKEN) → workflow_dispatch + accurate docs; SECURITY.md fixed version; runner awk exit code; release.yml token scoping. Loop 2: restore required-checks recommendation (dispatch satisfies them); vet compile errors as findings; reword 1ffb311 to build(scripts). |
| P4.M | Remediation | all P4.C*, P4.X | TODO | | | merged `audit/2026-11` | | — | | |
| P4.XP | Remediation | P4.M | TODO | | | X post-merge items on `audit/2026-11` | | go-reviewer | | sequential, after P4.M (see REMEDIATION_PLAN.md cross-cluster rule) |
| R-DEPS | Remediation | P4.XP | TODO | | | `go.mod`/`go.sum` | | go-reviewer | | skip if no approved dep change |
| P5 | Validation | R-DEPS | TODO | | | `REMEDIATION_REPORT.md` | | — | | |
