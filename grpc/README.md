# grpc

Server and client factories for **gRPC**, backed by
[CloudWeGo Kitex](https://www.cloudwego.io/docs/kitex/) and wired with the
SDK's tracing (`kitex-contrib/obs-opentelemetry`), panic recovery, and
`errorkit` conventions. Independent package — does **not** build on
`httpserver/hertz`.

Import path is `github.com/trypanic/go-sdk/grpc`; package identifier is
`grpc`. Alias it (e.g. `sdkgrpc`) if you also import `google.golang.org/grpc`.

Importing this package pulls the Kitex runtime and OTel-Kitex tracing into
your binary.

## Codegen first

Kitex is IDL/codegen-bound. Generate your service with the `kitex` tool from a
`.proto` (gRPC) IDL, then pass the generated `*serviceinfo.ServiceInfo` and
your handler implementation to `New`. This package standardizes construction;
it does not generate code.

## API

| Symbol | Purpose |
|---|---|
| `New(cfg ServerConfig, svcInfo, handler) (*Server, error)` | gRPC server with `DefaultServerOptions()` (tracing + recovery). |
| `NewWithOptions(cfg, svcInfo, handler, opts ServerOptions)` | Same with explicit toggles. |
| `(*Server).Run() error` | Start and block until stopped/failed. |
| `(*Server).Stop() error` | Graceful drain (bounded by `ServerConfig.ExitWaitTime`). |
| `DialOptions(cfg ClientConfig, opts ClientOptions) []client.Option` | gRPC client options — feed to a codegen-typed client. |
| `NewClient(svcInfo, cfg ClientConfig) (client.Client, error)` | Generic client with `DefaultClientOptions()`. |
| `NewClientWithOptions(svcInfo, cfg, opts)` | Same with explicit toggles. |

Errors are `*errorkit.AppError`: `ERR_SYSTEM_CONFIG_INVALID` for bad
config/address, `ERR_SYSTEM_UNEXPECTED` for construction/run failures.

## Defaults

- `DefaultServerOptions()`: tracing on, recovery on.
- `DefaultClientOptions()`: tracing on.
- gRPC: server auto-detects gRPC traffic for protobuf services; the client
  sets `transport.GRPC` explicitly.

## Quick start

Server:

```go
import (
    sdkgrpc "github.com/trypanic/go-sdk/grpc"
    "your/gen/echo" // kitex-generated package
)

func main() {
    srv, err := sdkgrpc.New(
        sdkgrpc.ServerConfig{Host: "0.0.0.0", Port: 8888},
        echo.NewServiceInfo(),
        &EchoImpl{},
    )
    if err != nil { panic(err) }
    _ = srv.Run() // blocks; call srv.Stop() to drain
}
```

Client (codegen-typed — the common case):

```go
cfg := sdkgrpc.ClientConfig{Hosts: []string{"127.0.0.1:8888"}}
cli, err := echo.NewClient("echo", sdkgrpc.DialOptions(cfg, sdkgrpc.DefaultClientOptions())...)
```
