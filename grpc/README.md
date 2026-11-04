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
| `(*Server).ListenAndServe() error`                                       | Listen on TCP at `ServerConfig.Address` and serve; blocks until stopped.     |
| `(*Server).Serve(lis net.Listener) error`                                | Serve on your own listener (e.g. a bufconn); blocks until stopped.           |
| `(*Server).Shutdown(ctx) error`                                          | Graceful drain of in-flight RPCs/streams; hard-stops on ctx expiry.          |
| `(*Server).Stop()`                                                       | Immediate hard stop.                                                         |
| `Dial(cfg ClientConfig, opts ...ClientOption) (*grpc.ClientConn, error)` | Dial a connection to wrap with a generated client.                           |
| `ToStatus(err error) error`                                              | Map an `*errorkit.AppError` to a gRPC status (code + message + `ErrorInfo`). |
| `ToStatusWithPolicy(err error, p StatusPolicy) error`                    | `ToStatus` with an explicit message policy.                                  |

**Server options:** `WithServerTracing(bool)`, `WithServerRecovery(bool)`,
`WithServerInsecure`, `WithServerTracerProvider`, `WithServerPropagator`, `WithServerLogger`,
`WithUnaryInterceptors`, `WithStreamInterceptors`, `WithRawServerOptions`.

**Client options:** `WithClientTracing(bool)`, `WithClientInsecure`,
`WithClientDefaultTimeout`, `WithClientTracerProvider`,
`WithClientPropagator`, `WithClientUnaryInterceptors`,
`WithClientStreamInterceptors`, `WithRawDialOptions`.

Construction errors are `*errorkit.AppError`: `ERR_SYSTEM_CONFIG_INVALID` for a
missing address/target or missing (or conflicting) transport credentials,
`ERR_SYSTEM_UNEXPECTED` for listen/serve/dial failures (a taken port
included),
`ERR_SYSTEM_TIMEOUT_INTERNAL` when `Shutdown` exceeds its deadline.

### Error mapping

`ToStatus` converts an `*errorkit.AppError` into a gRPC status and surfaces
the errorkit code and error ID as an `errdetails.ErrorInfo` (`Reason` = the
code, `Metadata["id"]` = the error ID, for correlation with server logs).
Return it from your handlers (`return nil, sdkgrpc.ToStatus(err)`).

The status message depends on the class of the error, so internal details
never reach a remote caller:

| Class                                                                                       | Status message                                               |
| ------------------------------------------------------------------------------------------- | ------------------------------------------------------------ |
| Client error (registered `HTTPStatus` < 500, gRPC code not `Internal`/`Unknown`/`DataLoss`) | the errorkit message, `"[CODE] reason"`                      |
| Server error (registered `HTTPStatus` >= 500, or gRPC code `Internal`/`Unknown`/`DataLoss`) | the registered `Metadata.Description`, or `"internal error"` |

The reason of a server error is still in your logs (`logger.ErrorCtx`), keyed
by the same error ID. `ToStatusWithPolicy(err, sdkgrpc.StatusPolicy{ExposeInternalReasons: true})`
restores the pre-2.0 behavior (the reason on the wire for every class); use it
only for trusted internal callers, never on an endpoint reachable by untrusted
clients.

Core `errorkit` codes have an explicit mapping:

| errorkit code                                                                                                                                                              | gRPC code           |
| -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------- |
| `ERR_VALIDATION`, `ERR_VALIDATION_INVALID_FORMAT`, `ERR_VALIDATION_MISSING_FIELD`, `ERR_VALIDATION_INCONSISTENT`, `ERR_VALIDATION_BUSINESS_RULE`, `ERR_CLIENT_BAD_REQUEST` | `InvalidArgument`   |
| `ERR_VALIDATION_DUPLICATE`                                                                                                                                                 | `AlreadyExists`     |
| `ERR_CLIENT_NOT_FOUND`                                                                                                                                                     | `NotFound`          |
| `ERR_CLIENT_RATE_LIMIT`                                                                                                                                                    | `ResourceExhausted` |
| `ERR_SYSTEM_TIMEOUT_INTERNAL`                                                                                                                                              | `DeadlineExceeded`  |
| `ERR_SYSTEM_CONCURRENCY`                                                                                                                                                   | `Aborted`           |
| `ERR_SYSTEM_CONFIG_INVALID`, `ERR_SYSTEM_UNEXPECTED`, `ERR_INTERNAL`, `ERR_UNKNOWN`                                                                                        | `Internal`          |

Matching is exact (there is no prefix matching). Any other code — including
the codes other SDK packages and your service register with
`errorkit.MustRegister` — is mapped from its registered `Metadata.HTTPStatus`,
with `Metadata.Retriable` as the tie-breaker:

| `HTTPStatus`             | gRPC code                                         |
| ------------------------ | ------------------------------------------------- |
| 401                      | `Unauthenticated`                                 |
| 403                      | `PermissionDenied`                                |
| 404                      | `NotFound`                                        |
| 408, 504                 | `DeadlineExceeded`                                |
| 409                      | `Aborted` if retriable, otherwise `AlreadyExists` |
| 429                      | `ResourceExhausted`                               |
| 499                      | `Canceled`                                        |
| 501                      | `Unimplemented`                                   |
| 503                      | `Unavailable`                                     |
| other 4xx                | `InvalidArgument`                                 |
| other 5xx (and the rest) | `Unavailable` if retriable, otherwise `Internal`  |

So `ERR_DB_POSTGRES_UNAVAILABLE` (503, retriable) becomes `Unavailable`, which
gRPC retry policies act on, and `ERR_AUTH_UNAUTHENTICATED` (401) becomes
`Unauthenticated`.

Non-`errorkit` errors: `context.DeadlineExceeded` and `context.Canceled` (also
when wrapped, e.g. `ToStatus(ctx.Err())`) become `codes.DeadlineExceeded` and
`codes.Canceled`; anything else falls back to `status.Convert` (an existing
status passes through; a plain error becomes `codes.Unknown`).

## Defaults

| Setting                                  | Default            | Notes                                                                                                                                                                                                   |
| ---------------------------------------- | ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Tracing                                  | on                 | `otelgrpc` stats handler; instruments **all four** call modes.                                                                                                                                          |
| Recovery                                 | on                 | Unary **and** streaming handlers and the `With{Unary,Stream}Interceptors` interceptors; a panic becomes `codes.Internal`, server stays up. See [Interceptors and recovery](#interceptors-and-recovery). |
| Transport security                       | **required**       | Set `ServerConfig.Creds` / `ClientConfig.Creds`, or opt into plaintext with `With*Insecure()`; see [Transport security](#transport-security).                                                           |
| `MaxConcurrentStreams`                   | unlimited          | Streams per connection; each holds a goroutine. **Set it (e.g. 100–1000) on servers reachable by untrusted clients.**                                                                                   |
| Client call deadline                     | none               | A call whose `ctx` has no deadline waits until the server answers. Set a deadline per call, or `WithClientDefaultTimeout(d)`.                                                                           |
| Keepalive                                | gRPC defaults      | Zero-valued keepalive fields preserve stock gRPC behavior.                                                                                                                                              |
| `MaxConnectionAge` / `MaxConnectionIdle` | **infinity (off)** | Opt-in. Setting them terminates healthy long-lived streams — leave unset unless you want connections recycled.                                                                                          |

> **Observability note:** the stats handler emits traces tied to the injected
> (or global) tracer provider + propagator, and RPC metrics to the global
> OpenTelemetry `MeterProvider` (a no-op unless your app installed one). The SDK
> `telemetry` package covers tracing only.

## Interceptors and recovery

Add server interceptors with `WithUnaryInterceptors` / `WithStreamInterceptors`.
They run in the order given, **inside** the recovery interceptor, so a panic in
one of them becomes `codes.Internal` and the server stays up.

> **Do not** pass `grpc.UnaryInterceptor(...)` or `grpc.StreamInterceptor(...)`
> through `WithRawServerOptions`: grpc-go runs such an interceptor **before**
> the whole chain, outside recovery, so a panic in it crashes the process.

Recovery only covers the handler goroutine. A goroutine your handler starts
(e.g. a separate send loop in a bidi handler) must recover its own panics.

## Transport security

There is no silent plaintext default. `New` and `Dial` return
`ERR_SYSTEM_CONFIG_INVALID` unless exactly one of these is set:

- `ServerConfig.Creds` / `ClientConfig.Creds` — TLS or mTLS (recommended);
- `WithServerInsecure()` / `WithClientInsecure()` — plaintext on purpose.

> **Warning:** with `With*Insecure()`, request bodies and metadata (bearer
> tokens included) travel unencrypted and the server cannot identify its
> peers. Use it only behind a service mesh or sidecar that terminates mTLS,
> or in tests.

Wire TLS through the config (not through `WithRawServerOptions(grpc.Creds(...))`
or `WithRawDialOptions(grpc.WithTransportCredentials(...))`, which the check
above does not see):

```go
import "google.golang.org/grpc/credentials"

serverCreds, err := credentials.NewServerTLSFromFile("server.crt", "server.key")
if err != nil { return err }
srv, err := sdkgrpc.New(sdkgrpc.ServerConfig{Address: ":8443", Creds: serverCreds})

clientCreds, err := credentials.NewClientTLSFromFile("ca.crt", "")
if err != nil { return err }
cc, err := sdkgrpc.Dial(sdkgrpc.ClientConfig{Target: "dns:///payments:8443", Creds: clientCreds})
```

For mTLS, build the credentials with `credentials.NewTLS(&tls.Config{...})`
(`ClientAuth: tls.RequireAndVerifyClientCert` on the server, `Certificates`
on the client) and read the peer identity in handlers with `peer.FromContext`.

## Client deadlines

gRPC calls without a deadline wait until the server answers or the connection
breaks. Give every call a deadline (`context.WithTimeout`), or set a default:

```go
cc, err := sdkgrpc.Dial(cfg, sdkgrpc.WithClientDefaultTimeout(5*time.Second))
```

`WithClientDefaultTimeout(d)` applies `d` to unary and streaming calls whose
`ctx` has **no** deadline; a deadline already on `ctx` is never overridden. It
also applies to streams, so give long-lived streams their own (longer) deadline.

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
    Creds:   serverCreds, // see "Transport security"
    MaxConcurrentStreams: 256, // cap streams per connection
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
    Creds:  clientCreds, // see "Transport security"
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

| Before (Kitex)                                             | After (`google.golang.org/grpc`)                                                                                                                                                                                       |
| ---------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `New(cfg, svcInfo, handler)`                               | `New(cfg, opts...)` + `pb.RegisterXxxServer(srv.Registrar(), impl)`                                                                                                                                                    |
| `NewWithOptions(..., ServerOptions{...})`                  | `New(cfg, sdkgrpc.WithServerTracing(false), ...)`                                                                                                                                                                      |
| `(*Server).Run()`                                          | `(*Server).Serve(lis)`                                                                                                                                                                                                 |
| `(*Server).Stop() error`                                   | `(*Server).Shutdown(ctx) error` (graceful) / `(*Server).Stop()` (hard)                                                                                                                                                 |
| `ServerConfig{Host, Port, ReadWriteTimeout, ExitWaitTime}` | `ServerConfig{Address, Keepalive, MaxRecvMsgSize, MaxSendMsgSize, MaxConcurrentStreams, Creds}`                                                                                                                        |
| `DialOptions(cfg, opts)` / `NewClient(svcInfo, cfg)`       | `Dial(cfg, opts...)` returning `*grpc.ClientConn` + `pb.NewXxxClient(cc)`                                                                                                                                              |
| `ClientConfig{Hosts, RPCTimeout, ConnectTimeout}`          | `ClientConfig{Target, Keepalive, Creds, MaxRecvMsgSize, MaxSendMsgSize}`; `RPCTimeout` → `WithClientDefaultTimeout(d)` or a deadline on each call's `ctx`; `ConnectTimeout` → the call deadline (connections are lazy) |
| Plaintext unless TLS was configured                        | `Creds` required, or explicit `WithServerInsecure()` / `WithClientInsecure()`                                                                                                                                          |
| Stubs from the `kitex` CLI                                 | Stubs from `protoc-gen-go` + `protoc-gen-go-grpc` (or `buf`)                                                                                                                                                           |
