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

## Shutdown

`Shutdown(ctx)` delegates to `*server.Hertz.Shutdown(ctx)`. Calling it
before `Run()` returns Hertz's `engine is not running` error verbatim.

## Quick start

```go
import (
    "context"
    "time"

    "github.com/trypanic/go-sdk/httpserver"
    hertzadapter "github.com/trypanic/go-sdk/httpserver/hertz"
)

func main() {
    srv := hertzadapter.New(httpserver.ServerConfig{Host: "0.0.0.0", Port: 8080})

    srv.GET("/hello", func(ctx context.Context, c httpserver.HTTPContext) {
        c.JSON(200, map[string]string{"message": "hello"})
    })

    go func() { _ = srv.Run() }()

    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    _ = srv.Shutdown(shutdownCtx)
}
```
