# httpserver

Framework-agnostic HTTP server contracts. The core package defines the
`HTTPServer`, `RouterGroup`, `HTTPContext`, `HandlerFunc`, and
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

    "github.com/trypanic/go-sdk/httpserver"
    hertzadapter "github.com/trypanic/go-sdk/httpserver/hertz"
)

func main() {
    srv := hertzadapter.New(httpserver.ServerConfig{
        Host: "0.0.0.0",
        Port: 8080,
    })

    srv.GET("/hello", func(ctx context.Context, c httpserver.HTTPContext) {
        c.JSON(http.StatusOK, map[string]string{"message": "hello"})
    })

    go func() { _ = srv.Run() }()

    // Graceful shutdown.
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    _ = srv.Shutdown(shutdownCtx)
}
```

---

## Core types

### `HTTPServer`

```go
type HTTPServer interface {
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

`Run()` blocks until the server terminates. `Shutdown(ctx)` triggers a
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
| `NoRouteHandler` / `NoMethodHandler` | `nil` (use status defaults) |
| `Audit.CaptureBodies` | `false` |
| `Audit.BodyRedactor` | `DefaultBodyRedactor` (replaces non-empty body with `[REDACTED]`) |
| `Reply.Layout` | `"2006-01-02 15:04:05"` |
| `Reply.Clock` | `time.Now` |

To opt out of a middleware, set its `Enable*` flag to `false`. To install
your own NoRoute/NoMethod response, set `NoRouteHandler` /
`NoMethodHandler`. To capture audit bodies under your own policy, set
`Audit.CaptureBodies = true` and `Audit.BodyRedactor` to a function that
implements your redaction policy.

### `Reply` and `OptionReply`

```go
type Reply struct {
    Message   *string `json:"message,omitempty"`
    Error     *string `json:"error,omitempty"`
    TraceID   string  `json:"trace_id,omitempty"`
    Timestamp string  `json:"timestamp,omitempty"`
    Metadata  any     `json:"metadata,omitempty"`
    Data      any     `json:"data,omitempty"`
}

type OptionReply func(*Reply)

func WithMessageOpt(message string) OptionReply
func WithErrorOpt(err error) OptionReply       // copies errorkit TraceID
func WithMetadataOpt(metadata any) OptionReply
func WithDataOpt(data any) OptionReply

func BuildReply(opts ReplyOptions, options ...OptionReply) *Reply
```

`HTTPContext.Reply(status, opts...)` ultimately calls `BuildReply` with
the server's `ReplyOptions`. The `Timestamp` is stamped from
`opts.Clock().Format(opts.Layout)` so callers can override the layout or
inject a deterministic clock for tests.

### Body redaction

```go
type BodyRedactor func([]byte) []byte

func DefaultBodyRedactor(b []byte) []byte // → []byte("[REDACTED]") if non-empty
func RawBodyRedactor(b []byte) []byte     // identity, opt-in only
```

The inbound audit middleware passes every captured request/response body
through `Audit.BodyRedactor` before logging. Setting `BodyRedactor` to
`RawBodyRedactor` is the explicit opt-in for raw capture.

---

## Hertz adapter (`go-sdk/httpserver/hertz`)

| Symbol | Purpose |
|---|---|
| `New(cfg)` | Hertz `HTTPServer` with `DefaultServerOptions()` |
| `NewWithOptions(cfg, opts)` | Hertz `HTTPServer` with explicit options; zero fields backfilled from defaults |
| `RecoveryHandler` | Panic-recovery handler installed by `EnableRecovery` |
| `RequestLoggerMiddleware(base *logger.Logger)` | Per-request logger middleware |

`Shutdown(ctx)` delegates to `*server.Hertz.Shutdown(ctx)`. Calling it
before `Run()` returns Hertz's `engine is not running` error; this is
intentional and surfaced verbatim.

---

## Lifecycle

- All route registration must happen before `Run()`.
- `Run()` blocks. Run it from the main goroutine or a dedicated one.
- `Shutdown(ctx)` triggers graceful drain; the supplied context bounds
  how long to wait before forcing termination.
- `HTTPContext` is single-goroutine. Do not stash it past the handler.

---

## Acceptance criteria (Phase 12)

- Core httpserver package has zero web-framework imports.
- All built-in middleware (tracing, recovery, audit, health, NoRoute,
  NoMethod) is opt-in via `ServerOptions`.
- `Shutdown(ctx)` is part of the `HTTPServer` interface.
- 404/405 are the defaults for unknown routes/methods; the previous
  HTTP 200 behavior is no longer the default.
- Audit body capture is OFF by default; enabling it requires an explicit
  `BodyRedactor` policy.
