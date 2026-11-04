package grpc

import (
	"context"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/trypanic/go-sdk/grpc/internal/testsvc"
)

// spanSink records ended spans and signals each one on a channel, so tests
// wait for a span without sleeping (a stats handler may end the server span
// after the client already has its response).
type spanSink struct {
	*tracetest.SpanRecorder
	ended chan sdktrace.ReadOnlySpan
}

func (s *spanSink) OnEnd(span sdktrace.ReadOnlySpan) {
	s.SpanRecorder.OnEnd(span)
	s.ended <- span
}

// newSpanSink returns a tracer provider backed by a spanSink.
func newSpanSink(t *testing.T) (*sdktrace.TracerProvider, *spanSink) {
	t.Helper()
	sink := &spanSink{SpanRecorder: tracetest.NewSpanRecorder(), ended: make(chan sdktrace.ReadOnlySpan, 64)}
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sink))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return tp, sink
}

// next waits for the next ended span of kind, failing the test on ctx expiry.
func (s *spanSink) next(ctx context.Context, t *testing.T, kind trace.SpanKind) sdktrace.ReadOnlySpan {
	t.Helper()
	for {
		select {
		case span := <-s.ended:
			if span.SpanKind() == kind {
				return span
			}
		case <-ctx.Done():
			t.Fatalf("no %s span ended: %v", kind, ctx.Err())
			return nil
		}
	}
}

// remoteParent is the span context an external caller puts in traceparent.
var remoteParent = trace.NewSpanContext(trace.SpanContextConfig{
	TraceID:    trace.TraceID{0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x10},
	SpanID:     trace.SpanID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
	TraceFlags: trace.FlagsSampled,
	Remote:     true,
})

// invokeWithTraceparent makes a unary call whose metadata carries the
// remoteParent traceparent, as an untrusted external caller would send it.
// Client tracing is off so only the header sets the incoming context.
func invokeWithTraceparent(t *testing.T, opts ...ServerOption) sdktrace.ReadOnlySpan {
	t.Helper()
	tp, sink := newSpanSink(t)
	opts = append([]ServerOption{
		WithServerTracerProvider(tp),
		WithServerPropagator(propagation.TraceContext{}),
	}, opts...)
	srv, lis := serve(t, ServerConfig{}, &echoImpl{}, opts...)
	cc := connect(t, lis, ClientConfig{}, WithClientTracing(false))
	t.Cleanup(func() { _ = cc.Close(); srv.Stop() })

	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(trace.ContextWithRemoteSpanContext(context.Background(), remoteParent), carrier)
	ctx := testCtx(t)
	callCtx := metadata.AppendToOutgoingContext(ctx, "traceparent", carrier.Get("traceparent"))
	if err := cc.Invoke(callCtx, testsvc.UnaryMethod, wrapperspb.String("x"), new(wrapperspb.StringValue)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	return sink.next(ctx, t, trace.SpanKindServer)
}

// Control: by default the incoming traceparent is the server span's parent.
func TestServerTracing_IncomingTraceparentIsParentByDefault(t *testing.T) {
	span := invokeWithTraceparent(t)
	if span.SpanContext().TraceID() != remoteParent.TraceID() {
		t.Fatalf("server span trace ID = %s, want the incoming %s", span.SpanContext().TraceID(), remoteParent.TraceID())
	}
	if span.Parent().SpanID() != remoteParent.SpanID() {
		t.Fatalf("server span parent = %s, want the incoming %s", span.Parent().SpanID(), remoteParent.SpanID())
	}
}

// assertNotParented checks that span starts a new trace and only links to the
// caller's span context.
func assertNotParented(t *testing.T, span sdktrace.ReadOnlySpan) {
	t.Helper()
	if span.SpanContext().TraceID() == remoteParent.TraceID() {
		t.Fatalf("server span joined the caller's trace %s; a public endpoint must start a new one", remoteParent.TraceID())
	}
	if span.Parent().IsValid() {
		t.Fatalf("server span has parent %s, want a root span", span.Parent().SpanID())
	}
	links := span.Links()
	if len(links) != 1 || !links[0].SpanContext.Equal(remoteParent) {
		t.Fatalf("server span links = %v, want one link to the caller's span context", links)
	}
}

// WithServerPublicEndpoint treats incoming trace context as untrusted: the
// server span is a new root linked to the caller's span.
func TestServerTracing_PublicEndpointDoesNotParentIncomingTrace(t *testing.T) {
	assertNotParented(t, invokeWithTraceparent(t, WithServerPublicEndpoint()))
}

// WithServerOTelOptions forwards otelgrpc options to the server handler.
func TestServerTracing_OTelOptionsAreForwarded(t *testing.T) {
	public := otelgrpc.WithPublicEndpointFn(func(context.Context, *stats.RPCTagInfo) bool { return true })
	assertNotParented(t, invokeWithTraceparent(t, WithServerOTelOptions(public)))
}
