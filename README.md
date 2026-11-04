# go-sdk

[![Go Reference](https://pkg.go.dev/badge/github.com/trypanic/go-sdk.svg)](https://pkg.go.dev/github.com/trypanic/go-sdk)

Opinionated Go SDK for distributed systems — bundles logging, errors, tracing,
metrics, HTTP, messaging, and storage primitives behind a small, explicit
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
		appErr, ok := err.(*errorkit.AppError)
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

Import the directory; refer to symbols by the package identifier. Two
directories declare a different identifier.

| Directory (import path suffix) | Package identifier | Purpose |
|--------------------------------|--------------------|---------|
| [`algorithms`](./algorithms)   | `algorithms`       | exponential backoff factory |
| [`envs`](./envs)               | `envs`             | caarlos0/env wrapper |
| [`errorkit`](./errorkit)       | `errorkit`         | typed AppError + registry |
| [`grpc`](./grpc)               | `grpc`             | gRPC server + client factories (Kitex-backed); alias it (e.g. `sdkgrpc`) next to `google.golang.org/grpc` |
| [`httpclient`](./httpclient)   | `httpclient`       | net/http.Client factory |
| [`httprequest`](./httprequest) | `httprequest`      | retrying HTTP requester |
| [`httpserver`](./httpserver)   | `httpserver`       | framework-agnostic HTTPServer contracts |
| [`httpserver/hertz`](./httpserver/hertz) | `hertz`  | Hertz adapter (only nested package) |
| [`ioutils`](./ioutils)         | `ioutils`          | dev-only JSON dump helpers |
| [`llmclient`](./llmclient)     | `llmclient`        | OpenAI-compatible chat client (non-streaming) |
| [`logger`](./logger)           | `logger`           | zerolog wrapper + OTLP log provider |
| [`marshal`](./marshal)         | `marshal`          | JSON encoding helpers |
| [`messaging`](./messaging)     | `messaging`        | RabbitMQ pub/sub + topology loader |
| [`mongo`](./mongo)             | **`mongodb`**      | mongo-driver wrapper + tracing |
| [`postgres`](./postgres)       | **`database`**     | pgx pool + StoredProcedure[T] helper |
| [`slices`](./slices)           | `slices`           | generic slice helpers |
| [`storage`](./storage)         | `storage`          | KV + append-log generic store |
| [`stringutils`](./stringutils) | `stringutils`      | normalize + markdown strip |
| [`telemetry`](./telemetry)     | `telemetry`        | OTel tracer wrapper + Instrumenter |
| [`urlkit`](./urlkit)           | `urlkit`           | URL build + path join |
| [`validators`](./validators)   | `validators`       | input validators |

```go
import "github.com/trypanic/go-sdk/postgres" // package database

pool, err := database.NewPostgresClientFromEnv(database.Config{})
```

## Package status

| Package                                          | Status                                |
|--------------------------------------------------|---------------------------------------|
| `errorkit`                                       | Stable                                |
| `telemetry`, `logger`                            | Stable; opt-in tracing/logging        |
| `httpclient`, `httprequest`                      | Stable; concurrent-safe; opt-in tracing |
| `httpserver`                                     | Contracts stable; only Hertz adapter shipped |
| `httpserver/hertz`                               | Adapter; pulls Hertz transitively     |
| `grpc`                                           | Stable; gRPC server + client (Kitex-backed); pulls Kitex transitively |
| `postgres`, `mongo`, `storage`                   | Stable                                |
| `messaging`                                      | Stable; topology + transport flat     |
| `envs`                                           | Stable                                |
| `marshal`, `stringutils`, `urlkit`, `validators` | Stable                                |
| `algorithms`                                     | Stable                                |
| `llmclient`                                      | Stable; non-streaming only            |
| `slices`                                         | Stable                                |
| `ioutils`                                        | **Dev-only** — do not use in production paths |

## Cross-cutting concern policy

- **Tracing.** Every package that produces spans accepts a
  `*telemetry.Instrumenter` at construction. Default constructors install a
  wrapper that uses the global OTel tracer for source compatibility;
  `*WithoutTracing` and `*WithInstrumenter` variants are explicit.
- **Logging.** No package mutates the global logger. Adapters that need a
  per-request logger accept an explicit `*logger.Logger`.
- **Errors.** Every package that returns an error returns
  `*errorkit.AppError` with a documented `ErrorCode`.
- **Configuration.** No package reads environment variables silently. Where
  an env var is conventional (`POSTGRES_DSN`, `RABBITMQ_TOPOLOGY_FILE`,
  `LLM_API_KEY`), a sibling `*FromEnv` constructor reads it explicitly.
- **Body capture.** HTTP request and inbound audit middleware redact bodies
  by default; raw capture is an explicit opt-in.

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
task tools:install        # goimports, pre-commit and semgrep on top
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
task check                 # all of the above: the same commands as CI
task lint                  # golangci-lint, default linters (informational, not in check)
```

`task check` covers every blocking CI job except the toolchain matrix: it runs
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
2. The release PR is merged only when [`ci`](./.github/workflows/ci.yml) is
   green on it: build, vet, race tests, static analysis, govulncheck and
   module checks gate the release, before any tag exists.
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
