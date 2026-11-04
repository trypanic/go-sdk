# R-DEPS independent review — loop 1

**Verdict: CHANGES_REQUESTED.** Two reproducible regressions require correction:
local infrastructure listeners are exposed on wildcard host addresses, and the
new toolchain directive makes the CI Go 1.26 matrix silently execute Go 1.27.
The dependency selection, `/v2` migration, adapter behavior tests and XP probes
otherwise pass independent verification.

Reviewed head: `ac8ab8e93c0eeef029da27d533593699db09c1bc` on
`audit/2026-11-DEPS`; base: `5e57bba`; implementation: `1f40242`.
Reviewer: independent Codex / GPT-6.1 Astra, 2026-11-04; not the author.
The assigned worktree remained clean. No commits, ledger edits, dependency edits,
pushes, tags or releases were made. Mutations ran in exported temporary copies.
The author report in `validation/R-DEPS.md` was preserved.

## 1. HIGH — constrain every integration listener, including Ryuk, to loopback

**Locations:** `internal/integrationtest/container.go:37`, `:46`, `:49`;
`postgres/postgres_integration_test.go:29`, `:30`;
`mongo/mongo_integration_test.go:29`; `messaging/messaging_integration_test.go:27`.

The helper verifies that the client's selected endpoint is localhost, but it
never configures the Docker host port bindings. Testcontainers v0.44.0 fills
missing bindings with an ephemeral port and no HostIP. Docker consequently
publishes them on every host interface. The `container.Host()` check still
passes. Independently inspecting actual running fixtures produced:

| Owned listener | Actual host bindings |
| --- | --- |
| PostgreSQL `5432/tcp` | `0.0.0.0:32809`, `[::]:32809` |
| MongoDB `27017/tcp` | `0.0.0.0:32810`, `[::]:32810` |
| RabbitMQ `5672/tcp` | `0.0.0.0:32808`, `[::]:32808` |
| Ryuk `8080/tcp` | `0.0.0.0:32811`, `[::]:32811` |

PostgreSQL uses `POSTGRES_HOST_AUTH_METHOD=trust`, MongoDB has no authentication,
and RabbitMQ uses a public fixed test password. Ryuk additionally mounts
`/var/run/docker.sock:/var/run/docker.sock`; its `Privileged` value was false.
This exposes test services and the cleanup service to networks reachable through
the host's interfaces while the suite runs. No remote exploitation or external
network traffic was attempted, and no production SDK path is affected.
[Docker documents these wildcard publishing semantics](https://docs.docker.com/engine/network/port-publishing/).

**Evidence:** [service inspection patch](./R-DEPS-loop1/inspect-bindings.patch.b64),
[actual service bindings](./R-DEPS-loop1/bindings.log),
[Ryuk inspection](./R-DEPS-loop1/ryuk-bindings.json),
[Ryuk command/output](./R-DEPS-loop1/ryuk-bindings.log).
The extra inspection only reads container metadata; it does not change network
configuration. A separate [runtime assertion patch](./R-DEPS-loop1/loopback-assertion.patch.b64)
fails on the original implementation with
`published 5432/tcp on 0.0.0.0:32814`, exit 1, then terminates its container:
[runtime failure](./R-DEPS-loop1/loopback-assertion-runtime.log).
The earlier assertion draft had a compile error (`HostIP` is `netip.Addr`);
that superseded attempt is retained separately and is not the failure proof.

**Required correction:** set explicit loopback HostIP bindings before container
creation/start, preserve ephemeral port allocation, and inspect actual published
bindings in regression assertions. Apply equivalent isolation to the built-in
Ryuk listener: changing only the three service requests leaves the reproduced
socket-mounted cleanup listener exposed. Retain pinned images, bounded cleanup
and crash cleanup; disabling Ryuk is not an isolation fix. Use a scoped test
setup rather than changing a developer's shared Docker daemon configuration.
Verify all owned listeners with real Docker inspection and repeat the race suites.

## 2. MEDIUM — preserve the toolchain selected by each CI matrix job

**Locations:** new `go.mod:5`; `.github/workflows/ci.yml:72`, `:77`, `:83`,
`:132`, `:137`.

The new `toolchain go1.27.3` directive is approved and should stay. However,
`setup-go` installing `1.26.x` does not prevent Go's default automatic toolchain
selection. The workflow never fixes `GOTOOLCHAIN`. Consequently its build/race
job labeled Go 1.26 and its vulnerability job labeled Go 1.26 switch to 1.27.3.
This removes the supported lower-line build coverage and scans the wrong
standard library for that vulnerability matrix entry.

Using the actual installed Go 1.26.10 executable on the reviewed tree:

- `GOTOOLCHAIN=auto <go1.26.10>/bin/go version` exits 0 and reports
  `go version go1.27.3 linux/amd64`:
  [automatic selection](./R-DEPS-loop1/ci-floor-auto-authorized.log).
- `GOTOOLCHAIN=local <go1.26.10>/bin/go version` exits 0 and reports
  `go version go1.26.10 linux/amd64`:
  [fixed selection](./R-DEPS-loop1/ci-floor-fixed.log).
- With Go 1.26.10 first on PATH and `GOTOOLCHAIN=auto`, the exact pinned
  govulncheck command exits 0 but explicitly reports scanning the
  **go1.27.3 standard library**:
  [scan evidence](./R-DEPS-loop1/ci-floor-vuln-auto.log),
  [reproducer](./R-DEPS-loop1/ci-floor-scan.py).

This is the documented behavior of the
[Go toolchain selector](https://go.dev/doc/toolchain).
Independent fixed-toolchain short and tagged-short runs on Go 1.26.10 do pass;
there is no demonstrated present-day source incompatibility. The defect is the
loss of the continuing matrix guarantee and lower-line vulnerability scan.

**Required correction:** set `GOTOOLCHAIN=local` for both matrix jobs (or an
exact selected version without an automatic suffix), and log/assert
`go env GOVERSION` against the selected matrix line before build/test and scan.
Repeat both supported toolchains with the intended version fixed. Keep
`go 1.26.1` and the approved development `toolchain go1.27.3` directives.

## 3. Independent validation

All commands below ran against the exact reviewed tree, except explicitly
identified temporary-copy checks. Complete commands, real exit codes and timing
are in [checks.jsonl](./R-DEPS-loop1/checks.jsonl). Logs are independent of the
author's evidence.

| Check | Exit / result |
| --- | --- |
| V1 `gofmt -l .` | 0; no output |
| V2 `go build ./...` | 0; independently repeated inside `task check` |
| V3 `go vet ./...` | 0 inside independent `task check` |
| V4 `task test` | 0; all 21 top-level groups selected correctly, including internal packages |
| V5 `go test -race -shuffle=on -short ./...` | 0 inside independent `task check` |
| V6 `go test -short -count=1 -coverprofile=/tmp/DEPS-review1/coverage.out ./...` | 0; `go tool cover -func` total 87.5% |
| V7 `task staticanalysis` | 0 inside `task check`; vet/staticcheck clean; 11 Semgrep warnings, 0 errors |
| V8 `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` | 0 inside `task check`; zero reachable/imported-package vulnerabilities |
| Additional `govulncheck@v1.9.0 -tags=integration -test ./...` | 0; zero reachable/imported-package vulnerabilities |
| `task check` | 0; includes build, vet, shuffled race tests, static analysis, vulnerability scan, tidy-diff, verify and hooks |
| V9 cached offline `task lint` | 0; 0 issues |
| V10 `task release:check RELEASE_VERSION=v2.0.0` | 0; explicit `github.com/trypanic/go-sdk@v1.0.1` baseline, approved breaks reported, v2 valid |
| V10 `task release:check BASE=none RELEASE_VERSION=v2.0.0` | 0; log explicitly shows `-base=none`, v2 valid |
| CI's pinned gorelease `-base=v1.0.1` without proposed version | 1; it does generate the cross-major API report, then cannot suggest a version across module paths; not a new finding |
| V11 `task precommit:run` in independent clean clone of reviewed SHA | 0; complete hooks including short tests and full static analysis; clone remained clean |
| Real `go test -v -race -tags integration -run Integration -count=2 -shuffle=on -timeout=5m ./postgres ./mongo ./messaging` | 0; all three real suites ran twice, no infrastructure skips |
| `go vet -tags integration ./...`; `staticcheck -tags integration ./...` | 0 / 0 |
| `GOTOOLCHAIN=go1.26.10 go test -count=1 -short ./...` | 0 |
| `GOTOOLCHAIN=go1.26.10 go test -count=1 -tags integration -short ./...` | 0 |
| Tagged-short Integration tests with nonexistent `DOCKER_HOST` | 0; each suite explicitly skips before Docker access, as intended |
| README and AGENTS complete main examples: `go build`, `go vet` | 0 each; compiled only, never executed |
| Final Docker `ps -a --filter label=org.testcontainers=true` | 0; no remaining containers |
| Assigned-tree final `git status --porcelain` | empty; unchanged `ac8ab8e` |

[Full gates](./R-DEPS-loop1/gates-authorized.log),
[real repeated integrations](./R-DEPS-loop1/integration-authorized.log),
[tagged vulnerability scan](./R-DEPS-loop1/tagged-vuln.log),
[initial v2 release check](./R-DEPS-loop1/release-initial.log),
[cross-major comparison](./R-DEPS-loop1/release-comparison-authorized.log),
[complete hooks](./R-DEPS-loop1/hooks-isolated.log).

The Semgrep warnings are existing categories: TLS test fixture, non-security
retry randomness, and documented messaging background lifecycle/compatibility
contexts. No new integration file receives a warning.

Initial sandboxed attempts failed on Docker/loopback permissions, network access
or Go-cache writes. Those failures are retained and are not passing results;
explicitly authorized reruns passed. Automatic approval review rejected the
standalone hooks on the assigned read-only worktree because hooks can rewrite
files. The safe alternative was a separate disposable Git clone; complete hooks
passed there. No approval rejection remains blocking.

`git diff --check 5e57bba..HEAD` reports space-before-tab indentation in the
preserved historical `validation/R-DEPS/integration-first.log`; this is diagnostic
log formatting, not a source defect. `gofmt` and repository hooks pass.

## 4. Test strength and scope checks

The permanent XP tests pass five times with `-race` on HEAD. In isolated copies:

| Mutation | Required committed test | Observed result |
| --- | --- | --- |
| Remove draining of the JSON suffix from `limitedBody.overLimit` | `TestChunkedJSONTrailingLimit` | exit 1, expected oversize error becomes nil |
| Derive `PubSub.life` from the construction context | `TestSubscribeOutlivesConstructionContext` | exit 1, unexpected `ERR_MQ_CLOSED` |
| Set both production AMQP `PublishDeferred` mandatory flags to false | `TestRabbitMQIntegration/mandatory_returns_are_failures` | exit 1, expected publish error becomes nil |

[Portable patches and runner](./R-DEPS-loop1/reproduce.sh),
[chunked failure](./R-DEPS-loop1/chunked.log),
[lifetime failure](./R-DEPS-loop1/setup-lifetime.log),
[mandatory failure](./R-DEPS-loop1/mandatory-disabled.log).
The mandatory fixture is valid: `Publisher.open` declares every exchange even
without `WithDeclareTopology`. The initial suspicion of an undeclared-exchange
false positive was explicitly disproved and is not a finding.

All 208 changed existing Go files are byte-for-byte identical to the base after
only replacing the SDK module-path prefix; no existing test assertion was
removed or weakened. Seven new Go files contain the helper/GoDoc, three real
adapter suites and two XP probes. The suites exercise real SDK operations,
results, error classifications, cancellation, cursor getMore and ownership,
confirmed publishing/mandatory returns, and active-subscription shutdown.
Cleanup uses fresh bounded contexts where required. The author's initial fixture
failures remain recorded; changing the Rabbit queue to durable and preventing
double-disconnect are correct fixture repairs.

## 5. Dependency, migration and source verification

Independent `go list -m all` comparisons and full `go mod graph` confirm
**45 new selected graph nodes, 35 new explicit requirements, zero removed
external modules and zero version changes to previously selected modules**.
Every new node has a recorded parent dependency. The additional explicit
httpsnoop/otelhttp requirements retain their previously selected versions.
[Complete delta](./R-DEPS-loop1/dependency-delta.json).
`go mod tidy -diff` and `go mod verify` pass. No replace/exclude directive,
module split, unrelated production upgrade or manual release-file change occurs.

The [Testcontainers v0.44.0 release](https://github.com/testcontainers/testcontainers-go/releases/tag/v0.44.0)
and official manifest confirm the approved test dependency's Go 1.25 requirement
and reaper/security fixes; the MIT license is compatible. The
[go-archive v0.3.0 release](https://github.com/moby/go-archive/releases/tag/v0.3.0)
and [official GO-2026-6253 advisory](https://vuln.go.dev/ID/GO-2026-6253.json)
confirm the required security floor. Its manifest requires sys/user v0.4.1;
that patch's official release also addresses bounded user/group parsing. Both
archive v0.2.1/v0.3.0 release notes and sys/user v0.4.1 notes were read.
Official source snapshots and URLs are under [sources](./R-DEPS-loop1/sources/).

The official crypto tag inventory still ends at v0.58.0; GO-2026-5932's fix is
only `v0.58.1-0.20261009181718-2f1f834330b2`. Leaving the existing tag is the
approved decision. Neither the normal nor integration test import closure
contains OpenPGP. Both scans report this only as one module-level advisory.

All 31 SDK package paths and the complete integration-test import closure use
`/v2`; normal tests do not link Testcontainers. The stack filter and ioutils
constant migrated. Repository URLs and clearly historical import examples remain
correct. Task package selection strips the actual module path. Both explicit
release checks use the intended baseline semantics. Release proxy warming uses
the declared module. AGENTS retains its ongoing-audit notice. The scoped XP
README errata match actual literals, links and boolean-default behavior.

Read the work order, handoff, Step D, approved plan/G1, reviewer role, AGENTS,
SECURITY, target contracts and errorkit registration/telemetry contracts. Applied
the three required project skills and pinned Eduardo router, dependency audit,
code review, test quality, context and database skills/references at
`e25b517bbd0c11db35f75a83a14f18848c1abdb0`. JetBrains source was
`155dc7ca10da5e1f6c841503086957b1b37f5815`: CLI ran for all 215 changed Go
files, producing one identical fully read Go 1.26 guideline list; the
`errors_as_type` explanation was also read. Canonical errorkit matching and
fresh shutdown contexts remain appropriate project-specific exceptions.

Remaining review scope is the two numbered corrections above. Testing covered
one pinned local server version per service, not remote infrastructure or every
deployment configuration. This verdict does not authorize merge or P5.

## 6. Repository evidence packaging

The orchestrator stores extracted examples as `.go.txt` so documentation does
not create an unintended Go package. Exact patch bytes are base64 encoded in
`.patch.b64`; `reproduce.sh` decodes them before applying them. Other text logs
are normalized for repository whitespace/newline hooks. The original captured
files remain under `/tmp/DEPS-review1/R-DEPS-loop1`; the source manifest is
preserved inside `manifest.json` alongside checksums of repository artifacts.
This packaging change does not alter any reviewed SDK implementation.
