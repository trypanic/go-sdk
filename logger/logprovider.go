package logger

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"google.golang.org/grpc/credentials"

	"github.com/trypanic/go-sdk/errorkit"
)

// LogProviderHandle wraps the SDK LoggerProvider so main can defer a clean
// shutdown without importing the SDK directly.
type LogProviderHandle struct {
	provider *log.LoggerProvider
}

// Close flushes any buffered log records, closes the exporter and returns
// the error, if any, as an *errorkit.AppError (ERR_INTERNAL wrapping the
// cause, so errors.Is(err, context.DeadlineExceeded) works). Give ctx its own
// timeout: Close blocks until the flush finishes or ctx is done. A nil
// handle is a no-op.
func (h *LogProviderHandle) Close(ctx context.Context) error {
	if h == nil || h.provider == nil {
		return nil
	}
	if err := h.provider.Shutdown(ctx); err != nil {
		return errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("logger: shut down OTLP log provider"),
			errorkit.WithWrapped(err),
		)
	}
	return nil
}

// Shutdown is Close for callers that cannot handle the error: it prints the
// error to stderr. Prefer Close.
func (h *LogProviderHandle) Shutdown(ctx context.Context) {
	if err := h.Close(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "logger: LoggerProvider shutdown error: %v\n", err)
	}
}

// LogProviderConfig configures NewLogProvider.
type LogProviderConfig struct {
	// ServiceName is stamped on every record as the service.name resource
	// attribute. Use the same value as your trace provider so backends
	// correlate logs with traces. Required.
	ServiceName string

	// Endpoint is the collector's gRPC address, host:port (for example
	// "otel-collector:4317"). Required.
	Endpoint string

	// Insecure sends records in plaintext. Leave it false in production:
	// the default is TLS with TLSConfig, or with the host's root CAs when
	// TLSConfig is nil. Insecure and TLSConfig are mutually exclusive.
	Insecure bool

	// TLSConfig customises the TLS connection (private CA, client
	// certificates, server name).
	TLSConfig *tls.Config

	// Headers are sent with every export request, for example an
	// authorization or API key header expected by the collector.
	Headers map[string]string

	// Timeout bounds each export request. Zero uses the exporter default
	// (10s); negative values are rejected.
	Timeout time.Duration
}

func (cfg LogProviderConfig) validate() error {
	var reason string
	switch {
	case cfg.ServiceName == "":
		reason = "ServiceName is required"
	case cfg.Endpoint == "":
		reason = "Endpoint is required"
	case cfg.Insecure && cfg.TLSConfig != nil:
		reason = "Insecure and TLSConfig are mutually exclusive"
	case cfg.Timeout < 0:
		reason = "Timeout must not be negative"
	default:
		return nil
	}
	return errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
		errorkit.WithReason("logger: invalid LogProviderConfig: %s", reason),
	)
}

func (cfg LogProviderConfig) exporterOptions() []otlploggrpc.Option {
	var opts []otlploggrpc.Option
	if cfg.Endpoint != "" {
		opts = append(opts, otlploggrpc.WithEndpoint(cfg.Endpoint))
	}
	// Transport security is always set explicitly so the exporter's
	// OTEL_EXPORTER_OTLP_* environment fallbacks cannot change it.
	if cfg.Insecure {
		opts = append(opts, otlploggrpc.WithInsecure())
	} else {
		opts = append(opts, otlploggrpc.WithTLSCredentials(credentials.NewTLS(cfg.TLSConfig)))
	}
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlploggrpc.WithHeaders(cfg.Headers))
	}
	if cfg.Timeout > 0 {
		opts = append(opts, otlploggrpc.WithTimeout(cfg.Timeout))
	}
	return opts
}

// NewLogProvider creates an OTLP/gRPC LoggerProvider from cfg, registers it
// as the global OTel LoggerProvider (so NewOTLPWriter resolves it) and
// returns its handle. Defer handle.Close in main.
//
// Errors are *errorkit.AppError: ERR_SYSTEM_CONFIG_INVALID for an invalid
// cfg or exporter configuration, ERR_INTERNAL otherwise. On error the global
// provider is left unchanged.
//
// The exporter connects lazily: an unreachable collector is not reported
// here; export failures go to the OTel error handler (otel.SetErrorHandler)
// and the records are dropped. Calling NewLogProvider again replaces the
// global provider; Close the previous handle yourself.
func NewLogProvider(ctx context.Context, cfg LogProviderConfig) (*LogProviderHandle, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return newLogProvider(ctx, cfg, true)
}

// newLogProvider builds and registers the provider. With strict false (the
// InitLogProvider compatibility path) cfg is not validated and a resource
// error is printed to stderr and tolerated, as InitLogProvider always did:
// an empty endpoint uses the exporter default and an empty service name
// yields an empty service.name.
func newLogProvider(ctx context.Context, cfg LogProviderConfig, strict bool) (*LogProviderHandle, error) {
	exporter, err := otlploggrpc.New(ctx, cfg.exporterOptions()...)
	if err != nil {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("logger: create OTLP log exporter"),
			errorkit.WithWrapped(err),
		)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
		),
	)
	if err != nil && !strict {
		fmt.Fprintf(os.Stderr, "logger: could not create OTel resource: %v — using the partial resource\n", err)
	} else if err != nil {
		_ = exporter.Shutdown(ctx)
		return nil, errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("logger: create OTel resource"),
			errorkit.WithWrapped(err),
		)
	}

	provider := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(exporter)),
		log.WithResource(res),
	)

	// Register as the global provider so NewOTLPWriter() can resolve it.
	otel.SetLoggerProvider(provider)

	return &LogProviderHandle{provider: provider}, nil
}

// InitLogProvider is NewLogProvider with a plaintext (Insecure) connection
// and no headers or timeout, kept for compatibility. It never fails and does
// not validate its arguments: an empty collectorEndpoint uses the exporter
// default and an empty serviceName an empty service.name, as before
// NewLogProvider existed. Only if the exporter cannot be created at all does
// it print the error to stderr and return a handle to a provider that exports
// nothing. Prefer NewLogProvider, which supports TLS and returns errors.
//
// Call it BEFORE logger.Init so that NewOTLPWriter can resolve the global
// provider. Defer Shutdown (or Close) to flush on exit.
//
// Usage:
//
//	lp := logger.InitLogProvider(ctx, "my-service", "otel-collector:4317")
//	defer lp.Shutdown(ctx)
//	_, cleanup := logger.Init("my-service", version, logger.NewOTLPWriter("my-service"))
//	defer cleanup.Close()
func InitLogProvider(ctx context.Context, serviceName, collectorEndpoint string) *LogProviderHandle {
	handle, err := newLogProvider(ctx, LogProviderConfig{
		ServiceName: serviceName,
		Endpoint:    collectorEndpoint,
		Insecure:    true,
	}, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: could not create OTLP log provider: %v — logs will not be exported\n", err)
		return &LogProviderHandle{provider: log.NewLoggerProvider()}
	}
	return handle
}

// SetupOTLP bundles the OTLP boilerplate so callers cannot easily get the
// order wrong. It calls InitLogProvider, then constructs an OTLPWriter
// against that provider. Returns the handle (caller must defer Shutdown)
// and the writer ready to pass to logger.Init or logger.New.
//
// Like InitLogProvider it uses a plaintext connection and reports setup
// errors only on stderr; for TLS, headers or errors use NewLogProvider
// followed by NewOTLPWriter.
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
