# httpserver

Framework-agnostic HTTP server contracts. The core package defines the
`Server`, `RouterGroup`, `Context`, `HandlerFunc`, and
`MiddlewareFunc` interfaces, the `Reply` envelope, and `ServerOptions`.
It pulls **no** web-framework code.

A Hertz adapter ships separately at `httpserver/hertz`. Consumers that
want Hertz import the adapter; consumers that want a different framework
implement the same interfaces.

```text
go-sdk/httpserver/         ← contracts + ServerOptions + Reply (no framework)
go-sdk/httpserver/hertz/   ← Hertz adapter (transitively pulls hertz deps)
```

---

## Quick Start (Hertz adapter)

```go
import (
    "context"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/trypanic/go-sdk/v2/httpserver"
    hertzadapter "github.com/trypanic/go-sdk/v2/httpserver/hertz"
    "github.com/trypanic/go-sdk/v2/logger"
)

func main() {
    srv := hertzadapter.NewServer(httpserver.ServerConfig{
        Host: "0.0.0.0",
        Port: 8080,
    })

    srv.GET("/hello", func(ctx context.Context, c httpserver.Context) {
        c.JSON(http.StatusOK, map[string]string{"message": "hello"})
    })

    // Run installs no signal handlers: the caller owns the lifecycle.
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    runErr := make(chan error, 1)
    go func() { runErr <- srv.Run() }()

    select {
    case err := <-runErr: // e.g. address already in use (*errorkit.AppError)
        logger.Panic(err, "http server failed to start")
    case <-ctx.Done():
    }

    // Graceful shutdown.
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    _ = srv.Shutdown(shutdownCtx)
}
```

---

## Core types

### `Server`

```go
type Server interface {
    GET(endpoint string, handler HandlerFunc)
    POST(endpoint string, handler HandlerFunc)
    PUT(endpoint string, handler HandlerFunc)
    DELETE(endpoint string, handler HandlerFunc)
    Group(prefix string) RouterGroup
    Use(middleware ...MiddlewareFunc)
    Run() error
    Shutdown(ctx context.Context) error
}
```

`Run()` blocks until the server terminates. It returns `nil` after a
graceful `Shutdown`, and an error when the server cannot start (the Hertz
adapter returns `ERR_SYSTEM_CONFIG_INVALID` for a listen failure such as
"address already in use"). Adapters install no signal handlers: stop the
server by calling `Shutdown`, for example on `signal.NotifyContext`. `Shutdown(ctx)` triggers a
graceful drain bounded by the supplied context. Adapters MAY return a
backend-specific error from `Shutdown` (e.g. when called before `Run`).

### `ServerOptions`

```go
type ServerOptions struct {
    EnableTracing  bool
    EnableRecovery bool
    EnableAudit    bool
    EnableHealth   bool
    EnableNoRoute  bool
    EnableNoMethod bool

    NoRouteStatus   int
    NoMethodStatus  int
    HealthPath      string                          // default "/health"
    HealthCheck     func(ctx context.Context) error // optional readiness hook
    NoRouteHandler  HandlerFunc
    NoMethodHandler HandlerFunc

    Audit AuditOptions
    Reply ReplyOptions
}
```

`DefaultServerOptions()` returns the compatibility profile:

| Field | Default |
|---|---|
| `EnableTracing` / `EnableRecovery` / `EnableAudit` / `EnableHealth` / `EnableNoRoute` / `EnableNoMethod` | `true` |
| `NoRouteStatus` | `404` |
| `NoMethodStatus` | `405` |
| `HealthPath` | `"/health"` (`DefaultHealthPath`) |
| `HealthCheck` | `nil` (liveness: always 200) |
| `NoRouteHandler` / `NoMethodHandler` | `nil` (use status defaults) |
| `Audit.CaptureBodies` | `false` |
| `Audit.BodyRedactor` | `DefaultBodyRedactor` (replaces non-empty body with `[REDACTED]`) |
| `Reply.Layout` | `"2006-01-02 15:04:05"` |
| `Reply.Clock` | `time.Now` |

**Health endpoint.** With `EnableHealth`, `GET <HealthPath>` answers
`200 {"status":"healthy"}`. The path is reserved: registering your own
`GET /health` on a default server panics in the framework, so set
`HealthPath` (e.g. `"/livez"`) or `EnableHealth = false`. Set
`HealthCheck` to make it a readiness probe: a non-nil error answers
`503 {"status":"unhealthy"}` and is logged at warn level, never sent.

`EnableRecovery` is the only recovery layer. When `true`, the adapter's
`RecoveryHandler` is the outermost middleware (it also covers the built-in
tracing and audit middlewares) and logs one structured error line with the
panic stack (`panic_stack`) through `logger.CtxOrGlobal(ctx)`. When
`false`, no recovery is installed and a handler panic propagates (with
Hertz it crashes the process); install your own recovery middleware if you
need one.

To opt out of a middleware, set its `Enable*` flag to `false`. To install
your own NoRoute/NoMethod response, set `NoRouteHandler` /
`NoMethodHandler`. To capture audit bodies under your own policy, set
`Audit.CaptureBodies = true` and `Audit.BodyRedactor` to a function that
implements your redaction policy.

### `Reply` and `OptionReply`

```go
type Reply struct {
    Message   *string `json:"message,omitempty"`
    Code      string  `json:"code,omitempty"`
    Error     *string `json:"error,omitempty"`
    TraceID   string  `json:"trace_id,omitempty"`
    Timestamp string  `json:"timestamp,omitempty"`
    Metadata  any     `json:"metadata,omitempty"`
    Data      any     `json:"data,omitempty"`
}

type OptionReply func(*Reply)

func WithMessageOpt(message string) OptionReply
func WithErrorOpt(err error) OptionReply       // client-safe code + message; copies errorkit TraceID
func WithMetadataOpt(metadata any) OptionReply
func WithDataOpt(data any) OptionReply

func BuildReply(opts ReplyOptions, options ...OptionReply) *Reply
```

`WithDataOpt` and `WithMetadataOpt` store the value as given, so
`Reply.Data` holds the caller's type and a `nil` value is omitted from the
JSON (`omitempty`) instead of being sent as `null`.

**Error replies never carry `err.Error()`.** The text of an error can hold
SQL, hostnames, file paths or panic values, so `WithErrorOpt` sets:

| Error | `code` | `error` |
|---|---|---|
| `*errorkit.AppError` with a 4xx `Metadata.HTTPStatus` | its error code | its `Reason` (falls back to `Metadata.Description`) |
| any other `*errorkit.AppError` | its error code | its `Metadata.Description` |
| any other error | `ERR_INTERNAL` | `Internal server error` |

The `*errorkit.AppError` is found anywhere in the chain (`errors.As`). The
Hertz adapter's `Context.WithError` also logs the full error
server-side through `logger.CtxOrGlobal(ctx)` with `trace_id`/`span_id`
(warn level for 4xx, error level otherwise) and fills `trace_id` in the
reply from the active span when the error carries none.

`Context.Reply(status, opts...)` ultimately calls `BuildReply` with
the server's `ReplyOptions`. The `Timestamp` is stamped from
`opts.Clock().Format(opts.Layout)` so callers can override the layout or
inject a deterministic clock for tests.

### Body redaction

```go
type BodyRedactor func([]byte) []byte

func DefaultBodyRedactor(b []byte) []byte // → []byte("[REDACTED]") if non-empty
func RawBodyRedactor(b []byte) []byte     // identity, opt-in only
```

The inbound audit middleware (`EnableAudit`, on by default) logs one
structured line per request through `logger.CtxOrGlobal(ctx)`, with
`trace_id`/`span_id` from the active span:

```json
{"level":"info","event":"http.audit.inbound","method":"POST","path":"/orders",
 "status_code":201,"request.headers":{"Authorization":"[REDACTED]","Content-Type":"application/json"},
 "request.body":"[REDACTED]","response.headers":{...},"response.body":"[REDACTED]",
 "duration_ms":12,"message":"http: inbound audit"}
```

- The query string is never logged (`path` only).
- Header values that may carry credentials (`Authorization`,
  `Proxy-Authorization`, `Cookie`, `Set-Cookie`, API-key headers and any
  name containing `token`, `secret`, `key`, `auth`, `password`, `session`,
  `signature` or `credential`) are logged as `[REDACTED]`; the keys stay.
- `Referer`, `Location` and `Content-Location` values omit userinfo and
  fragments and replace query values with `REDACTED`. Malformed URLs
  become `[invalid URL]`; opaque URLs are masked in full. `Link` and
  `Refresh` are masked in full because they contain compound URL syntax.
  Other diagnostic headers are preserved.
- With `Audit.CaptureBodies = false` (default) every non-empty body is
  logged as `[REDACTED]`, whatever `BodyRedactor` is set.
- With `Audit.CaptureBodies = true` bodies pass through
  `Audit.BodyRedactor`. Setting it to `RawBodyRedactor` is the explicit
  opt-in for raw capture (development only).
- Streamed bodies are never read; they are logged as `[STREAM]`.

The line costs one log write per request; set `EnableAudit = false` to
turn it off.

---

## Hertz adapter (`go-sdk/httpserver/hertz`)

| Symbol | Purpose |
|---|---|
| `New(cfg)` | Hertz `Server` with `DefaultServerOptions()` |
| `NewWithOptions(cfg, opts)` | Hertz `Server` with explicit options; only zero non-boolean fields backfilled from defaults; zero booleans remain disabled |
| `RecoveryHandler` | Panic-recovery handler installed by `EnableRecovery` |
| `RequestLogger(base *logger.Logger)` | Per-request logger as an `httpserver.MiddlewareFunc` (install with `srv.Use`); reuses and echoes `X-Request-ID` |
| `RequestLoggerMiddleware(base *logger.Logger)` | Same middleware as a Hertz `app.HandlerFunc` |

`Shutdown(ctx)` waits until no request is in flight or `ctx` is done;
only `ctx` bounds it (Hertz's 5 s cap is lifted) and idle keep-alive
connections do not delay it. It returns `ERR_SYSTEM_TIMEOUT_INTERNAL`
when `ctx` ends with requests still in flight. Calling it before `Run()`
returns Hertz's `engine is not running` error; this is intentional and
surfaced verbatim.

---

## Lifecycle

- All route registration must happen before `Run()`.
- `Run()` blocks. Run it from the main goroutine or a dedicated one, and
  handle its error: a listen failure is returned, never a panic.
- No adapter handles SIGINT/SIGTERM; the caller decides when to call
  `Shutdown`.
- `Shutdown(ctx)` triggers graceful drain; the supplied context bounds
  how long to wait, and a drain cut short by it is reported as
  `ERR_SYSTEM_TIMEOUT_INTERNAL`.
- `Context` is single-goroutine. Do not stash it past the handler.

---

## Acceptance criteria (Phase 12)

- Core httpserver package has zero web-framework imports.
- `New` and `NewServer` enable the default middleware profile.
  `NewWithOptions` uses explicit `ServerOptions`; zero booleans stay disabled.
- `Shutdown(ctx)` is part of the `Server` interface.
- 404/405 are the defaults for unknown routes/methods; the previous
  HTTP 200 behavior is no longer the default.
- Audit body capture is OFF by default; enabling it requires an explicit
  `BodyRedactor` policy.

### HTTP methods

`Server` and `RouterGroup` directly support `PATCH`, `HEAD`, `OPTIONS`,
and `Handle(method, path, handler)` alongside GET/POST/PUT/DELETE. Custom
implementations must implement these methods in v2. Group prefixes and
middleware apply equally to every method.

### Concrete adapters and escape hatches (v2)

Constructors return concrete adapters; consumers should accept the narrow
interfaces they need. HTTP and LLM clients retain the constructor's instrumenter
inside the concrete adapter, producing one span per call. `WrapWithInstrumenter`
still preserves its input when the instrumenter is nil; an explicit non-nil wrapper
exposes `Unwrap()` to recover the enclosed interface. Wrapping an already traced
client intentionally creates another span, so wrap a tracing-disabled client.

Hertz constructors return `*hertz.Server`; `NewEngine(cfg)` also exposes this
concrete adapter. `Engine()` gives access to the underlying Hertz engine for
framework features. Configure routes and middleware before starting the server.

### Functional server options

Prefer `hertz.NewServer(cfg, opts...)`: zero options enable tracing, recovery,
redacted audit, health and 404/405 handling. Options include `WithoutTracing`,
`WithoutRecovery`, `WithoutAudit`, `WithoutHealth`, `WithoutNoRoute`,
`WithoutNoMethod`, `WithLogger`, `WithHealth`, `WithAudit` and `WithReply`.
`WithLogger` applies to audit and recovery as well as handler context logging.
`NewWithOptions` remains the explicit struct profile: its boolean zero values
are disabled, so initialize that struct with `DefaultServerOptions()` when needed.

### Metadata-driven error replies

Use `httpserver.ReplyError(c, err)` in handlers to select the status from an
`AppError` anywhere in the error chain and send the adapter's client-safe reply.
The adapter's `WithError` hook still logs the failure. Nil errors do not write.
`StatusFor(nil)` returns 200; unknown errors and invalid failure statuses return
500. Registered 400–599 statuses are preserved. Do not serialize `err.Error()`.

Consumer tests can import [`httpservertest`](./httpservertest/README.md) for a reusable public test double without a live service.
