# httpserver/hertz

Hertz adapter for `httpserver`. Implements the framework-agnostic
`httpserver.HTTPServer`, `RouterGroup`, and `HTTPContext` contracts on
top of [CloudWeGo Hertz](https://github.com/cloudwego/hertz).

Importing this package transitively pulls Hertz, OTel-Hertz tracing, and
related dependencies into your binary. The core `httpserver` package
does not.

## API

| Symbol | Purpose |
|---|---|
| `New(cfg httpserver.ServerConfig) httpserver.HTTPServer` | Hertz-backed server with `httpserver.DefaultServerOptions()`. |
| `NewWithOptions(cfg, opts httpserver.ServerOptions) httpserver.HTTPServer` | Same with explicit options; zero-valued fields are backfilled from defaults. |
| `RecoveryHandler(ctx, c, err, stack)` | Panic-recovery handler installed when `EnableRecovery` is true. Logs via the SDK logger and aborts with HTTP 500. |
| `RequestLoggerMiddleware(base *logger.Logger) app.HandlerFunc` | Injects a per-request logger into ctx. Pass `nil` for `base` to derive from the global logger. |

## Defaults from `DefaultServerOptions()`

- Tracing, recovery, audit, `/health`, NoRoute, NoMethod all enabled.
- Audit body capture OFF; non-empty bodies redacted to `[REDACTED]`.
- Unknown route → HTTP 404. Unknown method → HTTP 405.
- Reply timestamp: `time.Now().Format("2006-01-02 15:04:05")`.

Override any of these by constructing your own `httpserver.ServerOptions`
and calling `NewWithOptions`.

## Run

`Run()` calls the Hertz engine directly (not `Spin`), so it installs no
SIGINT/SIGHUP/SIGTERM handlers. It returns `nil` after `Shutdown`, and an
`*errorkit.AppError` with code `ERR_SYSTEM_CONFIG_INVALID` when it cannot
listen (address in use, permission denied) instead of panicking.

## Shutdown

`Shutdown(ctx)` stops accepting connections and waits until no request
is in flight or `ctx` is done. Only `ctx` bounds the wait (Hertz's own
5 s `ExitWaitTimeout` is lifted), and idle keep-alive connections do not
delay it. If `ctx` ends while requests are still in flight, it returns
`ERR_SYSTEM_TIMEOUT_INTERNAL`, so the caller knows the drain is
incomplete before closing databases or exporters. Calling it before
`Run()` returns Hertz's `engine is not running` error verbatim.

## Quick start

```go
import (
    "context"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/trypanic/go-sdk/httpserver"
    hertzadapter "github.com/trypanic/go-sdk/httpserver/hertz"
    "github.com/trypanic/go-sdk/logger"
)

func main() {
    srv := hertzadapter.New(httpserver.ServerConfig{Host: "0.0.0.0", Port: 8080})

    srv.GET("/hello", func(ctx context.Context, c httpserver.HTTPContext) {
        c.JSON(200, map[string]string{"message": "hello"})
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
