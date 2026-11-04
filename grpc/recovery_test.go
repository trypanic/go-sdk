package grpc

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/trypanic/go-sdk/v2/errorkit"
	"github.com/trypanic/go-sdk/v2/grpc/internal/testsvc"
	"github.com/trypanic/go-sdk/v2/logger"
)

// fakeServerStream is a minimal grpc.ServerStream for interceptor unit tests.
type fakeServerStream struct{ ctx context.Context }

func (f *fakeServerStream) Context() context.Context     { return f.ctx }
func (f *fakeServerStream) SetHeader(metadata.MD) error  { return nil }
func (f *fakeServerStream) SendHeader(metadata.MD) error { return nil }
func (f *fakeServerStream) SetTrailer(metadata.MD)       {}
func (f *fakeServerStream) SendMsg(any) error            { return nil }
func (f *fakeServerStream) RecvMsg(any) error            { return nil }

func TestRecoveryUnaryInterceptor_PanicReturnsInternal(t *testing.T) {
	t.Parallel()
	interceptor := recoveryUnaryInterceptor(nil, "")
	_, err := interceptor(t.Context(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/M"},
		func(context.Context, any) (any, error) { panic("boom") })

	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %s, want %s", status.Code(err), codes.Internal)
	}
}

func TestRecoveryUnaryInterceptor_NoPanicPassesThrough(t *testing.T) {
	t.Parallel()
	want := errors.New("handler error")
	interceptor := recoveryUnaryInterceptor(nil, "")
	resp, err := interceptor(t.Context(), "req", &grpc.UnaryServerInfo{FullMethod: "/svc/M"},
		func(_ context.Context, req any) (any, error) { return req, want })

	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if resp != "req" {
		t.Fatalf("resp = %v, want %q", resp, "req")
	}
}

func TestRecoveryStreamInterceptor_PanicReturnsInternal(t *testing.T) {
	t.Parallel()
	interceptor := recoveryStreamInterceptor(nil, "")
	ss := &fakeServerStream{ctx: t.Context()}
	err := interceptor(nil, ss, &grpc.StreamServerInfo{FullMethod: "/svc/S"},
		func(any, grpc.ServerStream) error { panic("boom") })

	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %s, want %s", status.Code(err), codes.Internal)
	}
}

func TestRecoveryStreamInterceptor_NoPanicPassesThrough(t *testing.T) {
	t.Parallel()
	want := errors.New("stream error")
	interceptor := recoveryStreamInterceptor(nil, "")
	ss := &fakeServerStream{ctx: t.Context()}
	err := interceptor(nil, ss, &grpc.StreamServerInfo{FullMethod: "/svc/S"},
		func(any, grpc.ServerStream) error { return want })

	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

// errorInfo returns the first errdetails.ErrorInfo attached to st, or nil.
func errorInfo(st *status.Status) *errdetails.ErrorInfo {
	for _, d := range st.Details() {
		if info, ok := d.(*errdetails.ErrorInfo); ok {
			return info
		}
	}
	return nil
}

// testCtx returns a per-test context with a deadline so a hung RPC fails the
// test instead of blocking until the go test timeout.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// assertNoPanicLeak checks that a recovered panic reaches the client as a
// fixed codes.Internal status: no panic value, no method name, and an
// ErrorInfo carrying the errorkit code and error ID for correlation.
func assertNoPanicLeak(t *testing.T, err error, secret, method string) {
	t.Helper()
	st := status.Convert(err)
	if st.Code() != codes.Internal {
		t.Fatalf("code = %s, want %s", st.Code(), codes.Internal)
	}
	if strings.Contains(st.Message(), secret) {
		t.Fatalf("panic value leaked to the client: %q", st.Message())
	}
	if strings.Contains(st.Message(), method) {
		t.Fatalf("method name leaked to the client: %q", st.Message())
	}
	info := errorInfo(st)
	if info == nil {
		t.Fatal("recovered panic status has no ErrorInfo detail")
	}
	if info.GetReason() != string(errorkit.ERR_SYSTEM_UNEXPECTED) {
		t.Fatalf("ErrorInfo.Reason = %q, want %q", info.GetReason(), errorkit.ERR_SYSTEM_UNEXPECTED)
	}
	if info.GetMetadata()["id"] == "" {
		t.Fatal("ErrorInfo.Metadata[id] is empty; the errorkit ID is needed for log correlation")
	}
}

const panicSecret = "db password=hunter2 at 10.0.0.5"

func TestRecovery_UnaryPanicValueNotSentToClient(t *testing.T) {
	t.Parallel()
	impl := &echoImpl{
		unary: func(context.Context, *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
			panic(panicSecret)
		},
	}
	cc := startTestServer(t, impl)

	err := cc.Invoke(testCtx(t), testsvc.UnaryMethod, wrapperspb.String("x"), new(wrapperspb.StringValue))
	assertNoPanicLeak(t, err, "hunter2", testsvc.UnaryMethod)
}

func TestRecovery_StreamPanicValueNotSentToClient(t *testing.T) {
	t.Parallel()
	impl := &echoImpl{
		serverStream: func(grpc.ServerStream) error { panic(panicSecret) },
	}
	cc := startTestServer(t, impl)

	cs, err := cc.NewStream(testCtx(t), testsvc.ServerStreamDesc, testsvc.ServerStreamMethod)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := cs.SendMsg(wrapperspb.String("x")); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	if err := cs.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	err = cs.RecvMsg(new(wrapperspb.StringValue))
	assertNoPanicLeak(t, err, "hunter2", testsvc.ServerStreamMethod)
}

// The panic value and the stack stay in the server log, keyed by the same
// errorkit ID the client receives in ErrorInfo.
//
// Not parallel: logger.New writes zerolog's process-wide settings, which the
// other tests' loggers read (a logger package issue tracked outside grpc).
func TestRecovery_PanicValueLoggedWithClientVisibleID(t *testing.T) {
	var buf syncBuffer
	log := logger.New(logger.Config{Env: logger.Prod, Writer: &buf})
	impl := &echoImpl{
		unary: func(context.Context, *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
			panic("boom-marker-42")
		},
	}
	cc := startTestServer(t, impl, WithServerLogger(log))

	err := cc.Invoke(testCtx(t), testsvc.UnaryMethod, wrapperspb.String("x"), new(wrapperspb.StringValue))
	info := errorInfo(status.Convert(err))
	if info == nil {
		t.Fatal("no ErrorInfo detail")
	}
	out := buf.String()
	if !strings.Contains(out, "boom-marker-42") {
		t.Fatalf("panic value missing from server log: %s", out)
	}
	if !strings.Contains(out, info.GetMetadata()["id"]) {
		t.Fatalf("server log does not carry the client-visible error id %q: %s", info.GetMetadata()["id"], out)
	}
}

// syncBuffer is a bytes.Buffer safe for the concurrent writes of a server.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
