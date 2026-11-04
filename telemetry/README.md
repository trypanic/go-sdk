# telemetry

`telemetry` centralizes OpenTelemetry span creation and naming helpers.
`Span` is an alias of `trace.Span`, so callers rarely need to import
`go.opentelemetry.io/otel/trace` themselves.

## Explicit Instrumenter

SDK consumers should prefer `Instrumenter` when they need telemetry without mutating package-global state.

```go
provider := otel.GetTracerProvider()

instrumenter := telemetry.NewInstrumenter(telemetry.InstrumenterConfig{
    ScopeName:      "my-sdk-consumer",
    TracerProvider: provider,
})

ctx, span := instrumenter.Start(ctx, "operation.name")
defer span.End()
```

If `TracerProvider` is nil, `NewInstrumenter` uses the current global OpenTelemetry provider. If `ScopeName` is empty, it uses `"app"`.

## Naming Helpers

The same naming helpers are available on an `Instrumenter` instance:

```go
ctx, span := instrumenter.Job(ctx, "import")
ctx, span := instrumenter.Batch(ctx, "process")
ctx, span := instrumenter.External(ctx, "vendor", "fetch")
ctx, span := instrumenter.Messaging(ctx, "publish.queue")
ctx, span := instrumenter.DB(ctx, "postgres", "jobs", "insert")
```

They create these span names:

| Helper | Span name |
|---|---|
| `Job(ctx, "import")` | `job.import` |
| `Batch(ctx, "process")` | `batch.process` |
| `External(ctx, "vendor", "fetch")` | `external.vendor.fetch` |
| `Messaging(ctx, "publish.queue")` | `messaging.publish.queue` |
| `DB(ctx, "postgres", "jobs", "insert")` | `postgres.jobs.insert` |

## Compatibility Globals

The package-level helpers remain available for existing callers:

```go
telemetry.InitTracer("scope-name")

ctx, span := telemetry.Start(ctx, "operation.name")
ctx, span := telemetry.Job(ctx, "import")
ctx, span := telemetry.Batch(ctx, "process")
ctx, span := telemetry.External(ctx, "vendor", "fetch")
ctx, span := telemetry.Messaging(ctx, "publish.queue")
ctx, span := telemetry.DB(ctx, "postgres", "jobs", "insert")
```

These wrappers use a package-global tracer. They are compatibility APIs; new SDK-facing code should pass an explicit `Instrumenter`.

## Span Helpers

Use these helpers to keep OTel attribute and status handling consistent:

```go
telemetry.SetAttrString(span, "job.id", jobID)
telemetry.SetAttrInt(span, "attempt", attempt)
telemetry.SetAttrInt64(span, "duration_ms", duration)
telemetry.SetAttrStringSlice(span, "sources", sources)
telemetry.RecordError(span, err, "operation failed")
```

Span creation options:

```go
ctx, span := instrumenter.Messaging(
    ctx,
    "publish.queue",
    telemetry.WithSpanKind(trace.SpanKindProducer),
    telemetry.WithString("queue", "queue-name"),
)
```

`WithString`, `WithInt`, `WithBool` and `WithSpanKind` are span start
options; the `SetAttr*` helpers set attributes on a running span;
`RecordError` records the error and sets the span status to `Error` in one
call (a nil error is a no-op).

## Naming Rules

- Span names describe operations, never the service: the service name and
  version belong in the TracerProvider's Resource attributes.
- Pass bounded values (operation and resource names), never IDs or other
  per-request values, so the number of distinct span names stays small.
- `example_test.go` holds runnable examples (`go doc -all`, pkg.go.dev).

## Concurrency

`Instrumenter` values and the package-level helpers are safe for concurrent
use. `InitTracer` replaces the compatibility instrumenter atomically and may
run concurrently with span creation, but it is meant to be called once at
startup.

Tracing injection accepts a shared `*telemetry.Instrumenter`. Use
`grpc.WithServerInstrumenter` / `grpc.WithClientInstrumenter` or
`messaging.WithInstrumenter`; each keeps its component scope and uses
`instr.TracerProvider()`. A nil instrumenter selects a no-op provider.
Hertz tracing remains an exception: its adapter uses the global provider.
