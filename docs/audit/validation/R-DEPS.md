# R-DEPS / R-125 / R-160 validation

Implementation head: `1f40242` on `audit/2026-11-DEPS`; base `5e57bba`.
Date: 2026-11-04. Author: Codex dependencies remediator.
Status: ready for independent review; the author does not set DONE.
No push, tag, release, release-manifest edit, CHANGELOG edit or root ledger edit.

## Changes and commits

| Commit | Scope and findings | Result |
| --- | --- | --- |
| `7bcccb8` | R-DEPS; X3-001, X2-011, X3-002 | Add `toolchain go1.27.3`; retain `go 1.26.1`. Existing release workflow already uses supported `1.27.x`. |
| `73f81c6` | R-DEPS; X3-010 | Date and source the tagged-only crypto decision in SECURITY.md; keep v0.58.0. |
| `0c3c5ed` | XP review loop 1 N1; R-153/R-154 | Correct GroupDatabase literal, OTLP/Postgres links, actual request dependencies and explicit/default Hertz profiles. |
| `a8698a0` | XP review loop 1 N2; R-049/C2-007, R-067/C4-004/X1-009 | Preserve reviewer-proved one-byte JSON suffix and subscription-construction-lifetime probes as permanent tests. |
| `5ac4a99` | R-125; C4-009, C4-020, C5-012 | Add actual local PostgreSQL/Mongo/RabbitMQ suites, test-only Testcontainers requirement and separate Task targets. |
| `dc965b4` | R-DEPS/R-125 security follow-up | Fix newly introduced GO-2026-6253 with go-archive v0.3.0; accept required sys/user v0.4.1 patch. |
| `1f40242` | R-160; approved G1 v2 consequence | Change module/imports/constants/current docs to `/v2`, update Task package/base resolution and release proxy warm-up; breaking footer retained. |

R-082 keeps the single module, already documented in root README and AGENTS;
no artificial empty commit or module split. The test dependency is excluded
from normal package compilation by `integration`; only the internal helper's
package GoDoc remains visible for ordinary vet/staticcheck and staged hooks.

The two XP probes passed race tests. Their failures under the relevant behavior
mutations were independently established in `reviews/P4.XP-loop1.md` §3/§5 and
its portable sources; this change promotes those exact assertions with stable
names. No production behavior or existing assertion was weakened.

The final local reorder moves the security floor before R-160 and folds the
Task BASE-shadowing correction into R-160. Recorded Git tree hashes before and
after are identical. No previously published history was changed.

## Dependency decisions and primary sources

Testcontainers v0.44.0 is the approved infrastructure-test dependency. The core
module alone supplies lifecycle/readiness; no service-specific submodules are
needed. It is actively maintained, MIT licensed, pre-v1 API (confined to tests),
and declares Go 1.25.0, below the SDK language floor. Read the full
[v0.44.0 release notes](https://github.com/testcontainers/testcontainers-go/releases/tag/v0.44.0),
[manifest](https://raw.githubusercontent.com/testcontainers/testcontainers-go/v0.44.0/go.mod)
and [license](https://github.com/testcontainers/testcontainers-go/blob/v0.44.0/LICENSE).
The release addresses reaper startup handling and upstream security floors.
The standard library does not provide Docker orchestration, readiness or crash
cleanup; the approved Testcontainers library provides those facilities.

The additional integration-tagged vulnerability scan found GO-2026-6253 in the
new transitive go-archive v0.2.0. The [official advisory](https://vuln.go.dev/ID/GO-2026-6253.json)
sets v0.3.0 as fixed. Read both [v0.2.1](https://github.com/moby/go-archive/releases/tag/v0.2.1)
and [v0.3.0](https://github.com/moby/go-archive/releases/tag/v0.3.0) notes: extraction
containment and platform filesystem corrections justify the minor update.
Its [Go 1.25 manifest](https://raw.githubusercontent.com/moby/go-archive/v0.3.0/go.mod)
requires sys/user v0.4.1 and graph-only sys/mount v0.3.5. These MVS changes are
necessary consequences, not unrelated upgrades. Production V8 was already
clean; the tagged test scan becomes clean after this correction.

For GO-2026-5932, the [official advisory](https://vuln.go.dev/ID/GO-2026-5932.json)
was modified 2026-11-02 and names pseudo-version
`v0.58.1-0.20261009181718-2f1f834330b2` as fixed. The
[official tag inventory](https://proxy.golang.org/golang.org/x/crypto/@v/list)
rechecked 2026-11-04 still ends at v0.58.0. Keep the approved tagged requirement
and dated SECURITY triage. Neither normal nor integration test dependency
closures import OpenPGP. Production and tagged-test govulncheck report zero
reachable/imported-package vulnerabilities and one module-only advisory.

### Full selected-module version difference from base

No pre-existing selected dependency version changes. The SDK module itself
changes from `github.com/trypanic/go-sdk` to `github.com/trypanic/go-sdk/v2`.
The table includes every newly selected module, including graph-only nodes;
`modules-before.txt` and `modules-after.txt` preserve the full lists.

| Module | Before | Final | Requirement | Reason |
| --- | --- | --- | --- | --- |
| `dario.cat/mergo` | — (new) | `v1.0.2` | go.mod | Required by `github.com/testcontainers/testcontainers-go` |
| `github.com/AdaLogics/go-fuzz-headers` | — (new) | `v0.0.0-20240806141605-e8a1dd7889d6` | graph only | Required by `github.com/moby/go-archive` |
| `github.com/Azure/go-ansiterm` | — (new) | `v0.0.0-20250102033503-faa5f7b0171c` | go.mod | Required by `github.com/moby/moby/client`, `github.com/moby/term`, `github.com/testcontainers/testcontainers-go` |
| `github.com/Microsoft/go-winio` | — (new) | `v0.6.2` | go.mod | Required by `github.com/docker/go-connections`, `github.com/moby/moby/client`, `github.com/testcontainers/testcontainers-go` |
| `github.com/containerd/errdefs` | — (new) | `v1.0.0` | go.mod | Required by `github.com/containerd/errdefs/pkg`, `github.com/moby/moby/client`, `github.com/testcontainers/testcontainers-go` |
| `github.com/containerd/errdefs/pkg` | — (new) | `v0.3.0` | go.mod | Required by `github.com/moby/moby/client`, `github.com/testcontainers/testcontainers-go` |
| `github.com/containerd/log` | — (new) | `v0.1.0` | go.mod | Required by `github.com/containerd/platforms`, `github.com/moby/go-archive`, `github.com/testcontainers/testcontainers-go` |
| `github.com/containerd/platforms` | — (new) | `v0.2.1` | go.mod | Required by `github.com/testcontainers/testcontainers-go` |
| `github.com/containerd/typeurl/v2` | — (new) | `v2.2.0` | graph only | Required by `github.com/containerd/errdefs/pkg` |
| `github.com/cpuguy83/dockercfg` | — (new) | `v0.3.2` | go.mod | Required by `github.com/testcontainers/testcontainers-go` |
| `github.com/creack/pty` | — (new) | `v1.1.24` | graph only | Required by `github.com/moby/moby/client`, `github.com/moby/term` |
| `github.com/distribution/reference` | — (new) | `v0.6.0` | go.mod | Required by `github.com/moby/moby/client`, `github.com/testcontainers/testcontainers-go` |
| `github.com/docker/go-connections` | — (new) | `v0.7.0` | go.mod | Required by `github.com/moby/moby/client`, `github.com/testcontainers/testcontainers-go` |
| `github.com/docker/go-units` | — (new) | `v0.5.0` | go.mod | Required by `github.com/moby/moby/api`, `github.com/moby/moby/client`, `github.com/testcontainers/testcontainers-go` |
| `github.com/ebitengine/purego` | — (new) | `v0.10.1` | go.mod | Required by `github.com/shirou/gopsutil/v4`, `github.com/testcontainers/testcontainers-go` |
| `github.com/go-ole/go-ole` | — (new) | `v1.3.0` | go.mod | Required by `github.com/shirou/gopsutil/v4`, `github.com/testcontainers/testcontainers-go`, `github.com/yusufpapurcu/wmi` |
| `github.com/gogo/protobuf` | — (new) | `v1.3.2` | graph only | Required by `github.com/containerd/errdefs/pkg` |
| `github.com/lufia/plan9stats` | — (new) | `v0.0.0-20260330125221-c963978e514e` | go.mod | Required by `github.com/shirou/gopsutil/v4`, `github.com/testcontainers/testcontainers-go` |
| `github.com/magiconair/properties` | — (new) | `v1.8.10` | go.mod | Required by `github.com/testcontainers/testcontainers-go` |
| `github.com/moby/docker-image-spec` | — (new) | `v1.3.1` | go.mod | Required by `github.com/moby/moby/api`, `github.com/moby/moby/client`, `github.com/testcontainers/testcontainers-go` |
| `github.com/moby/go-archive` | — (new) | `v0.3.0` | go.mod | Required by `github.com/testcontainers/testcontainers-go`; v0.3.0 security floor for GO-2026-6253 |
| `github.com/moby/moby/api` | — (new) | `v1.55.0` | go.mod | Required by `github.com/moby/moby/client`, `github.com/testcontainers/testcontainers-go` |
| `github.com/moby/moby/client` | — (new) | `v0.5.0` | go.mod | Required by `github.com/testcontainers/testcontainers-go` |
| `github.com/moby/patternmatcher` | — (new) | `v0.6.1` | go.mod | Required by `github.com/moby/go-archive`, `github.com/testcontainers/testcontainers-go` |
| `github.com/moby/sys/mount` | — (new) | `v0.3.5` | graph only | Required by `github.com/moby/go-archive` |
| `github.com/moby/sys/mountinfo` | — (new) | `v0.7.2` | graph only | Required by `github.com/moby/go-archive` |
| `github.com/moby/sys/reexec` | — (new) | `v0.1.0` | graph only | Required by `github.com/moby/go-archive` |
| `github.com/moby/sys/sequential` | — (new) | `v0.7.0` | go.mod | Required by `github.com/moby/go-archive`, `github.com/testcontainers/testcontainers-go` |
| `github.com/moby/sys/user` | — (new) | `v0.4.1` | go.mod | Required by `github.com/moby/go-archive`, `github.com/testcontainers/testcontainers-go` |
| `github.com/moby/sys/userns` | — (new) | `v0.1.0` | go.mod | Required by `github.com/moby/go-archive`, `github.com/testcontainers/testcontainers-go` |
| `github.com/moby/term` | — (new) | `v0.5.2` | go.mod | Required by `github.com/moby/moby/client`, `github.com/testcontainers/testcontainers-go` |
| `github.com/opencontainers/go-digest` | — (new) | `v1.0.0` | go.mod | Required by `github.com/containerd/platforms`, `github.com/distribution/reference`, `github.com/moby/docker-image-spec` |
| `github.com/opencontainers/image-spec` | — (new) | `v1.1.1` | go.mod | Required by `github.com/containerd/platforms`, `github.com/moby/docker-image-spec`, `github.com/moby/moby/api` |
| `github.com/power-devops/perfstat` | — (new) | `v0.0.0-20240221224432-82ca36839d55` | go.mod | Required by `github.com/shirou/gopsutil/v4`, `github.com/testcontainers/testcontainers-go` |
| `github.com/russross/blackfriday` | — (new) | `v1.6.0` | graph only | Required by `github.com/opencontainers/image-spec` |
| `github.com/santhosh-tekuri/jsonschema/v5` | — (new) | `v5.3.1` | graph only | Required by `github.com/opencontainers/image-spec` |
| `github.com/shirou/gopsutil/v4` | — (new) | `v4.26.6` | go.mod | Required by `github.com/testcontainers/testcontainers-go` |
| `github.com/sirupsen/logrus` | — (new) | `v1.9.4` | go.mod | Required by `github.com/Microsoft/go-winio`, `github.com/containerd/log`, `github.com/containerd/platforms` |
| `github.com/testcontainers/testcontainers-go` | — (new) | `v0.44.0` | go.mod | Testcontainers core (R-125) |
| `github.com/tklauser/go-sysconf` | — (new) | `v0.4.0` | go.mod | Required by `github.com/shirou/gopsutil/v4`, `github.com/testcontainers/testcontainers-go` |
| `github.com/tklauser/numcpus` | — (new) | `v0.12.0` | go.mod | Required by `github.com/shirou/gopsutil/v4`, `github.com/testcontainers/testcontainers-go`, `github.com/tklauser/go-sysconf` |
| `github.com/yusufpapurcu/wmi` | — (new) | `v1.2.4` | go.mod | Required by `github.com/shirou/gopsutil/v4`, `github.com/testcontainers/testcontainers-go` |
| `golang.org/x/time` | — (new) | `v0.11.0` | graph only | Required by `github.com/moby/moby/client` |
| `gotest.tools/v3` | — (new) | `v3.5.2` | graph only | Required by `github.com/moby/go-archive`, `github.com/moby/moby/api`, `github.com/moby/moby/client` |
| `pgregory.net/rapid` | — (new) | `v1.2.0` | graph only | Required by `github.com/moby/moby/api` |

Two already-selected modules gain explicit indirect go.mod requirements without
a version change: `github.com/felixge/httpsnoop v1.1.0` and
`go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.72.0`, now
needed by the Docker client used in Testcontainers. In total, 35 requirements
are added to go.mod and 45 nodes to the selected module graph.

The extra go.sum entries authenticate these requirements and their dependency
tests. `go mod tidy -diff` and `go mod verify` pass; no replace/exclude directive
is introduced. The original SDK's runtime dependency versions remain unchanged.

## Integration behavior and pinned infrastructure

Each suite first checks `testing.Short()`. The shared helper then requires a
local Unix Docker socket and cached images, verifies the reaper digest, starts
only disposable containers, bounds readiness, verifies loopback endpoints and
registers termination before Start. Termination uses a fresh 30-second context;
Mongo/cursor cleanup likewise uses fresh bounded contexts. No application DSN,
cloud database, remote broker, volume or real external API is consumed.
Download happens separately through `task test:integration:pull`.

Image tags were checked against the Docker official image manifests for
[PostgreSQL](https://raw.githubusercontent.com/docker-library/official-images/master/library/postgres),
[MongoDB](https://raw.githubusercontent.com/docker-library/official-images/master/library/mongo)
and [RabbitMQ](https://raw.githubusercontent.com/docker-library/official-images/master/library/rabbitmq).
All actual pulls completed before testing. Docker engine: 29.9.2 linux/amd64.
On this host the image IDs equal the listed SHA-256 digest values.

| Image | Digest / actual image ID |
| --- | --- |
| postgres:18.6-alpine3.24 | `sha256:10a5c69a83035ab5239f5620d708520e8b28f5aa1f2b20143e6024532edadfb5` |
| mongo:9.0.4-resolute | `sha256:714cc3db1e396e5a464b8443ccde50e53283368809401f53f1997b684ddb5be0` |
| rabbitmq:4.4.0-alpine | `sha256:21577d148f4f7563a87d761862fa353f59348a094833ca8b1b1a46a8102dffce` |
| testcontainers/ryuk:0.14.0 | `sha256:7c1a8a9a47c780ed0f983770a662f80deb115d95cce3e2daa3d12115b8cd28f0` |

- PostgreSQL: real table and SQL functions, all four stored-procedure query
  variants, returned rows, no-row cause, unique-constraint SQLSTATE 23505,
  canceled context and server-side statement_timeout SQLSTATE 57014; pool
  remains usable after construction context cancellation.
- MongoDB: actual inserts, bulk upsert IDs, match/modify/delete counts,
  sorted batch-size-one cursor (getMore), decode errors, duplicate and missing
  document causes, canceled operations, borrowed adapter preserving ownership,
  owner Close disconnecting the driver.
- RabbitMQ: publish waits for confirmation, actual envelope consumption after
  setup cancellation, mandatory unroutable publish failing, subscription
  cancellation, Close during an active handler, terminal post-Close operations.
  Channels synchronize tests; no sleep-based synchronization.

The first run exposed two test-fixture errors: double-closing an owned Mongo
client and using a transient nonexclusive queue with RabbitMQ 4.4. The fixture
now closes exactly once and uses a durable queue; no SDK production change was
needed. RabbitMQ's [queue durability contract](https://www.rabbitmq.com/docs/queues)
explains the server restriction. Repeated race/shuffled runs and final corrected
graph runs pass. No Testcontainers-labeled container remained after validation.

## Migration and release validation

All runtime/test imports and module-path constants (including
`ioutils/imports_test.go` and stack expectations) use `/v2`. `go list ./...`
reports 31 `/v2` package paths. The dependency checks still exclude gRPC from
base logger, httprequest and llmclient, and exclude Testcontainers from normal
tests. Current READMEs, AGENTS and llms.txt use v2 imports. Repository URLs,
release-owned files and historical audit evidence remain unchanged. Explicitly
labeled v1 import examples remain in the migration guide. AGENTS keeps its
ongoing-audit notice until P5.

Task derives package names from each package's actual module path, so `task test`
selects the original package directories rather than a nonexistent `v2` directory.
The release workflow warms the module declared by go.mod. Gorelease's
[official cross-major baseline syntax](https://pkg.go.dev/golang.org/x/exp/cmd/gorelease)
is used with `github.com/trypanic/go-sdk@v1.0.1`.

The first `BASE=none` attempt exposed Task variable shadowing and actually
repeated the cross-major comparison. Its exit 0 is NOT counted as initial-v2
proof. The corrected command visibly passes `-base=none`; both final clean-tree
checks below are distinct. The default gorelease command cannot suggest a
version across different module paths and exits 1 (Task 201); specifying the
approved `v2.0.0` validates the comparison and reports all approved API changes.
No invalid unqualified v1 baseline is presented as passing.

## Checks and actual exits

Default toolchain: Go 1.27.3. Supported lower-line toolchain explicitly verified:
Go 1.26.10. The initial cache-restricted `go mod tidy -diff` failed with exit 1;
it was rerun with authorized cache access and passed. No permission bypasses,
hook skips to hide failures, disabled analyses or weakened assertions were used.

| Check | Actual exit | Evidence / interpretation |
| --- | --- | --- |
| V1 `gofmt -l .`, diff check | 0 / 0 | Empty format output; clean diff |
| V2 `go build ./...` | 0 | Also repeated inside final task check |
| V3 `go vet ./...` | 0 | Also repeated inside final task check |
| V4 `task test` | 0 | Dynamic package selection includes every package |
| V5 `go test -race -shuffle=on -short ./...` | 0 | Final task check |
| V6 short coverage | 0 | 87.4%; integration-only code excluded, no claim that coverage measures all behavior |
| V7 `task staticanalysis` | 0 | Vet/staticcheck zero errors; 11 known Semgrep warnings, no errors |
| V8 pinned govulncheck `./...` | 0 | Zero reachable/imported; crypto module-only advisory |
| Additional tagged `-test` govulncheck for the three suites | 0 | After go-archive correction; initial tagged scan exit 1 (`go run`, tool exit 3) retained as failure evidence |
| `task check` after security floor | 0 | Full build/vet/race/static/vuln/tidy/verify/hooks |
| V9 cached offline `task lint` | 0 | Zero issues |
| V10 pre-migration `task release:check`, clean | 201 / underlying 1 | Approved existing v2 API breaks, not a passing compatibility gate |
| V10 final default `task release:check`, clean | 201 / underlying 1 | Valid explicit cross-major comparison; cannot suggest across module paths |
| V10 final `task release:check RELEASE_VERSION=v2.0.0`, clean | 0 | Explicit v1.0.1 module baseline, v2.0.0 valid |
| V10 final `task release:check BASE=none RELEASE_VERSION=v2.0.0`, clean | 0 | Independent first-v2 validation, not a compatibility comparison |
| V11 `task precommit:run` | 0 | Full hooks; every implementation commit also ran its hooks |
| `task test:integration`, actual local containers, race | 0 | All three suites pass, including final patched graph |
| Integration `-count=2 -shuffle=on`, race | 0 | All three suites pass twice |
| `go test -tags integration -short ./...` | 0 | Compiles tagged code without creating containers |
| Tagged-short with deliberately nonexistent Docker socket | 0 | Each named integration suite explicitly SKIP before Docker access |
| Normal `go test -short ./...` | 0 | No Testcontainers package in the normal test dependency closure |
| `go vet -tags integration ./...`, tagged staticcheck | 0 / 0 | Includes integration helper/tests |
| `GOTOOLCHAIN=go1.26.10 go test -short ./...` | 0 | Supported lower-line toolchain verified |
| `GOTOOLCHAIN=go1.26.10 go test -tags integration -short ./...` | 0 | Repeated after go-archive floor; no accidental newer Go API |
| Root README and AGENTS complete examples, build and vet | 0 each | Compiled only, never executed |

The 11 full-scan Semgrep warnings are unchanged categories: the HTTP test TLS
fixture, non-cryptographic retry jitter, and documented messaging compatibility/
lifecycle background contexts. No new warning is introduced by the suites.
An initial staged hook failed because the integration-only helper had no default
build files; its package GoDoc fixes package discoverability. An initial tagged
staticcheck failed on Testcontainers' deprecated reaper constant; an explicit
pinned test constant fixes it. Auto-format hook changes were restaged and hooks
rerun. All final checks pass with the documented V10 default exit exception.

Machine-readable command exits and selected command logs are under
[`R-DEPS/`](./R-DEPS/). `checks.jsonl` retains superseded attempts; use the table
and final-prefixed/corrected checks above for the final outcome. Coverage data,
compiled examples, downloaded source bodies and complete original logs remain
under `/tmp/DEPS-validation`; coverage/binaries are not committed. Committed
log whitespace and terminal newlines are normalized by the repository hooks.

## Sources, limits and pending work

Read HANDOFF, audit rules/gates/ledger, approved plan items/G1, remediator role,
AGENTS, SECURITY, package contracts and errorkit registration/telemetry contracts.
Applied project go-testing-strategy, go-design-principles and go-code-quality-check;
read pinned Eduardo router, dependency-audit, test-quality, table-driven, context,
database and build-release skills/references. Source revision:
`e25b517bbd0c11db35f75a83a14f18848c1abdb0`. JetBrains source revision:
`155dc7ca10da5e1f6c841503086957b1b37f5815`. Read the complete Go 1.26 guideline
list and errors_as_type explanation; migration CLI output for each changed Go
file matched that fully read list. Canonical errorkit.As is retained for its
typed-nil and joined-error semantics.

No assigned item is silently deferred. Crypto's absent tagged fix is an explicit
approved decision, not a claimed patch. Integration covers one pinned local
server version per service, not every deployment configuration or live service.
Independent R-DEPS review and P5 remain the orchestrator's next steps.
