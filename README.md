# go-sdk

Opinionated Go SDK for distributed systems — bundles logging, errors, tracing,
metrics, HTTP, messaging, and storage primitives behind a small, explicit
API surface.

Module path: `github.com/trypanic/go-sdk` (language floor `go 1.26.1`; see
[Supported toolchains](#supported-toolchains)).

## Package map

```text
go-sdk/
├── algorithms/        exponential backoff factory
├── envs/              caarlos0/env wrapper
├── errorkit/          typed AppError + registry
├── grpc/              gRPC server + client factories (Kitex-backed)
├── httpclient/        net/http.Client factory
├── httprequest/       retrying HTTP requester
├── httpserver/        framework-agnostic HTTPServer contracts
│   └── hertz/         Hertz adapter (only nested package)
├── ioutils/           dev-only JSON dump helpers
├── llmclient/         OpenAI-compatible chat client (non-streaming)
├── logger/            zerolog wrapper + OTLP log provider
├── marshal/           JSON encoding helpers
├── messaging/         RabbitMQ pub/sub + topology loader
├── mongo/             mongo-driver wrapper + tracing
├── postgres/          pgx pool + StoredProcedure[T] helper
├── slices/            generic slice helpers
├── storage/           KV + append-log generic store
├── stringutils/       normalize + markdown strip
├── telemetry/         OTel tracer wrapper + Instrumenter
├── urlkit/            URL build + path join
└── validators/        input validators
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

## Supported toolchains

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
staticcheck, skillshare and sentrux. Third-party tools resolve through the
plugins in [`trypanic/proto-plugins`](https://github.com/trypanic/proto-plugins).

System packages required for `semgrep` (one-time, host install):

```bash
sudo apt install build-essential python3-dev clang
```

Then, from the repository root:

```bash
bash <(curl -fsSL https://moonrepo.dev/install/proto.sh)   # once per machine
proto trust .prototools   # allow the pinned plugins, env and auto-install
proto install             # go, python, task, staticcheck, skillshare, sentrux
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
task check                 # all of the above
```

## Updating dependencies

Every task below takes an optional list of SDK packages after `--`; without it,
the task covers all packages (`task packages` lists them).

```bash
task deps:outdated                       # available updates, per package, in parallel
task deps:outdated -- logger mongo       # same, for selected packages
task deps:update -- postgres             # update one package's dependencies
task deps:update -- postgres mongo grpc  # update several
task deps:update                         # update all
UPGRADE=-u=patch task deps:update        # patch releases only
```

All packages share one `go.mod`, so `deps:update` resolves the upgrade in a
single `go get` call (concurrent `go get` runs would race on `go.mod`), runs
`go mod tidy`, and then builds, vets and tests each selected package in
parallel.

Per-tool reports and grouped fix-plans land in `.staticanalysis/`. See
[`SECURITY.md`](./SECURITY.md) for the full runner contract, env knobs, CI
workflow, and rule-promotion process.

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
   the tagged tree and attaches a CycloneDX SBOM, a source archive and
   `checksums.txt`, and warms the Go module proxy. It can be re-run for an
   existing tag with `workflow_dispatch`.

Before merging a release PR:

```bash
task release:check   # gorelease: API diff against the last tag + suggested version
task changelog       # git-cliff: preview of the unreleased changelog entry
```

Use `fix(deps): ...` for dependency updates; `chore` commits never trigger a
release.

## Changelog

See [`CHANGELOG.md`](./CHANGELOG.md) for the full per-phase change log.

## Common issues

When Python is installed via `proto`, some Python binaries are not on `PATH`.
Add the proto-managed Python `bin/` directory:

```bash
echo 'export PATH="$(dirname $(proto bin python))/../bin:$PATH"' >> ~/.zshrc
```
