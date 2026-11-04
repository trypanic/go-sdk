# Audit handoff — state on 2026-11-04 (end of session 1)

Read this first, then `audit.md` §0 and §9 (ledger), then
`REMEDIATION_PLAN.md` §12 (G1 decision and v2 scope). This file is the
orchestrator's working memory; the ledger is the source of truth for task
status.

## 1. Where things are

- Orchestration branch: `audit/2026-11` (main checkout
  `/home/trypanic/Code/github.com/trypanic/go-sdk`). It holds only the audit
  documents; no remediation code is merged yet.
- Cluster branches, one worktree each under
  `/home/trypanic/Code/github.com/trypanic/go-sdk-wt/<cluster>`, all based on
  `db8d2c8` (`audit/2026-11` at P4 start). Every worktree is clean
  (committed); nothing is pushed.

| Cluster | Branch head | Commits | Ledger status | Next action |
| --- | --- | --- | --- | --- |
| C1 errorkit/logger/telemetry | `d449128` | 30 | CHANGES_REQUESTED (loop 1 fixes committed) | Run final checks, then re-review (loop 2) |
| C2 http*/llmclient | `c7977c5` | 32 | IN_REVIEW (review interrupted, no verdict) | Start a fresh review (loop 1) |
| C3 grpc | `f58b77b` | 15 | DONE | Merge in P4.M |
| C4 postgres/mongo/storage | `422f509` | 31 | IN_REVIEW loop 2 (review interrupted, no verdict) | Re-review loop 2 |
| C5 messaging | `29e6a03` | 30 | CHANGES_REQUESTED (loop 1 fixes committed) | Re-review loop 2 |
| C6 utilities | `5d5c63d` | 15 | DONE | Merge in P4.M |
| X CI/tooling/docs | `1aa63f3` | 25 | DONE (approved at loop 3) | Merge in P4.M |

Review loops are capped at 3 (`audit.md` §8.2); after the third
CHANGES_REQUESTED a task goes BLOCKED for the owner.

## 2. Pending review details

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
