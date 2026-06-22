# grpc

Reusable server and client factories for **gRPC**, built on
[`google.golang.org/grpc`](https://pkg.go.dev/google.golang.org/grpc) and wired
with the SDK's standard cross-cutting concerns: OpenTelemetry tracing (via the
[`otelgrpc`](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc)
stats handler), panic recovery, keepalive, graceful lifecycle, and `errorkit`
error mapping. Independent package — does **not** build on `httpserver/hertz`.

All four call modes — **unary, server-streaming, client-streaming, and
bidirectional-streaming** — get the same wiring. You register your generated
service stubs and the cross-cutting concerns apply to every method.

Import path is `github.com/trypanic/go-sdk/grpc`; the package identifier is
`grpc`, which collides with upstream `google.golang.org/grpc`. **Alias it** when
you import both:

```go
import sdkgrpc "github.com/trypanic/go-sdk/grpc"
```

## Codegen

This package does **not** generate code. Generate your service stubs with the
ecosystem-standard Go toolchain — [`protoc-gen-go`](https://pkg.go.dev/google.golang.org/protobuf/cmd/protoc-gen-go)
+ [`protoc-gen-go-grpc`](https://pkg.go.dev/google.golang.org/grpc/cmd/protoc-gen-go-grpc),
driven by `protoc` or [`buf`](https://buf.build/). **No bespoke or non-Go binary
is required** beyond the standard protoc toolchain. A minimal `buf.gen.yaml`:

```yaml
version: v2
plugins:
  - remote: buf.build/protocolbuffers/go
    out: gen
    opt: paths=source_relative
  - remote: buf.build/grpc/go
    out: gen
    opt: paths=source_relative
```

Then register the generated server (`pb.RegisterEchoServer`) on `Registrar()`
and wrap the dialed connection with the generated client (`pb.NewEchoClient`).

## API

| Symbol                                                                   | Purpose                                                                      |
| ------------------------------------------------------------------------ | ---------------------------------------------------------------------------- |
| `New(cfg ServerConfig, opts ...ServerOption) (*Server, error)`           | Build a gRPC server (tracing + recovery on by default).                      |
| `(*Server).Registrar() grpc.ServiceRegistrar`                            | Register generated services: `pb.RegisterEchoServer(srv.Registrar(), impl)`. |
| `(*Server).Serve(lis net.Listener) error`                                | Serve on a listener; blocks until stopped.                                   |
| `(*Server).Shutdown(ctx) error`                                          | Graceful drain of in-flight RPCs/streams; hard-stops on ctx expiry.          |
| `(*Server).Stop()`                                                       | Immediate hard stop.                                                         |
| `Dial(cfg ClientConfig, opts ...ClientOption) (*grpc.ClientConn, error)` | Dial a connection to wrap with a generated client.                           |
| `ToStatus(err error) error`                                              | Map an `*errorkit.AppError` to a gRPC status (code + message + `ErrorInfo`). |

**Server options:** `WithServerTracing(bool)`, `WithServerRecovery(bool)`,
`WithServerTracerProvider`, `WithServerPropagator`, `WithServerLogger`,
`WithUnaryInterceptors`, `WithStreamInterceptors`, `WithRawServerOptions`.

**Client options:** `WithClientTracing(bool)`, `WithClientTracerProvider`,
`WithClientPropagator`, `WithClientUnaryInterceptors`,
`WithClientStreamInterceptors`, `WithRawDialOptions`.

Construction errors are `*errorkit.AppError`: `ERR_SYSTEM_CONFIG_INVALID` for a
missing address/target, `ERR_SYSTEM_UNEXPECTED` for serve/dial failures,
`ERR_SYSTEM_TIMEOUT_INTERNAL` when `Shutdown` exceeds its deadline.

### Error mapping

`ToStatus` converts an `*errorkit.AppError` into a gRPC status, preserving the
message and surfacing the errorkit code as an `errdetails.ErrorInfo` (Reason =
the code). Return it from your handlers (`return nil, sdkgrpc.ToStatus(err)`).

| errorkit code                                                                       | gRPC code           |
| ----------------------------------------------------------------------------------- | ------------------- |
| `ERR_VALIDATION*`, `ERR_CLIENT_BAD_REQUEST`                                         | `InvalidArgument`   |
| `ERR_VALIDATION_DUPLICATE`                                                          | `AlreadyExists`     |
| `ERR_CLIENT_NOT_FOUND`                                                              | `NotFound`          |
| `ERR_CLIENT_RATE_LIMIT`                                                             | `ResourceExhausted` |
| `ERR_SYSTEM_TIMEOUT_INTERNAL`                                                       | `DeadlineExceeded`  |
| `ERR_SYSTEM_CONCURRENCY`                                                            | `Aborted`           |
| `ERR_SYSTEM_CONFIG_INVALID`, `ERR_SYSTEM_UNEXPECTED`, `ERR_INTERNAL`, `ERR_UNKNOWN` | `Internal`          |

Non-`errorkit` errors fall back to `status.Convert` (an existing status passes
through; a plain error becomes `codes.Unknown`).

## Defaults

| Setting                                  | Default            | Notes                                                                                                          |
| ---------------------------------------- | ------------------ | -------------------------------------------------------------------------------------------------------------- |
| Tracing                                  | on                 | `otelgrpc` stats handler; instruments **all four** call modes.                                                 |
| Recovery                                 | on                 | Unary **and** streaming handlers; a panic becomes `codes.Internal`, server stays up.                           |
| Transport security                       | insecure           | Set `ClientConfig.Creds` (or `WithRawServerOptions(grpc.Creds(...))`) to wire TLS.                             |
| Keepalive                                | gRPC defaults      | Zero-valued keepalive fields preserve stock gRPC behavior.                                                     |
| `MaxConnectionAge` / `MaxConnectionIdle` | **infinity (off)** | Opt-in. Setting them terminates healthy long-lived streams — leave unset unless you want connections recycled. |

> **Observability note:** the stats handler emits traces tied to the injected
> (or global) tracer provider + propagator, and RPC metrics to the global
> OpenTelemetry `MeterProvider` (a no-op unless your app installed one). The SDK
> `telemetry` package covers tracing only.

## Quick start

### Server (all four modes)

```go
import (
    "net"
    sdkgrpc "github.com/trypanic/go-sdk/grpc"
    pb "your/gen/echo"
)

srv, err := sdkgrpc.New(sdkgrpc.ServerConfig{
    Address: "0.0.0.0:8888",
    Keepalive: sdkgrpc.ServerKeepalive{
        Time:                30 * time.Second, // ping idle clients
        Timeout:             10 * time.Second, // ack deadline
        MinTime:             10 * time.Second, // reject abusive client pings
        PermitWithoutStream: true,             // allow keepalive on idle conns
    },
})
if err != nil { panic(err) }

pb.RegisterEchoServer(srv.Registrar(), &EchoImpl{}) // EchoImpl has all 4 methods

lis, err := net.Listen("tcp", "0.0.0.0:8888")
if err != nil { panic(err) }
go srv.Serve(lis)
// ...on shutdown signal:
_ = srv.Shutdown(context.Background()) // drains in-flight streams
```

### Client (all four modes)

```go
cc, err := sdkgrpc.Dial(sdkgrpc.ClientConfig{
    Target: "127.0.0.1:8888",
    Keepalive: sdkgrpc.ClientKeepalive{
        Time:                30 * time.Second,
        Timeout:             10 * time.Second,
        PermitWithoutStream: true, // keep long-lived idle streams warm
    },
})
if err != nil { panic(err) }
defer cc.Close()

client := pb.NewEchoClient(cc)

// Unary
resp, _ := client.Unary(ctx, &pb.Req{})

// Server-streaming
ss, _ := client.ServerStream(ctx, &pb.Req{})
for { msg, err := ss.Recv(); if err == io.EOF { break }; _ = msg }

// Client-streaming
cs, _ := client.ClientStream(ctx)
cs.Send(&pb.Req{}); agg, _ := cs.CloseAndRecv()

// Bidirectional-streaming (full-duplex: send and recv run independently)
bs, _ := client.Bidi(ctx)
go func() { bs.Send(&pb.Req{}); bs.CloseSend() }()
for { msg, err := bs.Recv(); if err == io.EOF { break }; _ = msg }
```

## Migration from the Kitex-based package

This package previously wrapped CloudWeGo Kitex. It now wraps
`google.golang.org/grpc`. The import path is unchanged; the API changed:

| Before (Kitex)                                             | After (`google.golang.org/grpc`)                                          |
| ---------------------------------------------------------- | ------------------------------------------------------------------------- |
| `New(cfg, svcInfo, handler)`                               | `New(cfg, opts...)` + `pb.RegisterXxxServer(srv.Registrar(), impl)`       |
| `NewWithOptions(..., ServerOptions{...})`                  | `New(cfg, sdkgrpc.WithServerTracing(false), ...)`                         |
| `(*Server).Run()`                                          | `(*Server).Serve(lis)`                                                    |
| `(*Server).Stop() error`                                   | `(*Server).Shutdown(ctx) error` (graceful) / `(*Server).Stop()` (hard)    |
| `ServerConfig{Host, Port, ReadWriteTimeout, ExitWaitTime}` | `ServerConfig{Address, Keepalive, MaxRecvMsgSize, MaxSendMsgSize}`        |
| `DialOptions(cfg, opts)` / `NewClient(svcInfo, cfg)`       | `Dial(cfg, opts...)` returning `*grpc.ClientConn` + `pb.NewXxxClient(cc)` |
| `ClientConfig{Hosts, RPCTimeout, ConnectTimeout}`          | `ClientConfig{Target, Keepalive, Creds, MaxRecvMsgSize, MaxSendMsgSize}`  |
| Stubs from the `kitex` CLI                                 | Stubs from `protoc-gen-go` + `protoc-gen-go-grpc` (or `buf`)              |
