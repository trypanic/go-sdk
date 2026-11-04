# Audit handoff — audit continuation in progress

Read this first, then `audit.md` §0 and §9 (ledger), then
`REMEDIATION_PLAN.md` §12 (G1 decision and v2 scope). This file is the
orchestrator's working memory; the ledger is the source of truth for task
status.

## 0. Who continues and how to find this file

- **Step-by-step task file for the next orchestrator (with subagents):**
  [`tasks/continue-audit.md`](../../tasks/continue-audit.md).

- The work continues with **GPT-6.1 Astra** and possibly **Kimi K3.1 Pro**
  (not Claude). Model and effort per role: `audit.md` §8.3 (columns for both
  were added; the Claude columns record how sessions 1 ran).
- Entry points that any agent reads: the notice at the top of the root
  [`AGENTS.md`](../../AGENTS.md) (on branch `audit/2026-11`) points here, and
  `audit.md` starts with the same pointer. Check out `audit/2026-11` first:
  `main` does not have these files.
- The Claude-only memory under `~/.claude/` is a convenience copy; nothing
  there is required.
- `.claude/agents/*.md` are Claude Code subagent definitions. Other agents
  use them as plain role instructions together with the brief templates in
  `audit.md` §8.4 (see §8.3 "How non-Claude agents map the roles").
- R-153 (P4.XP) rewrites `AGENTS.md`: it must keep the "Ongoing work"
  notice at the top until P5 is `DONE`, then remove it.
- Each new session should append a short dated "Session N" note at the end
  of this file (what it finished, what it left), so the next agent of any
  vendor can resume.

## 1. Where things are

- Orchestration branch: `audit/2026-11` (main checkout
  `/home/trypanic/Code/github.com/trypanic/go-sdk`). All approved cluster code
  and origin/main v1.0.1 are merged at `dab2c29`; P4.M is DONE.
- Cluster branches are retained, based on `db8d2c8` at P4 start. All cluster
  worktrees and the stale detached C2 loop-2 review worktree were removed
  after verification. Nothing is pushed.

| Cluster | Branch head | Commits | Ledger status | Next action |
| --- | --- | --- | --- | --- |
| C1 errorkit/logger/telemetry | `f0808cb` | — | DONE (loop 3 approved) | Merged in P4.M |
| C2 http*/llmclient | `9b930df` | — | DONE (owner-authorized loop 4 approved) | Merged in P4.M |
| C3 grpc | `f58b77b` | 15 | DONE | Merged in P4.M |
| C4 postgres/mongo/storage | `422f509` | 31 | DONE (loop 2 approved) | Merged in P4.M |
| C5 messaging | `29e6a03` | 30 | DONE (loop 2 approved) | Merged in P4.M |
| C6 utilities | `5d5c63d` | 15 | DONE | Merged in P4.M |
| X CI/tooling/docs | `1aa63f3` | 25 | DONE (approved at loop 3) | Merged in P4.M |

Review loops are capped at 3 (`audit.md` §8.2); after the third
CHANGES_REQUESTED a task goes BLOCKED for the owner.

- **P4.XP in progress:** `audit/2026-11-XP` in `/tmp/go-sdk-audit-XP`.
  All nineteen items and followups committed at clean head `6a390d4`,
  with R-153 last. Author full gates complete; independent loop1 review
  in progress. Report on XP branch: `validation/P4.XP.md`. Independent review follows the complete XP sequence.

## 2. Pending review details

**Latest:** C2 passed the owner-authorized loop 4 at `9b930df`; all clusters
are DONE and merged. P4.M validation: `validation/P4.M.md`. Next: P4.XP.
C2 review: `reviews/P4.C2-loop4.md`. Historical gate details follow.


**Owner continuation decision:** the owner replied "continua" after the
loop-3 gate, authorizing one additional correction and independent review
round for C2. The historical BLOCKED details below remain as evidence.


**Continuation update:** C1 passed final loop 3 at `f0808cb`; C4 and C5
passed loop 2. C2 is **BLOCKED** at `6586058` after loop 3 found a HIGH
URL-header leak: `Referer` and `Location` query secrets reach outbound audit
and default inbound audit. The orchestrator independently reproduced the
outbound leak. Evidence and preserved reproduction sources:
[`reviews/P4.C2-loop3.md`](./reviews/P4.C2-loop3.md). The two loop-2 fixes
are committed and independently verified: `6ce5b4a` prevents redirected
POST replay, `6586058` masks nested redirect diagnostics. The response-size
fix `48c1971` also passed mutation verification. Owner authorization is
required before any extra remediation/review cycle; do not merge C2 or
start P4.M until it is approved. All other clusters are DONE.
The following subsections preserve the original reproduction details.


### C1 — loop 1 CHANGES_REQUESTED, fixes committed after `f901d61`

The worker was stopped before its final report, so its final checks were
not reported. All requested fixes are on the branch:

| Review point | Commit |
| --- | --- |
| R-007 marked breaking (Config no longer comparable): original `f0601ed` reworded via rebase, all later SHAs changed | `11e4780 fix(logger)!: widen log redaction…` |
| R-130 fast path skipped keys with JSON escapes / `\/` (security) | `330963c` |
| errorkit OverrideMetadata tests not repeatable with `-count=3` | `9e80073` |
| InitLogProvider/SetupOTLP must stay never-failing | `2c3f3a0` |
| more redaction suffixes, full URL password masking | `8f65f47` |
| HasCode walks `errors.Join` trees | `77ec696` |
| dotted import paths in traces (`gopkg.in/yaml.v3`) | `d449128` |

Before the re-review, run in the C1 worktree:
`go build ./... && go vet ./... && go test -race -short -count=3 ./errorkit/... ./logger/... ./telemetry/... && go test -short ./...`.
The loop-1 review notes are in the ledger row and must be re-verified
(especially the escaped-key redaction reproduction).

Behavior changes in C1 `auto` commits accepted by the orchestrator (list
them in the release notes): `ERROR_KIT_MAX_STACK` now strictly parsed as an
integer (`123ea9e`); `logger.Error(nil)` logs `ERR_UNKNOWN`; trace file paths
are module-relative.

### C2 — no verdict yet

Review the whole range `audit/2026-11...audit/2026-11-C2` (items in the
ledger brief). Points to examine: the R-017 deviation (no pre-bind with
`WithListener`; the bind panic is converted into an error instead), R-056
(auto) changes `Shutdown` to return a timeout error and tracks in-flight
requests, extra fix `f91fca8` (hertz `EnableNoMethod` now returns 405 — a
defect not in the audit; record it as a new finding in the report), the
renamed test `TestExecuteNoDeadlineWhenTimeoutZero` → `…Negative`.
Open note from the worker: `hostKeyword` should use `net.SplitHostPort`;
deferred to R-113 (P4.XP).

### C4 — loop 2, fixes committed after `e7465c1`

`d5fc424` (no symlink follow at the final path: Lstat + `O_NOFOLLOW` on unix +
`SameFile`), `7607354` (legacy 125-byte keys readable; `TestStorageErrorCodes`
moved from 125 to 126 bytes for List/Delete — confirm it is not a
weakening), `97027bd` (garbled last line handled like a torn one), `422f509`
(SQLSTATE 57014/55P03 → TIMEOUT), `7741c14` (mode validation), `b07f5ae`
(README compatibility). Re-run the reviewer's reproductions.

### C5 — loop 1 CHANGES_REQUESTED, fixes committed after `e366890`

`9676d59` (Close racing reconnect returns nil), `ac8e6c0` (stale-return test
now exercises `drainReturns`), `29e6a03` (`docs(messaging)!`: declares that
`PubSub.Publish` returns specific `ERR_MQ_*` codes and that `PubSub` is no
longer comparable — accepted under approved R-025b), `8ce445e` (retry defaults
after C6 merge, DLX/amq.* docs), `4a15433` (markBroken on a nack that closed
the channel), `d745334` (Close racing Subscribe fails closed), `59bbc56` (log
recovered panics), `1179adf` (example errcheck).

## 3. What comes after the reviews

1. **P4.M** (sequential, `audit/2026-11`): merge each DONE cluster branch one
   at a time (`--no-ff`), run §6 checks after each. Also merge `origin/main`
   (release 1.0.1 commit `524ea11`: only `CHANGELOG.md` and
   `.release-please-manifest.json`; no conflicts expected). After merging,
   re-run the grpc registry-walk tests (they import C2/C4/C5 packages) and
   messaging retry tests (C6's R-035 changes `algorithms` defaults). Remove
   the worktrees when done.
2. **P4.XP** (sequential, after P4.M): X post-merge items in the order given
   in `REMEDIATION_PLAN.md` §12, ending with R-153 (sole editor of
   `AGENTS.md`). The proposed AGENTS.md wording for R-153 is in the commit
   bodies of: C1 `0272620` (PublicJSON) and `f6c6d5f` (As/HasCode); C2 R-016,
   R-085, R-048 commits; C5 `328b36b` (§2.4, §3); C6 `aaf363a`, `4102bfe`;
   X `14601a4`, `228a5e1`, `e14d5e7`, `dc33129` (line 14: use the R-152
   version). Also add: DetectEnv defaults to Prod; NewLogProvider for TLS.
   Extra follow-ups collected for P4.XP:
   - messaging: set an explicit `MaxElapsedTime` (C6 removed the implicit
     15-minute cap; C6 review N1).
   - `logger.global` plain pointer: Init concurrent with logging races
     (C1 worker note) — fix with an atomic pointer.
   - postgres `appendJSONArg` marshal failure still returns the retriable
     generic code (C4 worker note).
   - grpc `status.FromContextError` sends wrapped ctx error text (C3 N2;
     consider in R-020).
   - urlkit edge cases: `%2F` in path, opaque base (C6 review N2).
   - logger/README.md:415, :616 reference `httpserver.RequestLoggerMiddleware`
     — should be `hertz.RequestLogger` (C2 worker note).
   - errorkit README catalogue: add `ERR_REQUEST_CANCELED`, `ERR_DB_MONGO_CANCELED`, `ERR_MQ_CLOSED`.
   - R-093 must update the `doc.go` package clauses of postgres/mongo.
3. **R-DEPS** (alone): `toolchain go1.27.3` (or latest patched 1.26.x),
   x/crypto bump only to a tagged fix, R-125 testcontainers suites, then
   **R-160** module path `github.com/trypanic/go-sdk/v2` (rewrite all imports
   and docs; remember the string constant in
   `ioutils/imports_test.go:14`).
4. **P5**: run §6 on the final tree, compare with `baseline.md`, write
   `REMEDIATION_REPORT.md` (list deferred: R-159; C1-018 payload-in-FormatPlain
   part; integration suites if not done).

## 4. Decisions already taken (do not re-ask)

- Owner approved **all** approval items including breaking changes; release
  will be **2.0.0** (module path `/v2` via R-160). Licence **MIT** (done in X).
- Alternatives chosen: see `REMEDIATION_PLAN.md` §12. R-148: sentrux removed
  (binary needs OpenSSL 3; host has OpenSSL 4).
- Release gate: CI runs on the release-please PR only via manual
  `gh workflow run ci.yml --ref <branch>`; docs recommend required checks on
  `main` (owner action).
- Breaking commits: `!` in the type plus a `BREAKING CHANGE:` paragraph
  placed directly before the `Co-Authored-By` trailer.

## 5. Owner actions (outside the repo)

- Enable GitHub private vulnerability reporting.
- After a test PR, protect `main` with the CI checks listed in `ci.yml`
  (note: govulncheck as a required check blocks PRs on new reachable
  advisories).
- Optional: GitHub App / fine-grained PAT for release-please.

## 6. Environment gotchas

- Shell is zsh; `ls` is aliased to `eza` (use globs/`find`); quote
  `--include='*.go'`; `set -- $var` does not split.
- `go` on PATH is 1.26.2 in some shells, proto resolves 1.27.3 in the repo.
- Local semgrep crashes on Python 3.15 (protobuf TypeError); the committed
  runner `scripts/staticanalysis.sh` (branch X) uses the pinned Docker image.
- `.skillshare/skills/` is gitignored; copy it into new worktrees or the
  pre-commit `staticanalysis` hook fails (fixed by R-141 once X is merged).
- golangci-lint via `go run` may be blocked by the permission system; it runs
  offline with `GOPROXY=file:///home/trypanic/.go/pkg/mod/cache/download GOSUMDB=off`.
- `gh` CLI is not installed; use `git ls-remote` for read-only SHA lookups.
- benchstat is not installable here.
- Tag `v1.0.1` exists on origin and locally; use `-base=v1.0.1` for gorelease.
- Reviewer technique: for new-API items, prove "fails without the fix" with a
  mutation on HEAD; when changing a write path from rename to in-place,
  re-check symlink handling at the final path.

## 7. Session log

- **Session 1 (2026-11-04, Claude Code, Opus 5.5):** P0.1 → P3, G1 (owner
  approved everything, 2.0.0, MIT), P4 workers for all clusters; C3, C6, X
  approved; C1, C4, C5 loop-1 fixes committed; C2 awaiting first review.
  Handed over to GPT-6.1 Astra / Kimi K3.1 Pro.

- **Session 2 (2026-11-04, Codex orchestrator with GPT-6.1 Astra
  reviewers/remediator):** resumed from the newer ledger, fetched refs,
  approved C1 loop 3 (`f0808cb`), recorded new C2-028 routing defect,
  reconciled plan item statuses with branch commits. C2 loop 2 found
  redirect replay and nested diagnostic leaks; remediator committed
  `6ce5b4a` and `6586058`. Fresh loop-3 reviewer verified those fixes but
  found URL-bearing-header leaks in both audit directions. C2 is BLOCKED
  under the three-loop cap; owner must decide whether to authorize another
  fix/review cycle. No cluster merged, pushed, tagged or released; P4.M,
  P4.XP, R-DEPS and P5 remain TODO. Cluster heads are in §1. Seven cluster
  worktrees remain, plus the pre-existing clean detached C2 loop-2 review
  worktree at `48c1971`; current reviewer temporary worktrees were removed.
  `/tmp/audit-merge-checks.py` is prepared for sequential V1–V5 logging.
  Stop-point ledger commit: `5f0e1cf` (`chore(audit): P4.C2 BLOCKED`).

- **Session 3 (in progress, Codex orchestrator / GPT-6.1 Astra):** owner
  authorized an extra C2 round via "continua"; header leak fixed `9b930df`,
  fresh loop4 APPROVE. P4.M DONE: all clusters + origin/main merged through
  `dab2c29`, no integration conflicts; per-merge V1-V5 and cross-cluster tests
  pass, coverage88.5%, V7/V8/V11 pass,7 Semgrep warnings,9 informational test
  lint issues, V10 approved v2 breaks. Cluster/review worktrees removed,
  branches retained. P4.XP is in progress in `/tmp/go-sdk-audit-XP` through
  `0d99d79` (15 items); next R093/R094/R155/R153 last and remaining followups.
  Independent review pending. R-DEPS/P5 not started. Read-only R-DEPS
  preparation: `/tmp/audit-deps-preflight.md`; P5 inventory:
  `/tmp/audit-remediation-inventory-P4M.json`. Final session entry will update
  this progress snapshot when work stops or finishes.
