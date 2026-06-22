package grpc

import (
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"

	"github.com/trypanic/go-sdk/logger"
)

// ServerOption configures a Server at construction time.
type ServerOption func(*serverBuild)

// serverBuild accumulates server options before they are resolved into
// grpc.ServerOption values.
type serverBuild struct {
	tracing        bool
	recovery       bool
	tracerProvider trace.TracerProvider
	propagator     propagation.TextMapPropagator
	log            *logger.Logger
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

// WithServerLogger injects the logger used by panic recovery. Defaults to the
// SDK global logger.
func WithServerLogger(l *logger.Logger) ServerOption {
	return func(b *serverBuild) { b.log = l }
}

// WithUnaryInterceptors appends extra unary interceptors. They run after the
// recovery interceptor, in the order given.
func WithUnaryInterceptors(in ...grpc.UnaryServerInterceptor) ServerOption {
	return func(b *serverBuild) { b.unary = append(b.unary, in...) }
}

// WithStreamInterceptors appends extra stream interceptors. They run after the
// recovery interceptor, in the order given.
func WithStreamInterceptors(in ...grpc.StreamServerInterceptor) ServerOption {
	return func(b *serverBuild) { b.stream = append(b.stream, in...) }
}

// WithRawServerOptions passes raw grpc.ServerOption values straight through to
// grpc.NewServer, for anything this package does not expose directly.
func WithRawServerOptions(o ...grpc.ServerOption) ServerOption {
	return func(b *serverBuild) { b.raw = append(b.raw, o...) }
}

// ClientOption configures a client dial at construction time.
type ClientOption func(*clientBuild)

// clientBuild accumulates client options before they are resolved into
// grpc.DialOption values.
type clientBuild struct {
	tracing        bool
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
