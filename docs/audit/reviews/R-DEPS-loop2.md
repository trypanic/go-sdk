# R-DEPS independent review — loop 2

**Verdict: APPROVE.** Both blocking loop-1 findings are corrected and independently
verified. No new correctness, security or compatibility blocker was found.
One new test-helper style warning is nonblocking and recorded below.

Reviewed head: `ddf7afd873c661ef70e50f94882e4b01b98d8549` on
`audit/2026-11-DEPS`; base: `5e57bba`; prior reviewed head: `ac8ab8e`.
Fix commits: `40580e0` and `f32d1f3`. Reviewer: independent Codex /
GPT-6.1 Astra, 2026-11-04, fresh context and not the author.

The assigned worktree `/tmp/go-sdk-audit-DEPS` remained clean at the same head.
All executions that could modify files, including hooks, ran in a disposable
Git clone or exported mutation copies. No source edits, commits, dependency
changes, pushes, tags, releases or ledger changes were made on the assigned tree.

## 1. Actual service and Ryuk publications are constrained before startup

**Locations:** `internal/integrationtest/container.go:32`, `:48`, `:53`,
`:74`, `:79`, `:108`; `internal/integrationtest/container_test.go:14`.

The service request is modified before `GenericContainer` creates it. The
reaper's provider wrapper applies the same modifier before delegating to the
original Docker provider. The modifier composes the prior HostConfig modifier,
disables publish-all, and explicitly binds the requested ports to `127.0.0.1`
with `HostPort: "0"`, retaining ephemeral allocation. It preserves the reaper's
socket mount, bridge mode and auto-remove configuration.

The real test run used `-race -count=2 -shuffle=on`. All three adapter suites
executed twice without skips. Runtime Docker inspection recorded:

| Listener | First / second observed host publication |
| --- | --- |
| PostgreSQL `5432/tcp` | `127.0.0.1:32783` / `127.0.0.1:32787` |
| MongoDB `27017/tcp` | `127.0.0.1:32785` / `127.0.0.1:32786` |
| RabbitMQ `5672/tcp` | `127.0.0.1:32784` / `127.0.0.1:32788` |
| Shared session Ryuk `8080/tcp` | `127.0.0.1:32782` |

The assertions inspect every actual published binding on each owned service
and the session reaper. This is not a client-hostname check. The shared reaper
was observed by each package process; each process checks its own service,
avoiding inspection races with other processes' completed services.

Two independently constructed mutations prove detection:

- Removing only the service's `bindLoopback` call fails with
  `publishes 5432/tcp on 0.0.0.0`, exit 1.
- Removing only the reaper wrapper's `bindLoopback` call fails with
  `publishes 8080/tcp on 0.0.0.0`, exit 1.

[Repeated integrations](./R-DEPS-loop2/integration-repeat.log),
[service negative control](./R-DEPS-loop2/service-wildcard.log),
[reaper negative control](./R-DEPS-loop2/reaper-wildcard.log),
[independent probe runner](./R-DEPS-loop2/probes.py).
Exact mutation patches are preserved as `.patch.b64` files beside the logs.

## 2. The upstream reaper lifecycle and crash cleanup remain effective

**Locations:** `internal/integrationtest/container.go:40`, `:47`, `:50`,
`:56`, `:120`; `CONTRIBUTING.md:76`.

I read the pinned upstream `reaper.go` implementation, including `NewReaper`,
the session singleton/reuse path, `ReaperProvider`, `newReaper` and the original
HostConfig modifier. The wrapper delegates lifecycle management and uses the
same configured session ID. It does not replace the reaper protocol, disable
Ryuk, or alter the host Docker daemon or firewall. The explicitly approved
`SA1019` exception is one localized integration-only call, documented with the
missing upstream HostIP option and replacement condition. Tagged staticcheck
passes without a global suppression.

A new reviewer-owned standalone test binary started a service and reaper, then
paused for independent `docker inspect`. Inspection confirmed both were running,
all their publications were loopback, and Ryuk retained its socket bind and
auto-remove. The binary then called `os.Exit(23)`. A cleanup sentinel confirms
that test cleanup callbacks did not run. Bounded subsequent Docker polling found
zero containers for that session, including Ryuk. This independently verifies
process-exit cleanup rather than normal `t.Cleanup` alone.

[Probe source](./R-DEPS-loop2/crash-probe.gotxt),
[before-exit Docker inspection](./R-DEPS-loop2/crash-before.json),
[runtime output](./R-DEPS-loop2/crash-runtime.log),
[cleanup assertion](./R-DEPS-loop2/crash-result.json).

The local Unix-socket and cached-image checks remain in place before creation.
Service images retain explicit version/digest references; Ryuk's cached tag is
checked against its pinned RepoDigest. Readiness and ordinary termination retain
bounded contexts. Normal tests exclude Testcontainers; tagged short tests skip
infrastructure before Docker access, including with a nonexistent socket.

## 3. Both CI matrix jobs execute their selected toolchain

**Locations:** `.github/workflows/ci.yml:63`, `:79`, `:134`, `:152`;
`go.mod:3`, `:5`.

Both the build/test job and vulnerability job set `GOTOOLCHAIN=local` and assert
`go env GOVERSION` against the selected matrix line before their work. The
language directive remains `go 1.26.1`; the development directive remains
`toolchain go1.27.3`.

The independent [matrix runner](./R-DEPS-loop2/matrix.py) parses the committed
workflow and executes its actual `run` commands with the installed Go 1.26.10
and Go 1.27.3 binaries first on PATH, with GOROOT unset. Build, vet, race/shuffled
short tests, and the pinned govulncheck command all pass under each fixed version.
Tagged short tests also pass under each version. For each job's assertion,
the old `GOTOOLCHAIN=auto` environment with the Go 1.26.10 executable reports
`go1.27.3` and is rejected with exit 1. With `local`, it reports `go1.26.10`.

[Old-selection rejection](./R-DEPS-loop2/matrix-1.26.x-test-2-negative.log),
[Go 1.26.10 assertion](./R-DEPS-loop2/matrix-1.26.x-test-2.log),
[Go 1.27.3 assertion](./R-DEPS-loop2/matrix-1.27.x-test-2.log).
This agrees with the [official toolchain selection contract](https://go.dev/doc/toolchain).
These are local executions of the exact workflow commands, not claimed
GitHub-hosted workflow results.

## 4. Dependency and migration scope remains correct

**Locations:** `go.mod:1`, `:14`; `Taskfile.yml:31`, `:197`;
`ioutils/imports_test.go:14`; `errorkit/stack.go`.

Independent selected-module and graph comparisons show 45 new external graph
nodes and 35 new explicit requirements against `5e57bba`, no external removals,
and no version changes to previously selected external modules. Every added
node has a dependency parent. The selected graph is identical to `ac8ab8e`.
The only loop-2 manifest change promotes the existing Moby api/client versions
from indirect to direct; `go.sum` is unchanged.

[Dependency delta](./R-DEPS-loop2/scope.json),
[parent edges](./R-DEPS-loop2/dependency-parents.json),
[requirement summary](./R-DEPS-loop2/requirements-summary.json).

Official sources were fetched independently and preserved with URLs/checksums
in the [source manifest](./R-DEPS-loop2/sources/manifest.json). The
[Testcontainers v0.44.0 release](https://github.com/testcontainers/testcontainers-go/releases/tag/v0.44.0)
and manifest establish its Go 1.25 requirement. The
[go-archive v0.3.0 release](https://github.com/moby/go-archive/releases/tag/v0.3.0)
and [GO-2026-6253 advisory](https://vuln.go.dev/ID/GO-2026-6253.json)
confirm the security floor. Its manifest requires sys/user v0.4.1; that patch's
release notes were also read. The official crypto tag inventory still ends at
v0.58.0; the [GO-2026-5932 fix](https://vuln.go.dev/ID/GO-2026-5932.json)
is only a pseudo-version. The approved tagged-only decision remains valid.
Neither normal nor tagged-test import closure includes OpenPGP.

All 208 modified existing Go files equal their base versions after only replacing
the SDK import prefix with `/v2`. No prior test assertion was weakened. All 31
SDK package paths use `/v2`; normal test closure excludes Testcontainers.
The two permanent XP tests pass five times with `-race`. Independently removing
JSON suffix draining or deriving PubSub lifetime from its construction context
makes the corresponding committed test fail, exit 1:
[JSON mutation](./R-DEPS-loop2/xp-json.log),
[lifetime mutation](./R-DEPS-loop2/xp-lifetime.log).

The real adapter suites still verify stored-procedure results/classification,
Mongo result counts/cursor iteration/ownership, and RabbitMQ confirmed delivery,
mandatory returns and lifetime. Task selects actual package directories, and
both distinct explicit v2 release checks pass. Repository URLs, historical v1
examples and the ongoing-audit notice are preserved. Release-owned files are
untouched. Complete README and AGENTS examples independently build and vet.

## 5. Independent validation and actual exits

All commands and exits are in [checks.jsonl](./R-DEPS-loop2/checks.jsonl).
Checks ran on the exact reviewed head in an isolated clone unless identified
as intentional negative controls above.

| Check | Exit / result |
| --- | --- |
| V1 `gofmt -l .` | 0, no files |
| V2/V3 build and vet | 0 in `task check` and both actual toolchain matrix runs |
| V4 `task test` | 0, actual package tests executed |
| V5 race/shuffled short suite | 0 in `task check` and both matrix runs |
| V6 fresh short coverage | 0; total 87.5% |
| V7 static analysis | 0; vet/staticcheck clean, 12 Semgrep warnings, 0 errors |
| V8 production govulncheck | 0 under both supported toolchains |
| Additional `govulncheck -tags=integration -test ./...` | 0; 0 reachable/imported-package vulnerabilities, 1 module-only advisory |
| Full `task check` | 0, includes tidy-diff, module verify and hooks |
| V9 cached offline `task lint` | 0; 0 issues |
| V10 clean default `task release:check` | Task 201 / gorelease 1, cannot suggest across module paths; not a passing check |
| V10 `task release:check RELEASE_VERSION=v2.0.0` | 0; explicit `github.com/trypanic/go-sdk@v1.0.1` comparison and approved v2 changes |
| V10 `task release:check BASE=none RELEASE_VERSION=v2.0.0` | 0; log visibly passes `-base=none`, valid initial v2 release |
| V11 full standalone `task precommit:run` | 0; complete hooks including short tests and full static scan; clone remains clean |
| Real integration `-race -count=2 -shuffle=on` | 0; three services execute twice, modifier unit test also twice |
| Tagged vet / tagged staticcheck | 0 / 0 |
| Tagged short tests, actual Go 1.26.10 and 1.27.3 | 0 / 0 |
| Tagged short suite with nonexistent Docker socket | 0; infrastructure explicitly skips, modifier test executes |
| README/AGENTS complete examples, build and vet | 0 each; not executed |
| Final testcontainers Docker inventory | 0; empty |
| Assigned and clone final Git status | empty |

[Full gates](./R-DEPS-loop2/check.log),
[full hooks](./R-DEPS-loop2/hooks-full.log),
[tagged vulnerability scan](./R-DEPS-loop2/tagged-vuln.log),
[cross-major release comparison](./R-DEPS-loop2/release-comparison.log),
[initial-v2 release validation](./R-DEPS-loop2/release-initial.log).

Initial sandboxed attempts could not write the normal Go cache or access the
network. They are preserved separately and are not counted as successful gates;
in particular an initial `task test` returned 0 without selecting packages and
is explicitly excluded. Authorized reruns executed the actual work successfully.
There was no automatic-approval rejection or unresolved environment blocker in
this review. Hooks were run only in the disposable clone.

## 6. Nonblocking notes and limits

1. **LOW, style:** `internal/integrationtest/container.go:96` puts `*testing.T`
   before `context.Context`, triggering the repository's
   `context-must-be-first-param` Semgrep warning. Reordering these private helper
   parameters would remove it. This is test-only and has no demonstrated
   behavioral or security impact. The current total is **12 warnings**, not 11:
   11 previous TLS-fixture/random-jitter/lifecycle warnings plus this new one.
   Preserve this count and triage in P5.
   [Exact Semgrep report](./R-DEPS-loop2/report.semgrep.json).
2. `git diff --check 5e57bba..HEAD` exits 2 only for mixed indentation in the
   preserved historical `validation/R-DEPS/integration-first.log`. This was
   already identified in loop 1. The loop-2 diff itself is whitespace-clean;
   gofmt and repository hooks pass. This is diagnostic log formatting, not a
   source regression.
3. The approved deprecated `NewReaper` entry point remains a maintenance
   tradeoff until upstream exposes a supported scoped binding option. Tests
   cover one pinned local service version each, not arbitrary Docker setups
   or remote infrastructure. No external application database/API was used.

Read the root handoff, audit work order/gates/ledger, continuation Step D,
approved plan/G1, reviewer role, project AGENTS/SECURITY, adapter contracts and
errorkit registration/telemetry contracts. Applied the three required project
skills, pinned Eduardo router/dependency-audit/code-review/test-quality/context/
database/build-release skills and references at
`e25b517bbd0c11db35f75a83a14f18848c1abdb0`. JetBrains source was
`155dc7ca10da5e1f6c841503086957b1b37f5815`; its CLI ran for all 216 changed
Go files and produced one identical, fully read Go 1.26 guideline list. The
`errors_as_type` explanation was read; canonical errorkit matching and fresh
bounded cleanup contexts retain their required project semantics.

Evidence is packaged as text/JSON logs, exact base64 patches and `.gotxt`
examples so it cannot create Go packages under audit documentation. Temporary
execution clones and mutation copies were removed after preserving the evidence.
The orchestrator can now record R-DEPS as DONE and continue to P5.

## Repository evidence packaging

The orchestrator normalized whitespace/newlines in captured text for repository
hooks, retaining exact base64 patch bytes and non-build `.gotxt` programs.
`manifest.json` preserves the original capture manifest and records checksums of
repository artifacts; original raw files remain in `/tmp/DEPS-review2/`.
