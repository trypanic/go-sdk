# OTLP log export

Package `otlp` owns the optional gRPC log exporter and JSON writer. Import
`github.com/trypanic/go-sdk/logger/otlp` explicitly. Core `logger` has no
exporter dependency. The caller owns provider shutdown; writers do not close
providers. Provider handles and writers are safe for concurrent use.

```go
handle, err := otlp.NewProvider(ctx, otlp.Config{
    ServiceName: "my-service", Endpoint: "collector:4317",
})
if err != nil { return err }
defer handle.Close(ctx)
log := logger.New(logger.Config{Writer: otlp.NewWriter("my-service")})
log.Info("ready")
```

### NewProvider

```go
type Config struct {
    ServiceName string            // required; service.name resource attribute
    Endpoint    string            // required; collector host:port
    Insecure    bool              // plaintext; default is TLS
    TLSConfig   *tls.Config       // custom TLS (private CA, mTLS); exclusive with Insecure
    Headers     map[string]string // sent with every export (auth, API key)
    Timeout     time.Duration     // per-export timeout; 0 = exporter default (10s)
}

func NewProvider(ctx context.Context, cfg Config) (*Provider, error)
```

Creates an OTLP gRPC log exporter for `cfg.Endpoint`, wraps it in a batching `log.LoggerProvider`, and registers it as the global OTEL `LoggerProvider`.

- Transport security: TLS with the host's root CAs by default, `TLSConfig` when set, plaintext only with `Insecure: true`. It is always set explicitly, so the exporter's `OTEL_EXPORTER_OTLP_*` environment fallbacks cannot downgrade it.
- Errors are `*errorkit.AppError`: `ERR_SYSTEM_CONFIG_INVALID` for a missing `ServiceName`/`Endpoint`, `Insecure` together with `TLSConfig`, a negative `Timeout` or an exporter configuration error; `ERR_INTERNAL` otherwise. On error the global provider is unchanged.
- The exporter connects lazily: a wrong or unreachable endpoint is **not** reported at startup. Export failures go to the OTel error handler (`otel.SetErrorHandler`) and the records are dropped.
- Must be called **before** `NewWriter` — `NewWriter` resolves the global provider.
- Calling it again replaces the global provider; `Close` the previous handle yourself.
- `ServiceName` must match the service name of your trace provider so the backend can correlate logs with traces.

```go
handle, err := otlp.NewProvider(ctx, otlp.Config{
    ServiceName: serviceName,
    Endpoint:    "otel-collector:4317",
    Headers:     map[string]string{"authorization": "Bearer " + token},
    Timeout:     5 * time.Second,
})
if err != nil {
    return err
}
defer func() {
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := handle.Close(shutdownCtx); err != nil {
        logger.Error(err, "flush OTLP logs")
    }
}()
```

---

### InitProvider

```go
func InitProvider(ctx context.Context, serviceName, collectorEndpoint string) *Provider
```

Compatibility wrapper: `NewProvider` with `Insecure: true` (plaintext) and no headers or timeout, but without its validation: an empty `collectorEndpoint` uses the exporter default and an empty `serviceName` an empty `service.name`, as in earlier versions. It never returns an error: if the exporter cannot be created it prints to stderr and returns a handle to a provider that exports nothing, and the service keeps running without exported logs. There is no blocking dial: an unreachable collector is not detected at startup. Prefer `NewProvider`.

---

### NewWriter

```go
func NewWriter(serviceName string) *Writer
```

Returns an `*Writer` backed by the global OTEL `LoggerProvider`. Each `Write` call receives one complete JSON log line from zerolog and converts it to an OTEL `LogRecord`:

- Timestamp from the `"time"` field (RFC 3339 with nanoseconds; Unix epoch seconds from older SDK versions are still accepted).
- Numeric fields keep their precision: integers become `Int64` attributes (exact above 2^53), other numbers `Float64`.
- Severity mapped from the `"level"` field.
- Body from `"message"`.
- Trace correlation: `trace_id` and `span_id` fields written by `WithTrace` / `ErrorCtx` are promoted to OTEL first-class fields via a reconstructed `SpanContext` passed to `Emit`.
- All other fields (`error_code`, `reason`, `stack_trace`, etc.) become log attributes shown with the record in your observability backend.

`level`, `time`, `message`, `trace_id`, `span_id`, and `caller` are excluded from the attributes list (they map to dedicated OTEL fields or are omitted for noise reduction).

Must be called after `InitProvider`.

---

### Provider.Close / Shutdown

```go
func (h *Provider) Close(ctx context.Context) error
func (h *Provider) Shutdown(ctx context.Context)
```

`Close` flushes buffered log records, closes the gRPC exporter and returns the error (an `*errorkit.AppError` with code `ERR_INTERNAL` wrapping the cause, so `errors.Is(err, context.DeadlineExceeded)` works). Give `ctx` its own timeout. `Shutdown` does the same and prints the error to stderr instead of returning it. Defer one of them immediately after the provider is created. Both are no-ops on a nil handle.

---
