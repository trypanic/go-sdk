package grpc

import (
	"context"
	"slices"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/trypanic/go-sdk/v2/grpc/internal/testsvc"
)

// Interceptors added with WithUnaryInterceptors / WithStreamInterceptors run
// inside recovery: a panic in one of them is recovered, reaches the client
// as codes.Internal, and the server stays up.
func TestRecovery_WrapsSDKManagedUnaryInterceptor(t *testing.T) {
	t.Parallel()
	panicky := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		if req.(*wrapperspb.StringValue).GetValue() == "panic" {
			panic("interceptor " + panicSecret)
		}
		return h(ctx, req)
	}
	cc := startTestServer(t, &echoImpl{}, WithUnaryInterceptors(panicky))

	err := cc.Invoke(testCtx(t), testsvc.UnaryMethod, wrapperspb.String("panic"), new(wrapperspb.StringValue))
	assertNoPanicLeak(t, err, "hunter2", testsvc.UnaryMethod)

	out := new(wrapperspb.StringValue)
	if err := cc.Invoke(testCtx(t), testsvc.UnaryMethod, wrapperspb.String("ping"), out); err != nil {
		t.Fatalf("server down after interceptor panic: %v", err)
	}
	if out.GetValue() != "ping-pong" {
		t.Fatalf("got %q, want %q", out.GetValue(), "ping-pong")
	}
}

func TestRecovery_WrapsSDKManagedStreamInterceptor(t *testing.T) {
	t.Parallel()
	panicky := func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, h grpc.StreamHandler) error {
		panic("interceptor " + panicSecret)
	}
	cc := startTestServer(t, &echoImpl{}, WithStreamInterceptors(panicky))

	cs, err := cc.NewStream(testCtx(t), testsvc.BidiStreamDesc, testsvc.BidiMethod)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	err = cs.RecvMsg(new(wrapperspb.StringValue))
	assertNoPanicLeak(t, err, "hunter2", testsvc.BidiMethod)

	if err := cc.Invoke(testCtx(t), testsvc.UnaryMethod, wrapperspb.String("ping"), new(wrapperspb.StringValue)); err != nil {
		t.Fatalf("server down after stream interceptor panic: %v", err)
	}
}

// User interceptors run in the order given, after recovery and before the
// handler, for unary and streaming calls.
func TestServerInterceptors_RunInOrder(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var trail []string
	record := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		trail = append(trail, s)
	}
	unary := func(name string) grpc.UnaryServerInterceptor {
		return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
			record(name)
			return h(ctx, req)
		}
	}
	stream := func(name string) grpc.StreamServerInterceptor {
		return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, h grpc.StreamHandler) error {
			record(name)
			return h(srv, ss)
		}
	}
	impl := &echoImpl{
		unary: func(context.Context, *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
			record("handler")
			return wrapperspb.String("ok"), nil
		},
		clientStream: func(s grpc.ServerStream) error {
			record("handler")
			return s.SendMsg(wrapperspb.String("ok"))
		},
	}
	cc := startTestServer(t, impl,
		WithUnaryInterceptors(unary("u1"), unary("u2")),
		WithStreamInterceptors(stream("s1"), stream("s2")),
	)

	if err := cc.Invoke(testCtx(t), testsvc.UnaryMethod, wrapperspb.String("x"), new(wrapperspb.StringValue)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	cs, err := cc.NewStream(testCtx(t), testsvc.ClientStreamDesc, testsvc.ClientStreamMethod)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := cs.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	if err := cs.RecvMsg(new(wrapperspb.StringValue)); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if want := []string{"u1", "u2", "handler", "s1", "s2", "handler"}; !slices.Equal(trail, want) {
		t.Fatalf("call trail = %v, want %v", trail, want)
	}
}
