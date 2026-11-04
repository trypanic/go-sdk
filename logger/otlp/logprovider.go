package otlp

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

	"github.com/trypanic/go-sdk/v2/errorkit"
)

// Provider wraps the SDK LoggerProvider so main can defer a clean
// shutdown without importing the SDK directly.
type Provider struct {
	provider *log.LoggerProvider
}

// Close flushes any buffered log records, closes the exporter and returns
// the error, if any, as an *errorkit.AppError (ERR_INTERNAL wrapping the
// cause, so errors.Is(err, context.DeadlineExceeded) works). Give ctx its own
// timeout: Close blocks until the flush finishes or ctx is done. A nil
// handle is a no-op.
func (h *Provider) Close(ctx context.Context) error {
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
func (h *Provider) Shutdown(ctx context.Context) {
	if err := h.Close(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "logger: LoggerProvider shutdown error: %v\n", err)
	}
}

// Config configures NewProvider.
type Config struct {
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

func (cfg Config) validate() error {
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
		errorkit.WithReason("logger: invalid Config: %s", reason),
	)
}

func (cfg Config) exporterOptions() []otlploggrpc.Option {
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

// NewProvider creates an OTLP/gRPC LoggerProvider from cfg, registers it
// as the global OTel LoggerProvider (so NewWriter resolves it) and
// returns its handle. Defer handle.Close in main.
//
// Errors are *errorkit.AppError: ERR_SYSTEM_CONFIG_INVALID for an invalid
// cfg or exporter configuration, ERR_INTERNAL otherwise. On error the global
// provider is left unchanged.
//
// The exporter connects lazily: an unreachable collector is not reported
// here; export failures go to the OTel error handler (otel.SetErrorHandler)
// and the records are dropped. Calling NewProvider again replaces the
// global provider; Close the previous handle yourself.
func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return newLogProvider(ctx, cfg, true)
}

// newLogProvider builds and registers the provider. With strict false (the
// InitProvider compatibility path) cfg is not validated and a resource
// error is printed to stderr and tolerated, as InitProvider always did:
// an empty endpoint uses the exporter default and an empty service name
// yields an empty service.name.
func newLogProvider(ctx context.Context, cfg Config, strict bool) (*Provider, error) {
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

	// Register as the global provider so NewWriter() can resolve it.
	otel.SetLoggerProvider(provider)

	return &Provider{provider: provider}, nil
}

// InitProvider is NewProvider with a plaintext (Insecure) connection
// and no headers or timeout, kept for compatibility. It never fails and does
// not validate its arguments: an empty collectorEndpoint uses the exporter
// default and an empty serviceName an empty service.name, as before
// NewProvider existed. Only if the exporter cannot be created at all does
// it print the error to stderr and return a handle to a provider that exports
// nothing. Prefer NewProvider, which supports TLS and returns errors.
//
// Call it BEFORE logger.Init so that NewWriter can resolve the global
// provider. Defer Shutdown (or Close) to flush on exit.
//
// Usage:
//
//	lp := otlp.InitProvider(ctx, "my-service", "otel-collector:4317")
//	defer lp.Shutdown(ctx)
//	_, cleanup := logger.Init("my-service", version, otlp.NewWriter("my-service"))
//	defer cleanup.Close()
func InitProvider(ctx context.Context, serviceName, collectorEndpoint string) *Provider {
	handle, err := newLogProvider(ctx, Config{
		ServiceName: serviceName,
		Endpoint:    collectorEndpoint,
		Insecure:    true,
	}, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: could not create OTLP log provider: %v — logs will not be exported\n", err)
		return &Provider{provider: log.NewLoggerProvider()}
	}
	return handle
}

// Setup bundles the OTLP boilerplate so callers cannot easily get the
// order wrong. It calls InitProvider, then constructs an Writer
// against that provider. Returns the handle (caller must defer Shutdown)
// and the writer ready to pass to logger.Init or logger.New.
//
// Like InitProvider it uses a plaintext connection and reports setup
// errors only on stderr; for TLS, headers or errors use NewProvider
// followed by NewWriter.
//
// Usage:
//
//	handle, writer := otlp.Setup(ctx, "my-service", "otel-collector:4317")
//	defer handle.Shutdown(ctx)
//	_, cleanup := logger.Init("my-service", "1.0.0", writer)
//	defer cleanup.Close()
func Setup(ctx context.Context, serviceName, collectorEndpoint string) (*Provider, *Writer) {
	handle := InitProvider(ctx, serviceName, collectorEndpoint)
	return handle, NewWriter(serviceName)
}
