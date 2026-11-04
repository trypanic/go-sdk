# Task: continue the go-sdk audit remediation (P4 → P5)

Self-contained instructions for an orchestrating agent **with subagents**
(GPT-6.1 Astra, Kimi K3.1 Pro, or any other). Session 1 (Claude Code)
completed P0.1–P3, gate G1 and most of P4; this file tells you exactly what
remains and how to dispatch it.

## 0. Before anything

```bash
cd /home/trypanic/Code/github.com/trypanic/go-sdk
git switch audit/2026-11
git status --short                 # must be clean
git worktree list                  # 7 worktrees under ../go-sdk-wt/{C1..C6,X}
```

Read, in this order, and treat them as binding:

1. `docs/audit/HANDOFF.md` — exact state of every branch, pending review
   notes, follow-ups, decisions, environment gotchas.
2. `docs/audit/audit.md` — §0 (resume protocol), §3 (scope & rules), §6
   (validation checks), §7 (hard limits), §8.1–§8.4 (execution graph, review
   loop, models, brief templates), §9 (ledger = source of truth).
3. `docs/audit/REMEDIATION_PLAN.md` — §12 (G1 decision, v2 scope, chosen
   alternatives) and the items you dispatch.
4. `AGENTS.md` (repo conventions) and the role files
   `.claude/agents/go-auditor.md`, `go-reviewer.md`, `go-remediator.md`
   (use them as subagent role instructions; they are vendor-neutral text).

Decisions already made by the owner — **do not ask again**: every approval
item is approved, breaking changes allowed, next release **2.0.0** (module
path `/v2` via R-160), licence **MIT** (done).

## 1. Your role and rules

- You are the **orchestrator**: you own `docs/audit/audit.md` (ledger),
  `REMEDIATION_PLAN.md` status table and `HANDOFF.md`. Subagents never edit
  those files; they work in their worktree and report back.
- Models/effort: `audit.md` §8.3, column for your vendor. Remediation of
  concurrency/security items and all reviews at high effort; P5 at medium.
- **Reviewer ≠ author**: every review runs in a fresh subagent that did not
  write the code. A cross-vendor review (Kimi reviews GPT work or vice versa)
  is preferred when both are available.
- Review loops are capped at 3 per task; after a third CHANGES_REQUESTED set
  the task to `BLOCKED` and ask the owner.
- After every status change: edit the ledger row (status, owner, ISO
  timestamp, commit, reviewer, notes) and commit on `audit/2026-11` as
  `chore(audit): <task-id> <STATUS>`. Re-read the file before each edit.
- Hard limits: never `git push`, never tag or release, never edit
  `CHANGELOG.md` or `.release-please-manifest.json`, only R-DEPS/R-160 touch
  `go.mod`/`go.sum`, no traffic to external services from tests.
- Commits: Conventional Commits, item and finding IDs in the body, breaking
  changes with `!` and a `BREAKING CHANGE:` paragraph placed directly before
  the trailers. Never `--no-verify`; if a hook rewrites files, re-add and
  commit again. Add your own attribution trailer if your tool requires one.
- Talk to the owner in **Spanish**; write every file, code, commit and
  subagent brief in **English**.

## 2. Step A — finish the P4 reviews (run these four in parallel)

All four branches are complete and clean. Dispatch one reviewer subagent per
row, using the reviewer template in `audit.md` §8.4 plus the specifics
below. Set each row to `IN_REVIEW` (C1/C5 move from `CHANGES_REQUESTED`)
before dispatching.

| Task | Branch / worktree | Head | Loop | What the reviewer must cover |
| --- | --- | --- | --- | --- |
| P4.C1 | `audit/2026-11-C1`, `../go-sdk-wt/C1` | `d449128` | 2 | First run `go build ./... && go vet ./... && go test -race -short -count=3 ./errorkit/... ./logger/... ./telemetry/... && go test -short ./...` (the worker stopped before reporting). Then re-verify every loop-1 point listed in HANDOFF §2 "C1" — especially reproduce the escaped-key redaction leak (`to\tken`, `pass\nword`, `\/` in nested JSON) against `newLoggerWithWriter(&buf, Prod)` and confirm it is fixed by `330963c`; confirm `11e4780` carries the BREAKING paragraph for the non-comparable `logger.Config`; `-count=3` and `-count=5 -shuffle=on` for errorkit. |
| P4.C2 | `audit/2026-11-C2`, `../go-sdk-wt/C2` | `c7977c5` | 1 (fresh) | Full review of `audit/2026-11...audit/2026-11-C2` against items R-014, R-015, R-006, R-008, R-002, R-013, R-049, R-133, R-016, R-032, R-072, R-085, R-017, R-056, R-057, R-058, R-043, R-044, R-047, R-047b, R-048, R-095, R-121, R-122, R-102/R-154 (C2 shares) and extra fix `f91fca8`. Points in HANDOFF §2 "C2": R-017 deviation (no pre-bind), R-056 is `auto` but changes `Shutdown` semantics, `f91fca8` 404→405, renamed test `…WhenTimeoutZero`→`…Negative`. Secrets/internal text must never reach clients, logs, payloads or spans. |
| P4.C4 | `audit/2026-11-C4`, `../go-sdk-wt/C4` | `422f509` | 2 | Re-verify loop-1 points (HANDOFF §2 "C4"): symlink at the final `.jsonl`/`.json` path never read or written through, including the swap-between-read-and-append race (`d5fc424`); `GOOS=windows go vet ./storage/`; legacy 125-byte keys reachable and `GetAll` not broken (`7607354`, and confirm the 125→126 change in `TestStorageErrorCodes` is not a weakening); garbled last line (`97027bd`); SQLSTATE 57014/55P03 (`422f509`); mode validation (`7741c14`). |
| P4.C5 | `audit/2026-11-C5`, `../go-sdk-wt/C5` | `29e6a03` | 2 | Re-verify loop-1 points (HANDOFF §2 "C5"): `Close` racing reconnect returns nil (`9676d59`); stale-return test fails when `drainReturns` is deleted (`ac8e6c0`); `29e6a03` declares the `PubSub.Publish` code change and non-comparable `PubSub` as BREAKING; `4a15433`, `d745334`, `59bbc56`. Run `go test -race -short -count=10 ./messaging/...`. |

Every reviewer also runs, in its worktree: `go build ./...`, `go vet ./...`,
the package race tests, `go test -short ./...`, staticcheck, and gorelease
from a **clean detached temporary worktree** of the branch
(`go run golang.org/x/exp/cmd/gorelease@latest -base=v1.0.1`, then remove
the worktree). Every incompatible change must map to an `approval` item; a
break in an `auto` item is a finding. To prove a regression test fails
without its fix, check out the parent commit in a temporary worktree under
`../go-sdk-wt/review-<cluster>-*`, apply only the test files, run, remove;
for new-API items use a mutation on HEAD in a temporary copy. Reviewers are
read-only and return `APPROVE` or `CHANGES_REQUESTED` with numbered,
verifiable reasons.

On `CHANGES_REQUESTED`: dispatch a remediator subagent (template §8.4) into
the same worktree with the numbered reasons; it adds new commits; then send
the branch back to a reviewer (next loop). On `APPROVE`: set the row to
`DONE` with the reviewer's non-blocking notes in Notes.

## 3. Step B — P4.M merge (sequential, you do it)

Only when C1, C2, C4, C5 are `DONE` (C3, C6, X already are):

1. Claim `P4.M` in the ledger.
2. On `audit/2026-11`, merge one branch at a time with `git merge --no-ff`
   in this order: X, C6, C1, C3, C4, C5, C2. After each merge run §6 V1–V5
   (`gofmt -l .`, `go build ./...`, `go vet ./...`, `task test`,
   `go test -race -short ./...`). Fix only integration breakage (record it).
3. Merge `origin/main` (release 1.0.1 commit `524ea11`, touches only
   `CHANGELOG.md` and `.release-please-manifest.json`; expect no conflict).
4. Re-run the cross-cluster tests called out in HANDOFF §3: grpc
   registry-walk tests (`go test ./grpc/...`) and messaging retry tests
   (C6 changed `algorithms` defaults).
5. Run the full §6 table (V1–V11) and save results for P5. `task
   staticanalysis` should now pass (R-100 merged); `task release:check` will
   report the approved breaking changes.
6. Remove the cluster worktrees (`git worktree remove`), keep the branches.
7. Set `P4.M` to `DONE` (no review required).

## 4. Step C — P4.XP (sequential, after P4.M)

One remediator subagent works directly on `audit/2026-11` (or a single new
worktree `audit/2026-11-XP`), implementing the X post-merge items in this
exact order (from `REMEDIATION_PLAN.md` §12):

R-092, R-089, R-080, R-081, R-083, R-067, R-086, R-087, R-088, R-090,
R-112, R-113, R-020, R-091, R-132, R-093, R-094, R-155, R-153 (last).

Include the extra follow-ups listed in HANDOFF §3 item 2 (messaging
`MaxElapsedTime`, atomic `logger.global`, postgres `appendJSONArg` code,
grpc wrapped ctx text, urlkit `%2F`/opaque base, logger README hertz
references, errorkit code catalogue, `doc.go` package clauses for R-093,
`hostKeyword` with `net.SplitHostPort` in R-113).

R-153 is the **only** item that edits `AGENTS.md`: it applies every
`AGENTS.md → R-153` wording recorded in the commit bodies listed in HANDOFF
§3, corrects the root docs, and **keeps the "Ongoing work" notice at the top
of AGENTS.md** until P5 is `DONE`.

Then dispatch a reviewer for P4.XP (diff from the P4.M merge commit to the
P4.XP head) and loop as in Step A.

## 5. Step D — R-DEPS and R-160 (alone, after P4.XP)

One remediator subagent, one commit per change:

1. `toolchain go1.27.3` in `go.mod` (keep `go 1.26.1`); align
   `release.yml` if needed.
2. `golang.org/x/crypto`: bump only if a **tagged** release contains the
   GO-2026-5932 fix; otherwise leave it and keep the SECURITY.md triage.
3. R-125: `//go:build integration` testcontainers suites for Postgres, Mongo
   and RabbitMQ plus a separate Task target (tests must not run in `-short`).
4. **R-160**: `module github.com/trypanic/go-sdk/v2`; rewrite every import
   and every import path in READMEs, AGENTS.md, llms.txt and examples;
   remember the string constant in `ioutils/imports_test.go`; commit as
   `feat!:` with a BREAKING paragraph about the new import path.

Validate with §6 V2–V8. Reviewer checks per §8.2 (R-DEPS row).

## 6. Step E — P5 validation and report

One subagent (medium effort) runs every §6 check on the final tree, compares
with `docs/audit/baseline.md`, and writes `docs/audit/REMEDIATION_REPORT.md`
(per item: problem, solution, files, tests, checks; plus deferred items —
R-159, the C1-018 payload-in-FormatPlain part, anything not done — residual
risks, and the list of consumer-visible behavior changes for the 2.0.0
release notes, including those accepted in `auto` commits listed in
HANDOFF §2). Then remove the "Ongoing work" notice from `AGENTS.md`, set P5
to `DONE`, and tell the owner (in Spanish) what to do next: push the branch
and open a PR, run CI via `gh workflow run ci.yml`, and the owner-only
GitHub actions in HANDOFF §5.

## 7. End of every session

Append a dated "Session N (<agent/model>)" entry to `docs/audit/HANDOFF.md`
§7 with what was finished, what is in progress (branch heads), and what is
next; update HANDOFF §1 table; commit as `chore(audit): update handoff`.

## 8. Environment notes (details in HANDOFF §6)

zsh with `ls` aliased to `eza` (use globs or `find`); quote
`--include='*.go'`; no `gh` CLI (use `git ls-remote`); local semgrep crashes
on Python 3.15 (use the committed `scripts/staticanalysis.sh` after X is
merged, which runs the pinned Docker image); copy `.skillshare/skills/` into
any new worktree until R-141 is merged; golangci-lint offline with
`GOPROXY=file:///home/trypanic/.go/pkg/mod/cache/download GOSUMDB=off`;
benchstat is not installable; use `-base=v1.0.1` for gorelease.
