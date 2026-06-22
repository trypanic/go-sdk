# AGENTS.md

Operational guide for AI coding agents (Claude, Codex, Cursor, Continue, Cline,
Qwen, DeepSeek, etc.) working in projects that **import** `github.com/trypanic/go-sdk`,
or working **inside this repository** to extend it.

This file is the agent-facing complement to [`README.md`](./README.md). It exists
to make package selection deterministic and idiomatic without re-reading every
sub-`README.md`. When this file and a sub-`README.md` disagree, the sub-`README.md`
wins — link out before guessing.

- Module path: `github.com/trypanic/go-sdk`
- Go version: `1.26.1` (see [`go.mod`](./go.mod))
- License / status / changelog: [`README.md`](./README.md), [`CHANGELOG.md`](./CHANGELOG.md), [`SECURITY.md`](./SECURITY.md)

---

## 0. Import path vs. package name (read first)

A few directories declare a `package` name that differs from the directory.
**Use the directory in the import path; reference symbols by the package name.**

| Import path                                | Package identifier |
|--------------------------------------------|--------------------|
| `github.com/trypanic/go-sdk/postgres`      | `database`         |
| `github.com/trypanic/go-sdk/mongo`         | `mongodb`          |
| All other packages listed in §1            | matches directory  |

```go
import "github.com/trypanic/go-sdk/postgres"   // import the directory
// ...
pool, err := database.NewPostgresClientFromEnv(database.Config{}) // call by package name
```

If the compiler complains that `postgres` or `mongo` is undefined, this is why.

The `grpc` package matches its directory, but its identifier collides with
upstream `google.golang.org/grpc`. When you import both, alias ours:
`import sdkgrpc "github.com/trypanic/go-sdk/grpc"`.

---

## 1. Package selection table

Pick by task, then read the linked sub-`README.md` for the exact API.

| You need to…                                          | Use                                                     | Sub-doc                                              |
|-------------------------------------------------------|---------------------------------------------------------|------------------------------------------------------|
| Return a structured, classified error                 | `errorkit`                                              | [errorkit](./errorkit/README.md)                     |
| Log structured events / errors with trace correlation | `logger`                                                | [logger](./logger/README.md)                         |
| Create OTel spans with consistent naming              | `telemetry`                                             | [telemetry](./telemetry/README.md)                   |
| Build an `*http.Client` (pool, timeouts, TLS)         | `httpclient`                                            | [httpclient](./httpclient/README.md)                 |
| Make outbound HTTP with retries + structured errors   | `httprequest`                                           | [httprequest](./httprequest/README.md)               |
| Run an HTTP server (Hertz today)                      | `httpserver` + `httpserver/hertz`                       | [httpserver](./httpserver/README.md)                 |
| Run a gRPC server / client (4 call modes, tracing, recovery) | `grpc`                                           | [grpc](./grpc/README.md)                             |
| Talk to PostgreSQL (pgx pool, stored procs)           | `postgres`                                              | [postgres](./postgres/README.md)                     |
| Talk to MongoDB                                       | `mongo`                                                 | [mongo](./mongo/README.md)                           |
| Publish/consume RabbitMQ                              | `messaging`                                             | [messaging](./messaging/README.md)                   |
| Persist KV blobs / append-only logs                   | `storage`                                               | [storage](./storage/README.md)                       |
| Call OpenAI-compatible chat APIs (non-streaming)      | `llmclient`                                             | [llmclient](./llmclient/README.md)                   |
| Load typed config from environment                    | `envs`                                                  | [envs](./envs/README.md)                             |
| Build URLs / join paths safely                        | `urlkit`                                                | [urlkit](./urlkit/README.md)                         |
| Validate inputs (email, ULID, etc.)                   | `validators`                                            | [validators](./validators/README.md)                 |
| JSON encode/decode helpers                            | `marshal`                                               | [marshal](./marshal/README.md)                       |
| Slice helpers (chunks, dedupe)                        | `slices`                                                | [slices](./slices/README.md)                         |
| String normalization / markdown strip                 | `stringutils`                                           | [stringutils](./stringutils/README.md)               |
| Exponential backoff for custom loops                  | `algorithms`                                            | [algorithms](./algorithms/README.md)                 |
| **Dump JSON to disk for local debugging only**        | `ioutils`                                               | [ioutils](./ioutils/README.md) — **dev-only**        |

### Status snapshot

All packages are **stable** except `ioutils`, which is **dev-only**. Never call
`ioutils` from a production path. `httpserver` ships only the Hertz adapter
under `httpserver/hertz`; the contracts are framework-agnostic.

---

## 2. Universal conventions

These hold across every package. Memorize them — they explain the constructor
shapes you will see repeatedly.

### 2.1 Errors

- Every public function that can fail returns `error` whose dynamic type is
  `*errorkit.AppError`.
- Each error carries an `ErrorCode` (e.g. `ERR_DB_POSTGRES_TIMEOUT`), HTTP
  status, retriability, and a stack trace.
- **Wrap, do not re-encode.** If an SDK call already returned `*errorkit.AppError`,
  preserve it via `errorkit.WithWrapped(err)` rather than constructing a new
  generic `ERR_INTERNAL`.
- New error codes for downstream services: register at runtime in `init()` with
  `errorkit.MustRegister(...)`. Do **not** patch the SDK's `codes.go` from
  outside the SDK repo.
- Retryability is data-driven: `appErr.Metadata.Retriable`. Higher layers
  (`httprequest`) consult this directly — don't reimplement retry classification.

### 2.2 Tracing

- Most packages that produce spans accept a `*telemetry.Instrumenter` at
  construction. The common triplet:
  - **Default** (`New(...)`): installs an instrumenter backed by the global
    OTel tracer. Source-compatible: works without explicit OTel setup.
  - **`*WithoutTracing(...)`**: explicit no-op instrumenter.
  - **`*WithInstrumenter(...)`**: caller-provided `*telemetry.Instrumenter`.
- Packages following the triplet: `httprequest`, `llmclient`, `mongo`.
- **Exceptions to be aware of:**
  - `postgres` — pool construction (`NewPostgresClient` / `NewPostgresClientFromEnv`)
    has no tracing parameter. Tracing is added at the `StoredProcedure[T]` layer
    via `database.WrapWithInstrumenter[T](inner, instr)`.
  - `messaging` — accepts a plain `trace.Tracer` via `messaging.WithTracer(...)`
    option, not an `*Instrumenter`. Pass `otel.Tracer(serviceName)` if you
    want explicit control; otherwise omit and the global OTel tracer is used.
- Use `telemetry.Job`, `Batch`, `External`, `Messaging`, `DB` for span naming.
  Do not invent ad-hoc span name conventions.

### 2.3 Logging

- No SDK package mutates the global `zerolog` logger except `logger.Init`.
- Adapters and middleware that need a per-request logger accept an explicit
  `*logger.Logger` or read `logger.CtxOrGlobal(ctx)`.
- For service code, prefer `logger.ErrorCtx(ctx, appErr, "msg")` and
  `logger.LogInfoWithTrace(ctx, "msg")`. They attach `trace_id` / `span_id`
  from the active OTel span automatically.

### 2.4 Configuration

- No package reads environment variables silently.
- Where an env var is conventional (e.g. `POSTGRES_DSN`,
  `RABBITMQ_TOPOLOGY_FILE`, `LLM_API_KEY`), use the sibling `*FromEnv`
  constructor. Otherwise, populate the `Config` struct yourself.
- For typed config in your own service, use [`envs`](./envs/README.md) — a thin
  wrapper around `caarlos0/env`.

### 2.5 Body capture / redaction

- HTTP request middleware and inbound audit middleware **redact bodies by
  default**.
- Raw capture is opt-in via `WithRawBodies()`. Never enable in production.
- Custom policy: pass `WithBodyRedactor(func([]byte) []byte)`.

### 2.6 Concurrency

- All constructors return values that are safe for concurrent use after
  construction unless explicitly noted.
- Create one instance per logical role (one HTTP client, one Postgres pool, one
  RabbitMQ connection) and share it; do not construct per-request.
- Lifecycle (`Close`, `Shutdown`) is the caller's responsibility, deferred at
  process scope.

---

## 3. Canonical service bootstrap

The shape below is the recommended wiring for a new service that uses logging,
tracing, HTTP-out, Postgres, and RabbitMQ. Adapt by deletion — drop any
component you do not need rather than improvising.

```go
package main

import (
    "context"

    "os"

    "github.com/trypanic/go-sdk/errorkit"
    "github.com/trypanic/go-sdk/httpclient"
    "github.com/trypanic/go-sdk/httprequest"
    "github.com/trypanic/go-sdk/logger"
    "github.com/trypanic/go-sdk/messaging"
    "github.com/trypanic/go-sdk/postgres" // package identifier: database
    "github.com/trypanic/go-sdk/telemetry"
)

const (
    serviceName    = "my-service"
    serviceVersion = "0.1.0"
)

func main() {
    ctx := context.Background()

    // 1. OTel logs over OTLP (optional but recommended).
    logHandle, otlpWriter := logger.SetupOTLP(ctx, serviceName, "otel-collector:4317")
    defer logHandle.Shutdown(ctx)

    // 2. Global zerolog wired up. Same serviceName so traces and logs correlate.
    _, cleanup := logger.Init(serviceName, serviceVersion, otlpWriter)
    defer cleanup.Close()

    // 3. Tracing instrumenter — pass to packages that take *Instrumenter.
    instr := telemetry.NewInstrumenter(telemetry.InstrumenterConfig{
        ScopeName: serviceName,
    })

    // 4. Outbound HTTP — one client, one requester, shared across handlers.
    httpC := httpclient.NewDefaultClient()
    requester := httprequest.NewWithInstrumenter(httpC, instr)

    // 5. Postgres pool. Reads POSTGRES_DSN from env. Tracing is added later,
    //    per StoredProcedure, via database.WrapWithInstrumenter[T](...).
    pool, err := database.NewPostgresClientFromEnv(database.Config{})
    if err != nil {
        logger.Panic(err, "postgres init failed")
    }
    defer pool.Close()

    // 6. RabbitMQ. Topology source falls through:
    //    WithTopology > WithTopologyFile > RABBITMQ_TOPOLOGY_FILE env.
    mq, err := messaging.NewPubSub(os.Getenv("RABBITMQ_URL"))
    if err != nil {
        logger.Panic(err, "messaging init failed")
    }
    defer mq.Close()

    _ = errorkit.NewError // sanity import
    _ = requester
}
```

Key invariants:

1. **OTLP log provider before logger writer.** `SetupOTLP` enforces order.
2. **One `serviceName` everywhere.** Logs and traces correlate by service name.
3. **One instrumenter passed down.** Don't construct a fresh instrumenter per
   sub-component.
4. **Defer shutdowns at `main` scope.** Sub-packages do not own their lifecycle.

---

## 4. Idiomatic error handling

```go
result, err := svc.Do(ctx, input)
if err != nil {
    // Already an *errorkit.AppError? Preserve the original code.
    if appErr, ok := err.(*errorkit.AppError); ok {
        logger.ErrorCtx(ctx, appErr, "svc.Do failed")
        return appErr
    }
    // Otherwise wrap once at the boundary — pick a code that reflects the cause.
    wrapped := errorkit.NewError(errorkit.ERR_INTERNAL).
        With(errorkit.WithWrapped(err))
    logger.ErrorCtx(ctx, wrapped, "svc.Do failed")
    return wrapped
}
```

Rules:

- Wrap **once** at the boundary between your code and an external library.
- Never double-wrap an `*errorkit.AppError` with a generic `ERR_INTERNAL`.
- Pick the most specific code available; fall back to `ERR_INTERNAL` only when
  no specific code exists in the [error code reference](./errorkit/README.md#built-in-error-codes-reference).
- HTTP handler responses should serialize `appErr.PrettyJSON()` and use
  `appErr.Metadata.HTTPStatus`.

---

## 5. Anti-patterns (do not write this code)

- ❌ `httpclient.NewDefaultClient()` per request — defeats the connection pool.
- ❌ Calling `errorkit.NewError(errorkit.ERR_INTERNAL)` to wrap an existing
  `*errorkit.AppError` — destroys the original code and metadata.
- ❌ `log.Println` / `fmt.Println` / `slog` in service code — use `logger.*`.
- ❌ Reading env vars directly inside SDK call sites — use the `*FromEnv`
  constructor or `envs.Load[T]()`.
- ❌ `httprequest.WithRawBodies()` outside development — leaks credentials.
- ❌ Importing `ioutils` from production paths — it's a debug-only JSON dumper.
- ❌ Mutating the global zerolog logger after `Init` — race-prone.
- ❌ Reaching into `httpserver/hertz` from non-server code — pulls Hertz
  transitively into the binary.
- ❌ Closing or replacing an `*http.Client` between requests — discards pooled
  connections.
- ❌ Calling `telemetry.InitTracer(...)` in new code — compatibility shim only;
  use `telemetry.NewInstrumenter`.

---

## 6. Testing conventions

- Unit tests live alongside source: `foo.go` ↔ `foo_test.go`.
- `go test -short ./...` runs all unit tests with no external deps. Anything
  needing Postgres/Mongo/RabbitMQ must be gated behind `-short` or a
  `//go:build integration` tag.
- Mocks: prefer accepting interfaces (`HTTPRequester`, `errorkit.Registry`,
  `telemetry.Instrumenter`) and providing fakes in tests, over mocking
  concrete types.
- Use `stretchr/testify` (`require`, `assert`) — already a dependency.
- `*errorkit.AppError` assertions: check `.Code()`, not the string message.

---

## 7. Quality gates (run before committing)

```bash
go test -short ./...                      # unit tests
moon run repo:staticanalysis              # vet + staticcheck + semgrep
moon run repo:precommit-run               # all pre-commit hooks across the tree
```

Per-tool reports land in `.staticanalysis/`. See [`SECURITY.md`](./SECURITY.md)
for the full runner contract, env knobs, and CI workflow.

If you only changed one package, scope tests:

```bash
go test -short ./postgres/...
```

---

## 8. Working **inside** this repo (for SDK contributors / agents extending the SDK)

Read first when adding or modifying a package:

1. The target sub-`README.md` — describes the public contract you must preserve.
2. [`errorkit/README.md`](./errorkit/README.md) §"Registering New Error Codes"
   — for any new failure mode.
3. [`telemetry/README.md`](./telemetry/README.md) — for any new span.
4. [`SECURITY.md`](./SECURITY.md) — for the static-analysis policy and
   semgrep rule changes.

When adding a new public package:

- One sub-directory per package, one `README.md` per sub-directory.
- Tracing constructor triplet: `New`, `NewWithoutTracing`, `NewWithInstrumenter`.
- Errors use `errorkit` codes; register new codes with `errorkit.MustRegister`
  in the package's `init()` if they are SDK-defined, otherwise in `codes.go` +
  `registry.go`.
- Add the package to the table in §1 of this file and to the
  [`README.md`](./README.md) package map.
- Document concurrency and lifecycle expectations in the package `README.md`.

---

## 9. Where this file fits

- **README.md** — human-facing overview of the SDK (this is what
  pkg.go.dev points to).
- **AGENTS.md** (this file) — operational guide for AI agents using or
  extending the SDK.
- **Sub-package `README.md`** — authoritative contract for each package.
- **godoc** — symbol-level reference, generated from source comments.
- **CHANGELOG.md** — per-phase change log.

When in doubt: trust the source and the sub-`README.md` over this file. This
file is updated periodically; commit-level truth lives in the package itself.
