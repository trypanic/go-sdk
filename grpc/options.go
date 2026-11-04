package grpc

import (
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"

	"github.com/trypanic/go-sdk/logger"
	"github.com/trypanic/go-sdk/telemetry"
)

// ServerOption configures a Server at construction time.
type ServerOption func(*serverBuild)

// serverBuild accumulates server options before they are resolved into
// grpc.ServerOption values.
type serverBuild struct {
	tracing        bool
	recovery       bool
	insecure       bool
	publicEndpoint bool
	tracerProvider trace.TracerProvider
	propagator     propagation.TextMapPropagator
	otelOptions    []otelgrpc.Option
	log            *logger.Logger
	errorDomain    string
	unary          []grpc.UnaryServerInterceptor
	stream         []grpc.StreamServerInterceptor
	raw            []grpc.ServerOption
}

// defaultServerBuild returns the default server profile: tracing and recovery
// both enabled.
func defaultServerBuild() *serverBuild {
	return &serverBuild{tracing: true, recovery: true}
}

// WithServerTracing toggles the otelgrpc stats handler (traces every call
// mode). Enabled by default.
func WithServerTracing(on bool) ServerOption {
	return func(b *serverBuild) { b.tracing = on }
}

// WithServerRecovery toggles panic recovery on unary and streaming handlers.
// Enabled by default.
func WithServerRecovery(on bool) ServerOption {
	return func(b *serverBuild) { b.recovery = on }
}

// WithServerInsecure states that the server accepts plaintext (unencrypted,
// unauthenticated) connections, i.e. that ServerConfig.Creds is nil on
// purpose; without it a nil Creds is a configuration error. Use it only behind a service mesh or sidecar that terminates mTLS,
// or in tests. Combining it with ServerConfig.Creds is a configuration error.
func WithServerInsecure() ServerOption {
	return func(b *serverBuild) { b.insecure = true }
}

// WithServerTracerProvider injects the tracer provider used for spans.
// Defaults to the global OpenTelemetry provider.
func WithServerTracerProvider(tp trace.TracerProvider) ServerOption {
	return func(b *serverBuild) { b.tracerProvider = tp }
}

// WithServerPropagator injects the propagator used to extract trace context.
// Defaults to the global OpenTelemetry propagator.
func WithServerPropagator(p propagation.TextMapPropagator) ServerOption {
	return func(b *serverBuild) { b.propagator = p }
}

// WithServerPublicEndpoint treats the trace context of incoming calls as
// untrusted: each server span starts a new trace and carries a link to the
// caller's span instead of becoming its child. Use it on servers reachable by
// external (internet) clients, so callers cannot choose your trace IDs or,
// with a ParentBased sampler, force sampling of every call. Leave it off for
// internal service-to-service traffic, where joining the caller's trace is
// what you want. It has no effect when tracing is disabled.
func WithServerPublicEndpoint() ServerOption {
	return func(b *serverBuild) { b.publicEndpoint = true }
}

// WithServerOTelOptions passes extra otelgrpc options (for example
// otelgrpc.WithPublicEndpointFn or otelgrpc.WithFilter) to the server stats
// handler. They are applied after the options this package derives from
// WithServerTracerProvider, WithServerPropagator and WithServerPublicEndpoint,
// so they take precedence. It has no effect when tracing is disabled.
func WithServerOTelOptions(o ...otelgrpc.Option) ServerOption {
	return func(b *serverBuild) { b.otelOptions = append(b.otelOptions, o...) }
}

// WithServerLogger injects the logger used by panic recovery. Defaults to the
// SDK global logger.
func WithServerLogger(l *logger.Logger) ServerOption {
	return func(b *serverBuild) { b.log = l }
}

// WithServerErrorDomain sets the errdetails.ErrorInfo.Domain of the status
// returned for a recovered panic: the service that produced the error,
// usually its DNS name (e.g. "payments.example.com"). Defaults to "go-sdk".
// Pass the same value as StatusPolicy.Domain to ToStatusWithPolicy in your
// handlers so every error from the service carries one domain.
func WithServerErrorDomain(domain string) ServerOption {
	return func(b *serverBuild) { b.errorDomain = domain }
}

// WithUnaryInterceptors appends extra unary interceptors. They run after (inside)
// the recovery interceptor, in the order given, so recovery also catches
// their panics.
func WithUnaryInterceptors(in ...grpc.UnaryServerInterceptor) ServerOption {
	return func(b *serverBuild) { b.unary = append(b.unary, in...) }
}

// WithStreamInterceptors appends extra stream interceptors. They run after
// (inside) the recovery interceptor, in the order given, so recovery also
// catches their panics.
func WithStreamInterceptors(in ...grpc.StreamServerInterceptor) ServerOption {
	return func(b *serverBuild) { b.stream = append(b.stream, in...) }
}

// WithRawServerOptions passes raw grpc.ServerOption values straight through to
// grpc.NewServer, for anything this package does not expose directly.
//
// Do not add interceptors here: grpc.UnaryInterceptor and
// grpc.StreamInterceptor passed this way run outside (before) the recovery
// interceptor, so a panic in them is not recovered and crashes the process.
// Use WithUnaryInterceptors / WithStreamInterceptors, which run inside
// recovery.
func WithRawServerOptions(o ...grpc.ServerOption) ServerOption {
	return func(b *serverBuild) { b.raw = append(b.raw, o...) }
}

// ClientOption configures a client dial at construction time.
type ClientOption func(*clientBuild)

// clientBuild accumulates client options before they are resolved into
// grpc.DialOption values.
type clientBuild struct {
	tracing        bool
	insecure       bool
	defaultTimeout time.Duration
	tracerProvider trace.TracerProvider
	propagator     propagation.TextMapPropagator
	unary          []grpc.UnaryClientInterceptor
	stream         []grpc.StreamClientInterceptor
	raw            []grpc.DialOption
}

// defaultClientBuild returns the default client profile: tracing enabled.
func defaultClientBuild() *clientBuild {
	return &clientBuild{tracing: true}
}

// WithClientTracing toggles the otelgrpc client stats handler. Enabled by default.
func WithClientTracing(on bool) ClientOption {
	return func(b *clientBuild) { b.tracing = on }
}

// WithClientInsecure states that the connection is plaintext (unencrypted,
// unauthenticated), i.e. that ClientConfig.Creds is nil on purpose; without
// it a nil Creds is a configuration error. Use it
// only behind a service mesh or sidecar that terminates mTLS, or in tests.
// Combining it with ClientConfig.Creds is a configuration error.
func WithClientInsecure() ClientOption {
	return func(b *clientBuild) { b.insecure = true }
}

// WithClientDefaultTimeout applies timeout d to every call (unary and all
// streaming modes) whose context has no deadline. A deadline already set on
// the context is never overridden, so long-lived streams opt out by setting
// their own (longer) deadline. d <= 0 disables it (the default: calls without
// a deadline wait until the server answers or the connection breaks).
//
// It runs before the WithClientUnaryInterceptors / WithClientStreamInterceptors
// interceptors, so they see the deadline.
func WithClientDefaultTimeout(d time.Duration) ClientOption {
	return func(b *clientBuild) { b.defaultTimeout = d }
}

// WithClientTracerProvider injects the tracer provider used for client spans.
func WithClientTracerProvider(tp trace.TracerProvider) ClientOption {
	return func(b *clientBuild) { b.tracerProvider = tp }
}

// WithClientPropagator injects the propagator used to inject trace context.
func WithClientPropagator(p propagation.TextMapPropagator) ClientOption {
	return func(b *clientBuild) { b.propagator = p }
}

// WithClientUnaryInterceptors appends extra unary client interceptors.
func WithClientUnaryInterceptors(in ...grpc.UnaryClientInterceptor) ClientOption {
	return func(b *clientBuild) { b.unary = append(b.unary, in...) }
}

// WithClientStreamInterceptors appends extra stream client interceptors.
func WithClientStreamInterceptors(in ...grpc.StreamClientInterceptor) ClientOption {
	return func(b *clientBuild) { b.stream = append(b.stream, in...) }
}

// WithRawDialOptions passes raw grpc.DialOption values straight through to
// grpc.NewClient, for anything this package does not expose directly.
func WithRawDialOptions(o ...grpc.DialOption) ClientOption {
	return func(b *clientBuild) { b.raw = append(b.raw, o...) }
}

// WithServerInstrumenter uses the instrumenter's provider for server spans.
// A nil instrumenter disables recording through a no-op provider.
func WithServerInstrumenter(i *telemetry.Instrumenter) ServerOption {
	return WithServerTracerProvider(i.TracerProvider())
}

// WithClientInstrumenter uses the instrumenter's provider for client spans.
// A nil instrumenter disables recording through a no-op provider.
func WithClientInstrumenter(i *telemetry.Instrumenter) ClientOption {
	return WithClientTracerProvider(i.TracerProvider())
}
