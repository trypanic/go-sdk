# go-sdk

[![Go Reference](https://pkg.go.dev/badge/github.com/trypanic/go-sdk.svg)](https://pkg.go.dev/github.com/trypanic/go-sdk)

Opinionated Go SDK for distributed systems — bundles logging, errors, tracing,
HTTP, messaging, and storage primitives behind a small, explicit
API surface.

Module path: `github.com/trypanic/go-sdk` (language floor `go 1.26.1`; see
[Compatibility](#compatibility)). Licensed under the [MIT License](./LICENSE).

## Install

```bash
go get github.com/trypanic/go-sdk@latest
```

The SDK is one Go module: `go get` adds every package at the same version,
and a binary links only the packages it imports.

## Quick start

Structured logging and a classified error, wrapped once at the boundary:

```go
package main

import (
	"context"
	"errors"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/logger"
)

func main() {
	_, cleanup := logger.Init("my-service", "0.1.0")
	defer cleanup.Close()

	ctx := context.Background()
	if err := fetchOrder(ctx); err != nil {
		// Wrap once at the boundary; keep an existing *errorkit.AppError as is.
		appErr, ok := errorkit.As(err)
		if !ok {
			appErr = errorkit.NewError(errorkit.ERR_INTERNAL).With(errorkit.WithWrapped(err))
		}
		logger.ErrorCtx(ctx, appErr, "fetch order failed")
		return
	}
	logger.LogInfoWithTrace(ctx, "order fetched")
}

func fetchOrder(context.Context) error { return errors.New("upstream unavailable") }
```

Each package README has its own API and examples; the
[AGENTS.md bootstrap](./AGENTS.md#3-canonical-service-bootstrap) wires
logging, tracing, outbound HTTP, Postgres and RabbitMQ together.

## Package map

Public package identifiers match their directories. See the
[v2 API migration](./docs/v2-api-migration.md) for approved renames and removals.
The `/v2` module-path change is applied in the audit's subsequent dependency phase.

| Directory (import path suffix) | Package identifier | Purpose |
|--------------------------------|--------------------|---------|
| [`algorithms`](./algorithms)   | `algorithms`       | exponential backoff factory |
| [`envs`](./envs)               | `envs`             | caarlos0/env wrapper |
| [`errorkit`](./errorkit)       | `errorkit`         | typed AppError + registry |
| [`grpc`](./grpc)               | `grpc`             | gRPC server + client factories on google.golang.org/grpc; alias it (e.g. `sdkgrpc`) next to `google.golang.org/grpc` |
| [`httpclient`](./httpclient)   | `httpclient`       | net/http.Client factory |
| [`httprequest`](./httprequest) | `httprequest`      | retrying HTTP requester |
| [`httpserver`](./httpserver)   | `httpserver`       | framework-agnostic Server/Context contracts |
| [`httpserver/hertz`](./httpserver/hertz) | `hertz`  | Hertz adapter and Engine escape hatch |
| [`ioutils`](./ioutils)         | `ioutils`          | dev-only JSON dump helpers |
| [`llmclient`](./llmclient)     | `llmclient`        | OpenAI-compatible chat client (non-streaming) |
| [`logger/otlp`](./logger/otlp) | `otlp`             | explicit OTLP export and provider lifecycle |
| [`logger`](./logger)           | `logger`           | zerolog wrapper and trace correlation |
| [`marshal`](./marshal)         | `marshal`          | JSON encoding helpers |
| [`messaging`](./messaging)     | `messaging`        | RabbitMQ pub/sub + topology loader |
| [`mongo`](./mongo)             | `mongo`      | mongo-driver wrapper + tracing |
| [`postgres`](./postgres)       | `postgres`     | pgx pool + StoredProcedure[T] helper |
| [`batch`](./batch)             | `batch`            | bounded chunks and stable deduplication |
| [`storage`](./storage)         | `storage`          | KV + append-log generic store |
| [`stringutils`](./stringutils) | `stringutils`      | normalize + markdown strip |
| [`telemetry`](./telemetry)     | `telemetry`        | OTel tracer wrapper + Instrumenter |
| [`urlkit`](./urlkit)           | `urlkit`           | URL build + path join |
| [`validators`](./validators)   | `validators`       | input validators |

```go
import "github.com/trypanic/go-sdk/postgres"

pool, err := postgres.NewFromEnv(postgres.Config{})
```

## Package status

| Package                                          | Status                                |
|--------------------------------------------------|---------------------------------------|
| `errorkit`                                       | Stable                                |
| `telemetry`, `logger`                            | Stable; explicit logging/provider configuration        |
| `httpclient`, `httprequest`                      | Stable; concurrent-safe; requester tracing enabled by default |
| `httpserver`                                     | Contracts stable; only Hertz adapter shipped |
| `httpserver/hertz`                               | Adapter; pulls Hertz transitively     |
| `grpc`                                           | Stable; wraps google.golang.org/grpc; tracing and recovery on by default |
| `postgres`, `mongo`, `storage`                   | Stable                                |
| `messaging`                                      | Stable; topology + transport flat     |
| `envs`                                           | Stable                                |
| `marshal`, `stringutils`, `urlkit`, `validators` | Stable                                |
| `algorithms`                                     | Stable                                |
| `llmclient`                                      | Stable; non-streaming only            |
| `batch`                                         | Stable                                |
| `ioutils`                                        | **Dev-only** — do not use in production paths |

## Cross-cutting concern policy

- **Tracing.** HTTP, LLM and Mongo default constructors integrate client spans;
  explicit instrumenter and no-tracing constructors are available. PostgreSQL
  traces at the stored-procedure layer. Messaging and gRPC accept an instrumenter
  or their existing tracer/provider options. Hertz middleware uses the global
  provider. Failed client spans record sanitized classification and status.
- **Logging.** `logger.Init` configures the global logger; explicit `logger.New`
  instances avoid global mutation. Context loggers provide trace correlation.
  `logger/otlp` owns optional export; base logger/request packages do not link
  gRPC through logging. `DetectEnv` defaults to production JSON output.
- **Errors.** SDK-classified operations return `*errorkit.AppError` through `error`.
  Upstream clients/escape hatches, generated gRPC clients, callbacks and documented
  lifecycle methods can return other errors. Inspect chains with `errorkit.As` /
  `HasCode`; use `PublicJSON` or `httpserver.ReplyError` for responses, not diagnostic
  `PrettyJSON`. Expected validator failures omit stacks; configuration errors retain them.
- **Configuration.** Explicit environment APIs are `postgres.NewFromEnv`,
  `messaging.NewPubSubFromEnv`, `llmclient.ConfigFromEnv`, and `envs.Load[T]` /
  `LoadInto`. Context variants bound initial PostgreSQL/messaging setup without
  owning the resulting client's lifetime. Logger environment selection is documented.
- **Retries.** Retriable metadata alone does not authorize replay of side effects.
  HTTP also requires an idempotent request/key or a safe initial dial failure,
  unless explicitly opted into non-idempotent retries. HTTP/messaging share
  bounded full jitter; Retry-After and context bounds remain authoritative.
- **Body capture.** HTTP bodies are omitted or redacted by default; raw capture is
  a development-only opt-in. HTTP paths/models are span attributes, not operation names.
- **Ownership.** Share clients and close owned resources at process scope.
  `mongo.NewFromClient` borrows its driver client. `Engine`, `Database` and
  `Unwrap` expose documented low-level access without transferring ownership.

Consumer fakes are provided by
[`httpserver/httpservertest`](./httpserver/httpservertest/README.md),
[`mongo/mongotest`](./mongo/mongotest/README.md), and
[`messaging/messagingtest`](./messaging/messagingtest/README.md).
The SDK provides no public metrics API; applications configure metrics separately.

## Compatibility

### Versioning

The module follows [semantic versioning](https://semver.org) for its whole
exported API: every package shares the module version (`vX.Y.Z` tags on the
repository root). Within a major version, minor and patch releases do not
remove or change exported identifiers; a breaking change ships only in a new
major version with a new module path (`github.com/trypanic/go-sdk/v2`, …), as
Go requires. `task release:check` (gorelease) checks this before each
release. Packages marked **Dev-only** above (`ioutils`) are outside the
production contract. Security fixes and supported versions:
[`SECURITY.md`](./SECURITY.md#supported-versions).

### Supported toolchains

Build and test services that use the SDK with the **latest patch release of
Go 1.26 (1.26.10 or newer) or of Go 1.27**. CI tests both lines
(`1.26.x` and `1.27.x`, latest patch).

The `go 1.26.1` directive in `go.mod` is a *language* floor: it is the oldest
toolchain that compiles the module, not a safe one. Go 1.26.1–1.26.9 contain
standard-library vulnerabilities (`net/http`, `crypto/tls`, `crypto/x509`,
…) that ordinary SDK use reaches through `httprequest`, `messaging`,
`postgres`, `mongo`, `httpserver/hertz` and `logger`; they are fixed in
1.26.10 and in 1.27. Toolchain patching belongs to the consumer, so the SDK
does not raise its `go` directive to force it. Contributors can run
`task tools:check` to confirm their toolchain.

## Developer setup

Tool versions are pinned in [`.prototools`](./.prototools) and installed by
[proto](https://moonrepo.dev/proto): Go, Python, [Task](https://taskfile.dev),
staticcheck and skillshare. Third-party tools resolve through the
plugins in [`trypanic/proto-plugins`](https://github.com/trypanic/proto-plugins).

System packages required for `semgrep` (one-time, host install):

```bash
sudo apt install build-essential python3-dev clang
```

Then, from the repository root:

```bash
bash <(curl -fsSL https://moonrepo.dev/install/proto.sh)   # once per machine
proto trust .prototools   # allow the pinned plugins, env and auto-install
proto install             # go, python, task, staticcheck, skillshare
task tools:install        # pinned goimports, pre-commit, semgrep and git-cliff on top
```

Install git hooks:

```bash
task precommit:install
```

## Quality gates

The full vet + staticcheck + semgrep stack runs through a single script,
[`scripts/staticanalysis.sh`](./scripts/staticanalysis.sh), wired into both pre-commit (staged-files mode) and
[Task](https://taskfile.dev) (full-repo mode). Run `task --list` to see every
task defined in [`Taskfile.yml`](./Taskfile.yml).

```bash
task staticanalysis        # full repo (vet + staticcheck + semgrep)
task precommit:run         # all pre-commit hooks across the tree
task test                  # unit tests per package, in parallel, no external deps
task test:race             # go build + go vet + race-enabled, shuffled tests (CI "test")
task vuln                  # govulncheck, pinned (CI "vuln"; active toolchain only)
task tidy:check            # go mod tidy -diff + go mod verify (CI "tidy")
task check                 # the same commands as CI: test:race, staticanalysis, vuln, tidy:check, hooks
task lint                  # golangci-lint, default linters (informational, not in check)
```

`task check` runs each unit test once (in `test:race`; the hooks run with
`SKIP=go-test,staticanalysis-full` because those steps already ran) and
covers every blocking CI job except the toolchain matrix: it runs
with the active Go only (`task tools:check` confirms it is the pinned one).
`task release:check` (gorelease) stays separate because it needs a clean tree.

## Updating dependencies

Every task below takes an optional list of SDK packages after `--`; without it,
the task covers all packages (`task packages` lists them).

```bash
task deps:outdated                       # available updates, per package, in parallel, plus a whole-module pass
task deps:outdated -- logger mongo       # same, for selected packages
task deps:update -- postgres             # update one package's dependencies
task deps:update -- postgres mongo grpc  # update several
task deps:update                         # update all
UPGRADE=-u=patch task deps:update        # patch releases only
```

`deps:outdated` checks each package's build and test closure. Without
arguments it also lists every outdated, deprecated or retracted module of the
module graph, labelled `linked` (built into an SDK package or its tests),
`go.mod` (required in `go.mod` but not linked; consumers still inherit it as
a minimum version) or `graph-only` (never built).

All packages share one `go.mod`, so `deps:update` resolves the upgrade in a
single `go get` call (concurrent `go get` runs would race on `go.mod`), runs
`go mod tidy`, and then builds, vets and tests each selected package in
parallel.

Per-tool reports and grouped fix-plans land in `.staticanalysis/`. See
[`SECURITY.md`](./SECURITY.md#static-analysis-contributors) for the full
runner contract, env knobs, CI job, and rule-promotion process.

## Security

Report vulnerabilities privately through GitHub private vulnerability
reporting, never in a public issue. [`SECURITY.md`](./SECURITY.md) has the
reporting steps, the supported versions and the triage of known advisories.

## Releases

The SDK is a single Go module, so every package shares one version
(`vX.Y.Z` tags on the repository root).

Releases are automated by [release-please](https://github.com/googleapis/release-please)
from [Conventional Commits](https://www.conventionalcommits.org) (enforced by a
`commit-msg` hook):

1. Each push to `main` updates an open **release PR** that bumps the version
   (`feat` → minor, `fix` → patch, `!` or `BREAKING CHANGE:` → major) and
   prepends the new entry to [`CHANGELOG.md`](./CHANGELOG.md).
2. release-please runs with `GITHUB_TOKEN`, so its release PR does **not**
   trigger [`ci`](./.github/workflows/ci.yml) by itself. Before merging, the
   maintainer runs it on the release branch and merges only when it is green
   (build, vet, race tests, static analysis, govulncheck, module checks),
   before any tag exists:

   ```bash
   gh workflow run ci.yml --ref <release-please branch>   # or "Run workflow" in the Actions UI
   ```

   Recommended (owner action; not configured by this repository): protect
   `main` with the `ci` jobs as required status checks (test and govulncheck
   for both Go versions, staticanalysis, tidy; not the informational
   gorelease). Required checks match by check name on the PR head commit,
   whatever event started the run, so the manual dispatch above satisfies
   them on the release PR, and if it is not run the release PR stays blocked
   instead of merging without CI. Confirm this with a test PR before enabling
   the protection. Convenience only: a GitHub App or fine-grained PAT token
   for release-please (contents + pull-requests write on this repository)
   makes CI start on the release PR by itself.
3. Merging the release PR creates the tag and the GitHub Release.
4. The [`release`](./.github/workflows/release.yml) workflow then smoke-checks
   the tagged tree (`go mod verify`, `go mod tidy -diff`, build, vet, tests),
   attaches a CycloneDX SBOM, a source archive and `checksums.txt`, warms the
   Go module proxy, and records a signed build provenance attestation for
   each asset. It can be re-run for an existing tag with `workflow_dispatch`.

Verify a downloaded asset against the workflow that built it:

```bash
sha256sum -c checksums.txt --ignore-missing
gh attestation verify go-sdk-vX.Y.Z.src.tar.gz --repo trypanic/go-sdk
```

Before merging a release PR:

```bash
task release:check   # gorelease: API diff against the last tag + suggested version
task changelog       # git-cliff: preview of the unreleased changelog entry
```

Use `fix(deps): ...` for dependency updates; `chore` commits never trigger a
release.

## Changelog

Release notes are generated by release-please from Conventional Commits and
prepended to [`CHANGELOG.md`](./CHANGELOG.md), one entry per version, when a
release PR is merged. `build`, `chore`, `ci`, `style` and `test` commits are
not listed. The file starts with the first release made after v1.0.0; v1.0.0
itself has no entry.

## Contributing

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for the development setup, the
local gates, commit conventions and pull request checks.

## License

[MIT](./LICENSE) © 2026 trypanic.

## Common issues

When Python is installed via `proto`, some Python binaries are not on `PATH`.
Add the proto-managed Python `bin/` directory:

```bash
echo 'export PATH="$(dirname $(proto bin python))/../bin:$PATH"' >> ~/.zshrc
```
