package grpc

import (
	"context"
	"errors"
	"io"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
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
	propagation.TraceContext{}.Inject(trace.ContextWithRemoteSpanContext(t.Context(), remoteParent), carrier)
	ctx := testCtx(t)
	callCtx := metadata.AppendToOutgoingContext(ctx, "traceparent", carrier.Get("traceparent"))
	if err := cc.Invoke(callCtx, testsvc.UnaryMethod, wrapperspb.String("x"), new(wrapperspb.StringValue)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	return sink.next(ctx, t, trace.SpanKindServer)
}

// Control: by default the incoming traceparent is the server span's parent.
func TestServerTracing_IncomingTraceparentIsParentByDefault(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	assertNotParented(t, invokeWithTraceparent(t, WithServerPublicEndpoint()))
}

// WithServerOTelOptions forwards otelgrpc options to the server handler.
func TestServerTracing_OTelOptionsAreForwarded(t *testing.T) {
	t.Parallel()
	public := otelgrpc.WithPublicEndpointFn(func(context.Context, *stats.RPCTagInfo) bool { return true })
	assertNotParented(t, invokeWithTraceparent(t, WithServerOTelOptions(public)))
}

// collect waits for n ended spans, failing the test on ctx expiry.
func (s *spanSink) collect(ctx context.Context, t *testing.T, n int) []sdktrace.ReadOnlySpan {
	t.Helper()
	spans := make([]sdktrace.ReadOnlySpan, 0, n)
	for len(spans) < n {
		select {
		case span := <-s.ended:
			spans = append(spans, span)
		case <-ctx.Done():
			t.Fatalf("got %d ended spans, want %d: %v", len(spans), n, ctx.Err())
		}
	}
	return spans
}

// Tracing is on by default on both sides and covers all four call modes: each
// call yields one client span and one server span in the same trace, the
// server span a child of the client span (context propagated over the wire).
func TestTracing_ClientAndServerSpanPerCallMode(t *testing.T) {
	t.Parallel()
	calls := []struct {
		method string
		call   func(ctx context.Context, cc *grpc.ClientConn) error
	}{
		{testsvc.UnaryMethod, func(ctx context.Context, cc *grpc.ClientConn) error {
			return cc.Invoke(ctx, testsvc.UnaryMethod, wrapperspb.String("x"), new(wrapperspb.StringValue))
		}},
		{testsvc.ServerStreamMethod, func(ctx context.Context, cc *grpc.ClientConn) error {
			return drainStream(ctx, cc, testsvc.ServerStreamDesc, testsvc.ServerStreamMethod, 1)
		}},
		{testsvc.ClientStreamMethod, func(ctx context.Context, cc *grpc.ClientConn) error {
			return drainStream(ctx, cc, testsvc.ClientStreamDesc, testsvc.ClientStreamMethod, 3)
		}},
		{testsvc.BidiMethod, func(ctx context.Context, cc *grpc.ClientConn) error {
			return drainStream(ctx, cc, testsvc.BidiStreamDesc, testsvc.BidiMethod, 3)
		}},
	}
	for _, tc := range calls {
		t.Run(tc.method, func(t *testing.T) {
			t.Parallel()
			tp, sink := newSpanSink(t)
			prop := propagation.TraceContext{}
			srv, lis := serve(t, ServerConfig{}, &echoImpl{},
				WithServerTracerProvider(tp), WithServerPropagator(prop))
			cc := connect(t, lis, ClientConfig{},
				WithClientTracerProvider(tp), WithClientPropagator(prop))
			t.Cleanup(func() { _ = cc.Close(); srv.Stop() })

			ctx := testCtx(t)
			if err := tc.call(ctx, cc); err != nil {
				t.Fatalf("call: %v", err)
			}
			var client, server sdktrace.ReadOnlySpan
			for _, span := range sink.collect(ctx, t, 2) {
				switch span.SpanKind() {
				case trace.SpanKindClient:
					client = span
				case trace.SpanKindServer:
					server = span
				}
			}
			if client == nil || server == nil {
				t.Fatalf("want one client and one server span, got client=%v server=%v", client, server)
			}
			wantName := tc.method[1:] // "sdk.test.Echo/Unary"
			if client.Name() != wantName || server.Name() != wantName {
				t.Fatalf("span names = %q (client), %q (server), want %q", client.Name(), server.Name(), wantName)
			}
			if server.SpanContext().TraceID() != client.SpanContext().TraceID() {
				t.Fatalf("server trace %s != client trace %s", server.SpanContext().TraceID(), client.SpanContext().TraceID())
			}
			if server.Parent().SpanID() != client.SpanContext().SpanID() {
				t.Fatalf("server parent %s, want the client span %s", server.Parent().SpanID(), client.SpanContext().SpanID())
			}
		})
	}
}

// drainStream sends sends messages on a new stream, half-closes it and reads
// until io.EOF.
func drainStream(ctx context.Context, cc *grpc.ClientConn, desc *grpc.StreamDesc, method string, sends int) error {
	cs, err := cc.NewStream(ctx, desc, method)
	if err != nil {
		return err
	}
	for range sends {
		if err := cs.SendMsg(wrapperspb.String("x")); err != nil {
			return err
		}
	}
	if err := cs.CloseSend(); err != nil {
		return err
	}
	for {
		err := cs.RecvMsg(new(wrapperspb.StringValue))
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// With tracing disabled on both sides, no span is recorded. The server waits
// for its handlers on Shutdown (grpc.WaitForHandlers), so a server span, had
// one been started, would have ended before the check.
func TestTracing_DisabledRecordsNoSpans(t *testing.T) {
	t.Parallel()
	tp, sink := newSpanSink(t)
	srv, lis := serve(t, ServerConfig{}, &echoImpl{},
		WithServerTracing(false), WithServerTracerProvider(tp),
		WithRawServerOptions(grpc.WaitForHandlers(true)))
	t.Cleanup(srv.Stop)
	cc := connect(t, lis, ClientConfig{}, WithClientTracing(false), WithClientTracerProvider(tp))
	t.Cleanup(func() { _ = cc.Close() })

	ctx := testCtx(t)
	if err := cc.Invoke(ctx, testsvc.UnaryMethod, wrapperspb.String("x"), new(wrapperspb.StringValue)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if err := cc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if spans := sink.Ended(); len(spans) != 0 {
		t.Fatalf("tracing disabled but %d spans recorded: %v", len(spans), spans)
	}
}
