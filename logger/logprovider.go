package logger

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otelglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// LogProviderHandle wraps the SDK LoggerProvider so main.go can defer
// a clean shutdown without importing the SDK directly.
type LogProviderHandle struct {
	provider *log.LoggerProvider
}

// Shutdown flushes any buffered log records and closes the exporter.
// Always defer this in main.go immediately after InitLogProvider returns.
func (h *LogProviderHandle) Shutdown(ctx context.Context) {
	if err := h.provider.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "logger: LoggerProvider shutdown error: %v\n", err)
	}
}

// InitLogProvider creates and registers a global OTLP LoggerProvider that
// exports log records to the OTEL Collector over gRPC (port 4317).
//
// Call this BEFORE logger.Init() so that NewOTLPWriter can resolve the
// global provider. Defer Shutdown to flush on exit.
//
// The serviceName MUST match the value passed to provider.WithServiceName()
// for your trace provider — SigNoz uses it to correlate logs with traces.
//
// Usage in main.go:
//
//	func main() {
//	    ctx := context.Background()
//
//	    // 1. Trace provider (existing)
//	    p := provider.NewOpenTelemetryProvider(
//	        provider.WithServiceName(serviceName),
//	        provider.WithExportEndpoint("otel-collector:4317"),
//	        provider.WithInsecure(),
//	    )
//	    defer p.Shutdown(ctx)
//
//	    // 2. Log provider (new)
//	    lp := logger.InitLogProvider(ctx, serviceName, "otel-collector:4317")
//	    defer lp.Shutdown(ctx)
//
//	    // 3. Logger with OTLP writer (new — was just Init(serviceName, version))
//	    _, cleanup := logger.Init(serviceName, version, logger.NewOTLPWriter(serviceName))
//	    defer cleanup.Close()
//
//	    // ... rest of main unchanged
//	}
func InitLogProvider(ctx context.Context, serviceName, collectorEndpoint string) *LogProviderHandle {
	exporter, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint(collectorEndpoint),
		otlploggrpc.WithInsecure(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: could not create OTLP log exporter: %v — logs will not be exported\n", err)
		return &LogProviderHandle{provider: log.NewLoggerProvider()}
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: could not create OTel resource: %v — using default resource\n", err)
	}

	provider := log.NewLoggerProvider(
		// WithBatcher does not exist in the SDK — the correct API is
		// WithProcessor wrapping a NewBatchProcessor.
		log.WithProcessor(log.NewBatchProcessor(exporter)),
		log.WithResource(res),
	)

	// Register as the global provider so NewOTLPWriter() can resolve it.
	otelglobal.SetLoggerProvider(provider)

	return &LogProviderHandle{provider: provider}
}

// SetupOTLP bundles the OTLP boilerplate so callers cannot easily get the
// order wrong. It calls InitLogProvider, then constructs an OTLPWriter
// against that provider. Returns the handle (caller must defer Shutdown)
// and the writer ready to pass to logger.Init or logger.New.
//
// Usage:
//
//	handle, writer := logger.SetupOTLP(ctx, "my-service", "otel-collector:4317")
//	defer handle.Shutdown(ctx)
//	_, cleanup := logger.Init("my-service", "1.0.0", writer)
//	defer cleanup.Close()
func SetupOTLP(ctx context.Context, serviceName, collectorEndpoint string) (*LogProviderHandle, *OTLPWriter) {
	handle := InitLogProvider(ctx, serviceName, collectorEndpoint)
	return handle, NewOTLPWriter(serviceName)
}
