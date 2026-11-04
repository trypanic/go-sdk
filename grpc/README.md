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
`WithServerInsecure`, `WithServerTracerProvider`, `WithServerPropagator`,
`WithServerPublicEndpoint`, `WithServerOTelOptions`, `WithServerLogger`,
`WithServerErrorDomain`,
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

`ToStatus` converts an `*errorkit.AppError` into a gRPC status with an
`errdetails.ErrorInfo` detail. Return it from your handlers
(`return nil, sdkgrpc.ToStatus(err)`).

| `ErrorInfo` field       | Value                                                                    |
| ----------------------- | ------------------------------------------------------------------------ |
| `Reason`                | the errorkit code, e.g. `ERR_CLIENT_NOT_FOUND`                           |
| `Domain`                | `StatusPolicy.Domain`, or `"go-sdk"` when empty                          |
| `Metadata["id"]`        | the errorkit error ID, the key of the server log entry                   |
| `Metadata["retriable"]` | the registered `Metadata.Retriable`, `"true"` or `"false"`               |

Set the domain to your service's name (usually its DNS name) so callers can
tell which service produced an error: `ToStatusWithPolicy(err,
sdkgrpc.StatusPolicy{Domain: "payments.example.com"})` in handlers, and
`WithServerErrorDomain("payments.example.com")` for recovered panics.

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

## Tracing on public endpoints

By default the server span becomes a child of the caller's `traceparent`,
which is what you want between your own services. On a server reachable by
external clients, pass `WithServerPublicEndpoint()`: each server span then
starts a new trace and only **links** to the caller's span, so callers cannot
choose your trace IDs or, with the usual `ParentBased` sampler, force every
call to be sampled.

```go
srv, err := sdkgrpc.New(cfg, sdkgrpc.WithServerPublicEndpoint())
```

`WithServerOTelOptions(...otelgrpc.Option)` forwards any other `otelgrpc`
server option (e.g. `otelgrpc.WithPublicEndpointFn` to decide per call, or
`otelgrpc.WithFilter`); those options are applied last and take precedence.

## Interceptors and recovery

Add server interceptors with `WithUnaryInterceptors` / `WithStreamInterceptors`.
They run in the order given, **inside** the recovery interceptor, so a panic in
one of them becomes `codes.Internal` and the server stays up.

A recovered panic is logged (panic value and stack) as an
`ERR_SYSTEM_UNEXPECTED` error and answered with `ToStatus` of that error: the
caller gets `codes.Internal`, the registered description (never the panic
value), and the same `ErrorInfo` as any other error, whose `Metadata["id"]`
matches the log entry.

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
if err != nil {
    return err
}
srv, err := sdkgrpc.New(sdkgrpc.ServerConfig{Address: ":8443", Creds: serverCreds})
if err != nil {
    return err
}

clientCreds, err := credentials.NewClientTLSFromFile("ca.crt", "")
if err != nil {
    return err
}
cc, err := sdkgrpc.Dial(sdkgrpc.ClientConfig{Target: "dns:///payments:8443", Creds: clientCreds})
if err != nil {
    return err
}
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

The samples use generated stubs `pb` for an `Echo` service with one method per
call mode (`Unary`, `ServerStream`, `ClientStream`, `Bidi`) and a `pb.Req`
message; replace them with your own.

### Server (all four modes)

```go
import (
    "context"
    "os"
    "os/signal"
    "syscall"
    "time"

    sdkgrpc "github.com/trypanic/go-sdk/grpc"
    pb "your/gen/echo"
)

func run() error {
    srv, err := sdkgrpc.New(sdkgrpc.ServerConfig{
        Address:              "0.0.0.0:8888", // ListenAndServe binds it
        Creds:                serverCreds,    // see "Transport security"
        MaxConcurrentStreams: 256,            // cap streams per connection
        Keepalive: sdkgrpc.ServerKeepalive{
            Time:                30 * time.Second, // ping idle clients
            Timeout:             10 * time.Second, // ack deadline
            MinTime:             10 * time.Second, // reject abusive client pings
            PermitWithoutStream: true,             // allow keepalive on idle conns
        },
    })
    if err != nil {
        return err
    }
    pb.RegisterEchoServer(srv.Registrar(), &EchoImpl{}) // EchoImpl has all 4 methods

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    serveErr := make(chan error, 1)
    go func() { serveErr <- srv.ListenAndServe() }()

    select {
    case err := <-serveErr:
        return err // listen or serve failure (e.g. port in use)
    case <-ctx.Done(): // SIGINT/SIGTERM
    }

    // Drain in-flight calls and streams, but not forever: past the deadline
    // Shutdown hard-stops the server and returns ERR_SYSTEM_TIMEOUT_INTERNAL.
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if err := srv.Shutdown(shutdownCtx); err != nil {
        return err
    }
    return <-serveErr // nil once the server has stopped
}
```

Long-lived streams only end when the client or your handler ends them, so
make handlers return when `stream.Context()` is done.

### Client (all four modes)

```go
import (
    "context"
    "errors"
    "io"
    "time"

    sdkgrpc "github.com/trypanic/go-sdk/grpc"
    pb "your/gen/echo"
)

func run(ctx context.Context) error {
    cc, err := sdkgrpc.Dial(sdkgrpc.ClientConfig{
        Target: "dns:///echo:8888",
        Creds:  clientCreds, // see "Transport security"
        Keepalive: sdkgrpc.ClientKeepalive{
            Time:                30 * time.Second,
            Timeout:             10 * time.Second,
            PermitWithoutStream: true, // keep long-lived idle streams warm
        },
    })
    if err != nil {
        return err
    }
    defer cc.Close()
    client := pb.NewEchoClient(cc)

    // Every call gets a deadline (see "Client deadlines").
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    reqs := []*pb.Req{{}, {}, {}}

    // Unary
    resp, err := client.Unary(ctx, &pb.Req{})
    if err != nil {
        return err
    }
    _ = resp

    // Server-streaming: io.EOF is the normal end; any other error is final,
    // and Recv keeps returning it, so leave the loop.
    ss, err := client.ServerStream(ctx, &pb.Req{})
    if err != nil {
        return err
    }
    for {
        msg, err := ss.Recv()
        if errors.Is(err, io.EOF) {
            break
        }
        if err != nil {
            return err
        }
        _ = msg
    }

    // Client-streaming: Send returns io.EOF when the server ended the stream;
    // CloseAndRecv then returns the server's status.
    cs, err := client.ClientStream(ctx)
    if err != nil {
        return err
    }
    for _, req := range reqs {
        if err := cs.Send(req); err != nil {
            if errors.Is(err, io.EOF) {
                break
            }
            return err
        }
    }
    agg, err := cs.CloseAndRecv()
    if err != nil {
        return err
    }
    _ = agg

    // Bidirectional-streaming (full-duplex): send in a goroutine, receive
    // here, and collect the send result through a channel.
    bs, err := client.Bidi(ctx)
    if err != nil {
        return err
    }
    sendErr := make(chan error, 1)
    go func() {
        for _, req := range reqs {
            if err := bs.Send(req); err != nil {
                if errors.Is(err, io.EOF) {
                    err = nil // the server ended the stream; Recv reports why
                }
                sendErr <- err
                return
            }
        }
        sendErr <- bs.CloseSend()
    }()
    for {
        msg, err := bs.Recv()
        if errors.Is(err, io.EOF) {
            break
        }
        if err != nil {
            return err // cancel (deferred) aborts the stream and frees the sender
        }
        _ = msg
    }
    return <-sendErr
}
```

## Migration from the Kitex-based package

This package previously wrapped CloudWeGo Kitex. It now wraps
`google.golang.org/grpc`. The import path is unchanged; the API changed:

| Before (Kitex)                                             | After (`google.golang.org/grpc`)                                                                                                                                                                                       |
| ---------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `New(cfg, svcInfo, handler)`                               | `New(cfg, opts...)` + `pb.RegisterXxxServer(srv.Registrar(), impl)`                                                                                                                                                    |
| `NewWithOptions(..., ServerOptions{...})`                  | `New(cfg, sdkgrpc.WithServerTracing(false), ...)`                                                                                                                                                                      |
| `(*Server).Run()`                                          | `(*Server).ListenAndServe()` (or `(*Server).Serve(lis)` with your own listener)                                                                                                                                        |
| `(*Server).Stop() error`                                   | `(*Server).Shutdown(ctx) error` (graceful) / `(*Server).Stop()` (hard)                                                                                                                                                 |
| `ServerConfig{Host, Port, ReadWriteTimeout, ExitWaitTime}` | `ServerConfig{Address, Keepalive, MaxRecvMsgSize, MaxSendMsgSize, MaxConcurrentStreams, Creds}`                                                                                                                        |
| `DialOptions(cfg, opts)` / `NewClient(svcInfo, cfg)`       | `Dial(cfg, opts...)` returning `*grpc.ClientConn` + `pb.NewXxxClient(cc)`                                                                                                                                              |
| `ClientConfig{Hosts, RPCTimeout, ConnectTimeout}`          | `ClientConfig{Target, Keepalive, Creds, MaxRecvMsgSize, MaxSendMsgSize}`; `RPCTimeout` → `WithClientDefaultTimeout(d)` or a deadline on each call's `ctx`; `ConnectTimeout` → the call deadline (connections are lazy) |
| Plaintext unless TLS was configured                        | `Creds` required, or explicit `WithServerInsecure()` / `WithClientInsecure()`                                                                                                                                          |
| Stubs from the `kitex` CLI                                 | Stubs from `protoc-gen-go` + `protoc-gen-go-grpc` (or `buf`)                                                                                                                                                           |

The OTLP/gRPC dependency can register `x/net/trace` handlers on
`http.DefaultServeMux`. Services must use a dedicated mux for public HTTP
listeners rather than expose the process default mux.

Tracing injection accepts a shared `*telemetry.Instrumenter`. Use
`grpc.WithServerInstrumenter` / `grpc.WithClientInstrumenter` or
`messaging.WithInstrumenter`; each keeps its component scope and uses
`instr.TracerProvider()`. A nil instrumenter selects a no-op provider.
Hertz tracing remains an exception: its adapter uses the global provider.

### Receiving remote errors

`FromStatus(err)` converts a received gRPC status into an `AppError` and keeps
the original status as its wrapped cause. A known `ErrorInfo.Reason` restores
the registered local code when its gRPC class matches. Remote retry flags,
messages, IDs and domains never override local classification or public text.
Unknown or inconsistent details fall back to this mapping:

| gRPC status | Local code |
| --- | --- |
| InvalidArgument | ERR_VALIDATION |
| NotFound | ERR_CLIENT_NOT_FOUND |
| AlreadyExists | ERR_VALIDATION_DUPLICATE |
| ResourceExhausted | ERR_CLIENT_RATE_LIMIT |
| Aborted | ERR_SYSTEM_CONCURRENCY |
| DeadlineExceeded | ERR_EXTERNAL_SERVICE_TIMEOUT |
| Unavailable | ERR_EXTERNAL_SERVICE_UNAVAILABLE |
| Internal, Unknown, DataLoss | ERR_EXTERNAL_SERVICE_ERROR |
| Other statuses, including Canceled | ERR_EXTERNAL_INVALID_RESPONSE (nonretriable) |

Nil, existing `AppError` values and non-status errors pass through unchanged.
`ToStatus` sends canonical context cancellation/deadline messages even when
those errors are wrapped with private diagnostic text.
