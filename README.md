# go-sdk

Opinionated Go SDK for distributed systems — bundles logging, errors, tracing,
metrics, HTTP, messaging, and storage primitives behind a small, explicit
API surface.

Module path: `github.com/trypanic/go-sdk` (Go `1.26.1`).

## Package map

```text
go-sdk/
├── algorithms/        exponential backoff factory
├── envs/              caarlos0/env wrapper
├── errorkit/          typed AppError + registry
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

## Developer setup

System packages required for `semgrep` (one-time, host install):

```bash
sudo apt install build-essential python3-dev clang
python -m pip install --upgrade pip
pipx install semgrep==1.95.0     # or: pip install semgrep
pipx install pre-commit          # or: pip install pre-commit
go install golang.org/x/tools/cmd/goimports@latest
go install honnef.co/go/tools/cmd/staticcheck@latest
```

Install git hooks:

```bash
moon run repo:precommit-install
```

## Quality gates

The full vet + staticcheck + semgrep stack runs through a single script,
`staticanalysis.sh`, wired into both pre-commit (staged-files mode) and the
`moon` task (full-repo mode).

```bash
moon run repo:staticanalysis        # full repo (vet + staticcheck + semgrep)
moon run repo:precommit-run         # all pre-commit hooks across the tree
go test -short ./...                # unit tests, no external deps
```

Per-tool reports and grouped fix-plans land in `.staticanalysis/`. See
[`SECURITY.md`](./SECURITY.md) for the full runner contract, env knobs, CI
workflow, and rule-promotion process.

## Changelog

See [`CHANGELOG.md`](./CHANGELOG.md) for the full per-phase change log.

## Common issues

When Python is installed via `proto`, some Python binaries are not on `PATH`.
Add the proto-managed Python `bin/` directory:

```bash
echo 'export PATH="$(dirname $(proto bin python))/../bin:$PATH"' >> ~/.zshrc
```
