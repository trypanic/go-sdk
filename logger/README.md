# logger

Structured, environment-aware logging for Go applications — built on zerolog with first-class OpenTelemetry trace correlation and native errorkit integration.

## Table of Contents

- [logger](#logger)
  - [Table of Contents](#table-of-contents)
  - [Overview](#overview)
  - [Quick Start](#quick-start)
  - [Configuration / Settings](#configuration--settings)
    - [Environment Detection](#environment-detection)
    - [No Additional Config Structs](#no-additional-config-structs)
  - [API Reference](#api-reference)
    - [Init](#init)
    - [DetectEnv](#detectenv)
    - [InitLogProvider](#initlogprovider)
    - [NewOTLPWriter](#newotlpwriter)
    - [LogProviderHandle.Shutdown](#logproviderhandleshutdown)
    - [Error](#error)
    - [LogAndReturn](#logandreturn)
    - [Panic](#panic)
    - [Warn](#warn)
    - [Info](#info)
    - [ErrorCtx](#errorctx)
    - [WarnCtx](#warnctx)
    - [LogInfoWithTrace](#loginfowithtrace)
    - [LogErrorWithTrace](#logerrorwithtrace)
    - [LogWarnWithTrace](#logwarnwithtrace)
    - [WithTrace](#withtrace)
    - [WithErrorKit](#witherrorkit)
    - [TraceIDFromCtx](#traceidfromctx)
    - [Ctx](#ctx)
    - [CtxOrGlobal](#ctxorglobal)
    - [WithLogger](#withlogger)
  - [Real-World Usage](#real-world-usage)
    - [Bootstrap Without OTLP](#bootstrap-without-otlp)
    - [Bootstrap With OTLP](#bootstrap-with-otlp)
    - [Instance API](#instance-api)
    - [Fatal Startup Errors](#fatal-startup-errors)
    - [Logging Errors With Trace Context](#logging-errors-with-trace-context)
    - [HTTP Handler](#http-handler)
    - [Low-Level Event Building](#low-level-event-building)
  - [Lifecycle / Concurrency Notes](#lifecycle--concurrency-notes)
  - [Dependencies](#dependencies)

---

## Overview

| Symbol               | Kind            | Purpose                                                            |
| -------------------- | --------------- | ------------------------------------------------------------------ |
| `Environment`        | type (`string`) | Typed constant for `Dev` / `Prod`                                  |
| `Dev`                | const           | `"development"`                                                    |
| `Prod`               | const           | `"production"`                                                     |
| `OTLPWriter`         | struct          | `io.Writer` that converts zerolog JSON lines to OTEL `LogRecord`s  |
| `LogProviderHandle`  | struct          | Wraps the SDK `LoggerProvider`; call `Shutdown` on exit            |
| `Init`               | func            | Creates, configures, and installs the global logger                |
| `DetectEnv`          | func            | Reads `ENVIRONMENT` env var; defaults to `Dev`                     |
| `InitLogProvider`    | func            | Creates and registers a global OTLP `LoggerProvider` over gRPC     |
| `NewOTLPWriter`      | func            | Creates an `OTLPWriter` backed by the global `LoggerProvider`      |
| `SetupOTLP`          | func            | Bundles `InitLogProvider` + `NewOTLPWriter` in correct order; returns handle + writer |
| `Error`              | func            | Logs an `errorkit.AppError` at Error level via the global logger   |
| `LogAndReturn`       | func            | `Error` + returns the original error                               |
| `Panic`              | func            | `Error` + `os.Exit(1)` (non-zero exit so supervisors detect failure) |
| `New`                | func            | Builds an explicit `*Logger` instance from `Config` without touching the global |
| `Logger`             | struct          | Instance API: `Info`, `Warn`, `Error`, `InfoCtx`, `ErrorCtx`, `WarnCtx`, `WithFields`, `IntoContext`, `Zerolog` |
| `Config`             | struct          | Instance config: `AppName`, `Version`, `Env`, `Writer`, `OTLPWriter` |
| `Warn`               | func            | Printf-style Warn via the global logger                            |
| `Info`               | func            | Printf-style Info via the global logger                            |
| `ErrorCtx`           | func            | Error log with context logger and live OTel `trace_id` / `span_id` |
| `WarnCtx`            | func            | Warn log with context logger and live OTel `trace_id` / `span_id`  |
| `LogInfoWithTrace`   | func            | Info log with live OTel `trace_id` / `span_id`                     |
| `LogErrorWithTrace`  | func            | Error log with live OTel trace fields (equivalent to `ErrorCtx`)   |
| `LogWarnWithTrace`   | func            | Warn log with live OTel trace fields (equivalent to `WarnCtx`)     |
| `WithTrace`          | func            | Low-level: adds `trace_id` / `span_id` to a `*zerolog.Event`       |
| `WithErrorKit`       | func            | Low-level: adds all errorkit fields to a `*zerolog.Event`          |
| `TraceIDFromCtx`     | func            | Returns the W3C trace-id string from the active OTel span, or `""` |
| `Ctx`                | func            | Alias for `zerolog.Ctx` — extracts the logger stored in a context  |
| `CtxOrGlobal`        | func            | `Ctx` with fallback to the global logger                           |
| `WithLogger`         | func            | Stores a `*zerolog.Logger` into a context                          |

---

## Quick Start

```go
import (
    "github.com/trypanic/go-sdk/logger"
    "github.com/trypanic/go-sdk/errorkit"
)

func init() {
    _, cleanup := logger.Init("my-service", "1.0.0")
    defer cleanup.Close()
}

func main() {
    logger.Info("Starting %s", "my-service")

    if err := doWork(); err != nil {
        appErr := errorkit.NewError(errorkit.ERR_INTERNAL).
            With(errorkit.WithWrapped(err))
        logger.Panic(appErr, "startup failed")
    }
}
```

---

## Configuration / Settings

### Environment Detection

`Init` calls `DetectEnv()` internally to select the output format.

| `ENVIRONMENT` value      | Resolved constant | Log level | Output format             |
| ------------------------ | ----------------- | --------- | ------------------------- |
| `production`, `prod`     | `Prod`            | `Info`    | JSON to stdout            |
| anything else (or unset) | `Dev`             | `Debug`   | Colored console to stdout |

Both environments add `timestamp` (Unix epoch) and `caller` (file:line) to every log line automatically.

**Dev console** pretty-prints `service_name`, `service_version`, and all extra fields below the log line using blue ANSI color; `stack_trace` frames are JSON-marshalled inline.

### No Additional Config Structs

The logger has no `Config` struct. All behavior is controlled by:

- The `ENVIRONMENT` environment variable (see table above).
- The optional `*OTLPWriter` variadic argument to `Init`.

---

## API Reference

### Init

```go
func Init(appName, version string, otlpWriter ...*OTLPWriter) (*zerolog.Logger, io.Closer)
```

Creates and installs the global logger. Sets both the package-level global and `zerolog`'s `log.Logger` so code using `zerolog` directly picks up the same configuration.

- `appName` and `version` are embedded in every Dev console line as `service_name` and `service_version`.
- If an `*OTLPWriter` is supplied, each log event is fanned out to both the console/stdout writer **and** the OTLP writer via `zerolog.MultiLevelWriter`.
- Returns the logger pointer and an `io.NopCloser` (the closer is a no-op; it exists for forward-compatibility with the OTLP path).

Calling `Init` more than once replaces the global logger.

**Fallback behavior before Init.** Calling `logger.Info`, `logger.Warn`, `logger.Error`, etc. before `Init` does not panic. The package falls back to a minimal `zerolog.New(os.Stderr).With().Timestamp().Logger()` (`globalLogger()` in `logger.go`). The fallback writes to stderr in plain JSON without redaction. Production code must still call `Init` early in `main` to get the configured pretty/Prod writer and any OTLP fan-out.

**Instance API.** SDK consumers that prefer not to mutate the package global can use `logger.New(Config{...})` and pass the resulting `*Logger` explicitly. See `instance.go` and the `New` / `Logger` table entries below.

---

### DetectEnv

```go
func DetectEnv() Environment
```

Reads `os.Getenv("ENVIRONMENT")`. Returns `Prod` for `"production"` or `"prod"`; returns `Dev` for any other value including the empty string.

---

### InitLogProvider

```go
func InitLogProvider(ctx context.Context, serviceName, collectorEndpoint string) *LogProviderHandle
```

Creates an OTLP gRPC log exporter connected to `collectorEndpoint` (e.g. `"otel-collector:4317"`), wraps it in a batching `log.LoggerProvider`, and registers it as the global OTEL `LoggerProvider`.

- Must be called **before** `NewOTLPWriter` — `NewOTLPWriter` resolves the global provider.
- Uses `grpc.WithBlock()` so connection errors surface at startup. On failure it prints to stderr and returns a no-op provider; the service continues running but logs are not exported to SigNoz.
- `serviceName` must match the value passed to your trace provider's `WithServiceName` option so SigNoz can correlate logs with traces.

---

### NewOTLPWriter

```go
func NewOTLPWriter(serviceName string) *OTLPWriter
```

Returns an `*OTLPWriter` backed by the global OTEL `LoggerProvider`. Each `Write` call receives one complete JSON log line from zerolog and converts it to an OTEL `LogRecord`:

- Timestamp from the `"time"` field (Unix epoch seconds).
- Severity mapped from the `"level"` field.
- Body from `"message"`.
- Trace correlation: `trace_id` and `span_id` fields written by `WithTrace` / `ErrorCtx` are promoted to OTEL first-class fields via a reconstructed `SpanContext` passed to `Emit`.
- All other fields (`error_code`, `reason`, `stack_trace`, etc.) become log attributes visible in SigNoz's log detail panel.

`level`, `time`, `message`, `trace_id`, `span_id`, and `caller` are excluded from the attributes list (they map to dedicated OTEL fields or are omitted for noise reduction).

Must be called after `InitLogProvider`.

---

### LogProviderHandle.Shutdown

```go
func (h *LogProviderHandle) Shutdown(ctx context.Context)
```

Flushes buffered log records, closes the gRPC exporter. Always defer this immediately after `InitLogProvider` returns.

---

### Error

```go
func Error(appErr error, msg ...string) error
```

Logs `appErr` at Error level using the global logger.

- If `appErr` is an `*errorkit.AppError`, all errorkit fields are attached via `WithErrorKit`.
- If `appErr` is a plain `error`, it is wrapped in `errorkit.NewError(errorkit.ERR_INTERNAL)` before logging.
- `msg` (optional) should describe **what happened**, not repeat the error details already in `appErr.Reason`.
- Returns `appErr` unchanged so callers can chain: `return logger.Error(err, "msg")`.

Does not inject trace fields. Use `ErrorCtx` when a context with an active span is available.

---

### LogAndReturn

```go
func LogAndReturn(appErr error, msg ...string) error
```

Calls `Error(appErr, msg...)` and returns `appErr`. Syntactic sugar for the common pattern of logging and immediately returning the same error.

---

### Panic

```go
func Panic(appErr error, msg ...string)
```

Calls `Error(appErr, msg...)` then `os.Exit(1)`. Non-zero exit ensures supervisors (Lambda, systemd, k8s) detect the failure. Used for fatal errors at service startup or during subscription setup, where the process cannot continue.

---

### Warn

```go
func Warn(msg string, args ...any)
```

Printf-style Warn using the global logger (`global.Warn().Msgf(msg, args...)`). Does not accept an `*errorkit.AppError`. Use `WarnCtx` for structured warning logging.

---

### Info

```go
func Info(msg string, args ...any)
```

Printf-style Info using the global logger. Commonly used for startup banners and lifecycle messages where no context or span is available.

---

### ErrorCtx

```go
func ErrorCtx(ctx context.Context, appErr *errorkit.AppError, msg string)
```

The primary error logging function for service handlers and interactors. Combines:

1. The logger from `ctx` (via `CtxOrGlobal`) — inherits any request-scoped fields injected by `HertzRequestLogger`.
2. `trace_id` and `span_id` from the active OTel span in `ctx` (via `WithTrace`).
3. All errorkit fields from `appErr` (via `WithErrorKit`).

Trace fields come from the live span, not from `appErr.TraceID`, to avoid field collisions.

---

### WarnCtx

```go
func WarnCtx(ctx context.Context, appErr *errorkit.AppError, msg string)
```

Same as `ErrorCtx` but emits at Warn level. Intended for retriable / transient errors (e.g. cache timeouts) where the caller will retry and the error is not fatal.

---

### LogInfoWithTrace

```go
func LogInfoWithTrace(ctx context.Context, msg string)
```

Info log enriched with `trace_id` and `span_id` from the active OTel span. Used to bracket operations within a trace span.

---

### LogErrorWithTrace

```go
func LogErrorWithTrace(ctx context.Context, appErr *errorkit.AppError, msg string)
```

Equivalent to `ErrorCtx`. Provided as an alternative name; both produce identical output.

---

### LogWarnWithTrace

```go
func LogWarnWithTrace(ctx context.Context, appErr *errorkit.AppError, msg string)
```

Equivalent to `WarnCtx`.

---

### WithTrace

```go
func WithTrace(ctx context.Context, event *zerolog.Event) *zerolog.Event
```

Low-level building block. Reads the active OTel span from `ctx` and adds `trace_id` and `span_id` string fields to `event`. Fields are only added if the span context actually carries a trace ID / span ID. Returns the enriched event for chaining.

---

### WithErrorKit

```go
func WithErrorKit(event *zerolog.Event, appErr *errorkit.AppError) *zerolog.Event
```

Low-level building block. Adds the following fields to `event`:

| Field             | Source                                                               |
| ----------------- | -------------------------------------------------------------------- |
| `error_code`      | `appErr.Code()`                                                      |
| `error_id`        | `appErr.ID`                                                          |
| `error_timestamp` | `appErr.Timestamp`                                                   |
| `error_type`      | `appErr.Metadata.Type`                                               |
| `error_group`     | `appErr.Metadata.Group`                                              |
| `error_category`  | `appErr.Metadata.Category`                                           |
| `http_status`     | `appErr.Metadata.HTTPStatus`                                         |
| `retriable`       | `appErr.Metadata.Retriable`                                          |
| `reason`          | `appErr.Reason` (only if non-empty)                                  |
| `payload`         | `appErr.Payload` (only if non-nil)                                   |
| `wrapped_error`   | `appErr.Wrapped` (only if non-nil)                                   |
| `stack_trace`     | `appErr.Trace` as array of `{file, line, package, function}` objects |

Does **not** emit `trace_id` or `span_id` — those are exclusively owned by `WithTrace` to prevent field collision when both are used together.

Does **not** call `.Msg()`. The caller must finalize the event.

---

### TraceIDFromCtx

```go
func TraceIDFromCtx(ctx context.Context) string
```

Returns the W3C hex trace-id from the active OTel span in `ctx`, or `""` if no span is present. Use this when you need to embed a trace ID inside an `errorkit.AppError` to carry it outside a context-bearing call chain (e.g. a queue message payload).

```go
appErr := errorkit.NewError(errorkit.ERR_DB_MONGO_ERROR).
    With(errorkit.WithTraceID(logger.TraceIDFromCtx(ctx)))
```

---

### Ctx

```go
func Ctx(ctx context.Context) *zerolog.Logger
```

Direct alias for `zerolog.Ctx(ctx)`. Returns the logger stored in `ctx`, or a disabled logger if none exists. Use `CtxOrGlobal` if you want a fallback.

---

### CtxOrGlobal

```go
func CtxOrGlobal(ctx context.Context) *zerolog.Logger
```

Returns the logger stored in `ctx`. Falls back to the global logger if the context logger is at `zerolog.Disabled` level (which is what `zerolog.Ctx` returns when no logger was stored).

---

### WithLogger

```go
func WithLogger(ctx context.Context, l *zerolog.Logger) context.Context
```

Stores `l` in `ctx` so it can be retrieved later via `Ctx` or `CtxOrGlobal`. Thin wrapper around `l.WithContext(ctx)`.

---

### Web framework adapters

The core `logger` package has **no web framework dependency**. Per-request middleware that injects a request-scoped logger lives in adapter packages alongside each web server. For example, the SDK ships a Hertz adapter inside the `httpserver` package:

```go
import "github.com/trypanic/go-sdk/httpserver"

engine.Use(httpserver.RequestLoggerMiddleware(nil)) // nil = use global logger as base
```

In your own project you can write a similar middleware in three lines: build a child `zerolog.Logger` with `request_id` / `method` / `path` / `remote_addr` and call `l.WithContext(ctx)`. Downstream handlers retrieve the enriched logger via `logger.Ctx(ctx)` or `logger.CtxOrGlobal(ctx)`.

---

## Real-World Usage

The examples below use a generic `my-service` name. Replace it with your own service identifier; the logger has no project-specific assumptions.

### Bootstrap Without OTLP

`Init` is typically called from an `init()` function or at the top of `main` so the logger is ready before any other code runs.

```go
const (
    serviceName    = "my-service"
    serviceVersion = "0.1.0"
)

func init() {
    _, cleanup := logger.Init(serviceName, serviceVersion)
    defer cleanup.Close()
}

func main() {
    logger.Info("Starting %s %s", serviceName, serviceVersion)
    // ...
}
```

---

### Bootstrap With OTLP

The OTLP path needs the log provider before the logger writer, and they must share the same service name. Use `SetupOTLP` to bundle the order safely:

```go
func main() {
    ctx := context.Background()

    // Optional: configure your trace provider first if you want traces too.

    // Bundle InitLogProvider + NewOTLPWriter in correct order.
    handle, otlpWriter := logger.SetupOTLP(ctx, serviceName, "otel-collector:4317")
    defer handle.Shutdown(ctx)

    _, cleanup := logger.Init(serviceName, serviceVersion, otlpWriter)
    defer cleanup.Close()

    logger.Info("Starting %s %s", serviceName, serviceVersion)
}
```

The same `serviceName` should be used by your trace provider so log records and trace spans correlate under one service in your observability backend.

---

### Instance API

`logger.New` returns an explicit `*Logger` without mutating package state, useful in libraries or tests that should not depend on the global logger:

```go
l := logger.New(logger.Config{
    AppName: "my-service",
    Version: "0.1.0",
    Env:     logger.Prod,
})

l.Info("hello %s", "world")

ctx := l.IntoContext(context.Background()) // downstream code can use logger.Ctx(ctx)
l.WithFields(map[string]any{"request_id": "rid-42"}).
    InfoCtx(ctx, "processing request")
```

---

### Fatal Startup Errors

`logger.Panic` is the standard pattern for unrecoverable startup failures. It logs the error and then calls `os.Exit(1)` so process supervisors detect the failure.

```go
if err := loadConfig(); err != nil {
    logger.Panic(
        errorkit.NewError(errorkit.ERR_INTERNAL).With(errorkit.WithWrapped(err)),
        "config load failed",
    )
}
```

---

### Logging Errors With Trace Context

Wrap untyped errors in `errorkit.AppError` before logging, then use `ErrorCtx` to attach trace fields automatically:

```go
func handle(ctx context.Context, msg Message) error {
    logger.LogInfoWithTrace(ctx, "handler: message received")

    if err := process(ctx, msg); err != nil {
        appErr := errorkit.NewError(errorkit.ERR_INTERNAL).With(errorkit.WithWrapped(err))
        logger.ErrorCtx(ctx, appErr, "handler: process failed")
        return appErr
    }

    logger.LogInfoWithTrace(ctx, "handler: message processed")
    return nil
}
```

When a function already returns `*errorkit.AppError`, type-switch to preserve the original code:

```go
result, err := svc.Do(ctx, input)
if err != nil {
    if appErr, ok := err.(*errorkit.AppError); ok {
        logger.ErrorCtx(ctx, appErr, "svc: do")
    } else {
        wrapped := errorkit.NewError(errorkit.ERR_INTERNAL).With(errorkit.WithWrapped(err))
        logger.ErrorCtx(ctx, wrapped, "svc: do")
    }
    return err
}
```

Without a context (e.g. blocking startup paths), use `logger.Error`:

```go
if err := startSubscriber(); err != nil {
    appErr := errorkit.NewError(errorkit.ERR_INTERNAL).With(errorkit.WithWrapped(err))
    logger.Error(appErr, "subscribe failed")
}
```

---

### HTTP Handler

The logger is web-framework agnostic. Any handler that has access to a `context.Context` can use the trace-aware helpers:

```go
func handler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
    logger.LogInfoWithTrace(ctx, "handler: request received")

    if err := doWork(ctx, r); err != nil {
        appErr := errorkit.NewError(errorkit.ERR_INTERNAL).With(errorkit.WithWrapped(err))
        logger.ErrorCtx(ctx, appErr, "handler: work failed")
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }

    logger.LogInfoWithTrace(ctx, "handler: request complete")
}
```

---

### Low-Level Event Building

Use `WithTrace` and `WithErrorKit` directly when the high-level helpers do not fit:

```go
event := logger.WithTrace(ctx, zerolog.GlobalLogger().Error())
logger.WithErrorKit(event, appErr).
    Str("queue", queueName).
    Int("attempt", attempt).
    Msg("publish failed")
```

Always call `WithTrace` first to avoid field collision with `WithErrorKit`.

---

## Lifecycle / Concurrency Notes

- **Global logger** — `Init` sets a package-level `*zerolog.Logger` and `zerolog.log.Logger`. Both are safe for concurrent use after `Init` returns. Do not call `Init` concurrently or after the service has started handling requests.
- **`*OTLPWriter`** — `Write` is called by zerolog on every log event. zerolog guarantees exactly one `Write` call per event with a complete JSON line. `OTLPWriter` itself holds no mutable state and is safe for concurrent use.
- **`LogProviderHandle`** — `Shutdown` should be called exactly once, during process teardown (via `defer`). The underlying `log.LoggerProvider` is safe for concurrent use during the process lifetime.
- **Context loggers** — Loggers stored in a context via `WithLogger` (or any framework adapter that calls `(*zerolog.Logger).WithContext`) are `zerolog.Logger` values copied by value. They are safe for concurrent use.
- **Cleanup** — The `io.Closer` returned by `Init` is always `io.NopCloser(nil)`. The meaningful cleanup is `LogProviderHandle.Shutdown`, which must be deferred when the OTLP path is used.

---

## Dependencies

| Package                                                            | Role                                                                        |
| ------------------------------------------------------------------ | --------------------------------------------------------------------------- |
| `github.com/rs/zerolog`                                            | Structured JSON / console logging engine                                    |
| `go.opentelemetry.io/otel/trace`                                   | Extracts `SpanContext` from `context.Context`                               |
| `go.opentelemetry.io/otel/log`                                     | OTEL `LogRecord` and `LoggerProvider` API                                   |
| `go.opentelemetry.io/otel/log/global`                              | Registers and retrieves the global `LoggerProvider`                         |
| `go.opentelemetry.io/otel/sdk/log`                                 | `log.LoggerProvider`, `NewBatchProcessor`                                   |
| `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc`      | gRPC OTLP log exporter                                                      |
| `go.opentelemetry.io/otel/sdk/resource`                            | Attaches `service.name` semantic attribute to the provider                  |
| `go.opentelemetry.io/otel/semconv/v1.21.0`                         | `semconv.ServiceNameKey` constant                                           |
| `google.golang.org/grpc`                                           | gRPC dial for OTLP export                                                   |
| `github.com/trypanic/go-sdk/errorkit` | Structured error type consumed by `WithErrorKit`, `Error`, `ErrorCtx`, etc. |

The core `logger` package has **no web-framework dependency**. Per-request middleware for specific frameworks lives in the corresponding adapter package (e.g. `httpserver.RequestLoggerMiddleware` for Hertz). Consumers that do not use those frameworks never pull them in.
