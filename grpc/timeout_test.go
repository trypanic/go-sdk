package grpc

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/trypanic/go-sdk/grpc/internal/testsvc"
)

// deadlineProbe is an echoImpl whose unary and server-stream handlers report
// the deadline they see.
func deadlineProbe(seen chan<- time.Duration) *echoImpl {
	report := func(ctx context.Context) {
		d := time.Duration(-1) // no deadline
		if dl, ok := ctx.Deadline(); ok {
			d = time.Until(dl)
		}
		seen <- d
	}
	return &echoImpl{
		unary: func(ctx context.Context, _ *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
			report(ctx)
			return wrapperspb.String("ok"), nil
		},
		serverStream: func(s grpc.ServerStream) error {
			report(s.Context())
			return nil
		},
	}
}

func TestClientDefaultTimeout_AppliedWhenCtxHasNoDeadline(t *testing.T) {
	t.Parallel()
	const d = 2 * time.Second
	seen := make(chan time.Duration, 2)
	srv, lis := serve(t, ServerConfig{}, deadlineProbe(seen))
	t.Cleanup(srv.Stop)
	cc := connect(t, lis, ClientConfig{}, WithClientDefaultTimeout(d))
	t.Cleanup(func() { _ = cc.Close() })

	ctx := noDeadlineCtx(t)
	if err := cc.Invoke(ctx, testsvc.UnaryMethod, wrapperspb.String("x"), new(wrapperspb.StringValue)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	assertDeadlineAbout(t, "unary", <-seen, d)

	cs, err := cc.NewStream(ctx, testsvc.ServerStreamDesc, testsvc.ServerStreamMethod)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := cs.SendMsg(wrapperspb.String("x")); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	if err := cs.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	if err := cs.RecvMsg(new(wrapperspb.StringValue)); err == nil {
		t.Fatal("RecvMsg: want io.EOF, got a message")
	}
	assertDeadlineAbout(t, "stream", <-seen, d)
}

func TestClientDefaultTimeout_DoesNotOverrideCallerDeadline(t *testing.T) {
	t.Parallel()
	seen := make(chan time.Duration, 1)
	srv, lis := serve(t, ServerConfig{}, deadlineProbe(seen))
	t.Cleanup(srv.Stop)
	cc := connect(t, lis, ClientConfig{}, WithClientDefaultTimeout(time.Second))
	t.Cleanup(func() { _ = cc.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), time.Hour)
	defer cancel()
	if err := cc.Invoke(ctx, testsvc.UnaryMethod, wrapperspb.String("x"), new(wrapperspb.StringValue)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := <-seen; got < 59*time.Minute {
		t.Fatalf("server saw a deadline of %s; the caller's 1h deadline was overridden", got)
	}
}

// The default timeout actually ends a call the server never answers.
func TestClientDefaultTimeout_ExpiresHungCall(t *testing.T) {
	t.Parallel()
	impl := &echoImpl{
		unary: func(ctx context.Context, _ *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		bidi: func(s grpc.ServerStream) error {
			<-s.Context().Done()
			return nil
		},
	}
	srv, lis := serve(t, ServerConfig{}, impl)
	t.Cleanup(srv.Stop)
	cc := connect(t, lis, ClientConfig{}, WithClientDefaultTimeout(100*time.Millisecond))
	t.Cleanup(func() { _ = cc.Close() })

	err := cc.Invoke(noDeadlineCtx(t), testsvc.UnaryMethod, wrapperspb.String("x"), new(wrapperspb.StringValue))
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("unary: code = %s (err %v), want %s", status.Code(err), err, codes.DeadlineExceeded)
	}

	cs, err := cc.NewStream(noDeadlineCtx(t), testsvc.BidiStreamDesc, testsvc.BidiMethod)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := cs.RecvMsg(new(wrapperspb.StringValue)); status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("stream: code = %s (err %v), want %s", status.Code(err), err, codes.DeadlineExceeded)
	}
}

// noDeadlineCtx returns a context without a deadline, so the default timeout
// applies. A watchdog cancels it after 5s, so a broken default timeout fails
// the test (codes.Canceled) instead of hanging it.
func noDeadlineCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	stop := time.AfterFunc(5*time.Second, cancel)
	t.Cleanup(func() { stop.Stop(); cancel() })
	return ctx
}

func assertDeadlineAbout(t *testing.T, mode string, got, want time.Duration) {
	t.Helper()
	if got < 0 {
		t.Fatalf("%s: server saw no deadline; want about %s", mode, want)
	}
	if got > want || got < want-time.Second {
		t.Fatalf("%s: server saw a deadline of %s, want about %s", mode, got, want)
	}
}
