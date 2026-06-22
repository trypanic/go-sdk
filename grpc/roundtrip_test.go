package grpc

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/grpc/internal/testsvc"
)

// echoImpl implements testsvc.Impl. Each method falls back to a default echo
// behavior; tests override the relevant func to inject panics or hold streams.
type echoImpl struct {
	unary        func(context.Context, *wrapperspb.StringValue) (*wrapperspb.StringValue, error)
	serverStream func(grpc.ServerStream) error
	clientStream func(grpc.ServerStream) error
	bidi         func(grpc.ServerStream) error
}

func (e *echoImpl) Unary(ctx context.Context, in *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	if e.unary != nil {
		return e.unary(ctx, in)
	}
	return wrapperspb.String(in.GetValue() + "-pong"), nil
}

func (e *echoImpl) ServerStream(s grpc.ServerStream) error {
	if e.serverStream != nil {
		return e.serverStream(s)
	}
	in := new(wrapperspb.StringValue)
	if err := s.RecvMsg(in); err != nil {
		return err
	}
	for i := 0; i < 3; i++ {
		if err := s.SendMsg(wrapperspb.String(fmt.Sprintf("%s-%d", in.GetValue(), i))); err != nil {
			return err
		}
	}
	return nil
}

func (e *echoImpl) ClientStream(s grpc.ServerStream) error {
	if e.clientStream != nil {
		return e.clientStream(s)
	}
	count := 0
	for {
		in := new(wrapperspb.StringValue)
		err := s.RecvMsg(in)
		if err == io.EOF {
			return s.SendMsg(wrapperspb.String(fmt.Sprintf("count=%d", count)))
		}
		if err != nil {
			return err
		}
		count++
	}
}

func (e *echoImpl) Bidi(s grpc.ServerStream) error {
	if e.bidi != nil {
		return e.bidi(s)
	}
	for {
		in := new(wrapperspb.StringValue)
		err := s.RecvMsg(in)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.SendMsg(wrapperspb.String(in.GetValue() + "-echo")); err != nil {
			return err
		}
	}
}

func bufDialer(lis *bufconn.Listener) func(context.Context, string) (net.Conn, error) {
	return func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }
}

func serve(t *testing.T, scfg ServerConfig, impl testsvc.Impl, opts ...ServerOption) (*Server, *bufconn.Listener) {
	t.Helper()
	if scfg.Address == "" {
		scfg.Address = "bufnet"
	}
	lis := bufconn.Listen(1024 * 1024)
	srv, err := New(scfg, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	testsvc.Register(srv.Registrar(), impl)
	go func() { _ = srv.Serve(lis) }()
	return srv, lis
}

func connect(t *testing.T, lis *bufconn.Listener, ccfg ClientConfig, opts ...ClientOption) *grpc.ClientConn {
	t.Helper()
	if ccfg.Target == "" {
		ccfg.Target = "passthrough:///bufnet"
	}
	opts = append(opts, WithRawDialOptions(grpc.WithContextDialer(bufDialer(lis))))
	cc, err := Dial(ccfg, opts...)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	return cc
}

// startTestServer wires a default server + client over an in-memory listener.
func startTestServer(t *testing.T, impl testsvc.Impl, opts ...ServerOption) *grpc.ClientConn {
	srv, lis := serve(t, ServerConfig{}, impl, opts...)
	cc := connect(t, lis, ClientConfig{})
	t.Cleanup(func() { _ = cc.Close(); srv.Stop() })
	return cc
}

func TestRoundTrip_Unary(t *testing.T) {
	cc := startTestServer(t, &echoImpl{})
	out := new(wrapperspb.StringValue)
	if err := cc.Invoke(context.Background(), testsvc.UnaryMethod, wrapperspb.String("ping"), out); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out.GetValue() != "ping-pong" {
		t.Fatalf("got %q, want %q", out.GetValue(), "ping-pong")
	}
}

func TestRoundTrip_ServerStream(t *testing.T) {
	cc := startTestServer(t, &echoImpl{})
	cs, err := cc.NewStream(context.Background(), testsvc.ServerStreamDesc, testsvc.ServerStreamMethod)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := cs.SendMsg(wrapperspb.String("x")); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	if err := cs.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	var got []string
	for {
		out := new(wrapperspb.StringValue)
		err := cs.RecvMsg(out)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("RecvMsg: %v", err)
		}
		got = append(got, out.GetValue())
	}
	if len(got) != 3 {
		t.Fatalf("got %d messages, want 3: %v", len(got), got)
	}
}

func TestRoundTrip_ClientStream(t *testing.T) {
	cc := startTestServer(t, &echoImpl{})
	cs, err := cc.NewStream(context.Background(), testsvc.ClientStreamDesc, testsvc.ClientStreamMethod)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	for i := 0; i < 5; i++ {
		if err := cs.SendMsg(wrapperspb.String("x")); err != nil {
			t.Fatalf("SendMsg: %v", err)
		}
	}
	if err := cs.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	out := new(wrapperspb.StringValue)
	if err := cs.RecvMsg(out); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}
	if out.GetValue() != "count=5" {
		t.Fatalf("got %q, want %q", out.GetValue(), "count=5")
	}
}

func TestRoundTrip_BidiConcurrent(t *testing.T) {
	cc := startTestServer(t, &echoImpl{})
	cs, err := cc.NewStream(context.Background(), testsvc.BidiStreamDesc, testsvc.BidiMethod)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}

	const n = 10
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			if err := cs.SendMsg(wrapperspb.String(fmt.Sprintf("m%d", i))); err != nil {
				return
			}
		}
		_ = cs.CloseSend()
	}()

	got := 0
	for {
		out := new(wrapperspb.StringValue)
		err := cs.RecvMsg(out)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("RecvMsg: %v", err)
		}
		got++
	}
	wg.Wait()
	if got != n {
		t.Fatalf("got %d echoes, want %d", got, n)
	}
}

func TestRoundTrip_RecoveryUnary_ServerStaysUp(t *testing.T) {
	impl := &echoImpl{
		unary: func(_ context.Context, in *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
			if in.GetValue() == "panic" {
				panic("boom")
			}
			return wrapperspb.String("ok"), nil
		},
	}
	cc := startTestServer(t, impl)

	out := new(wrapperspb.StringValue)
	err := cc.Invoke(context.Background(), testsvc.UnaryMethod, wrapperspb.String("panic"), out)
	if status.Code(err) != codes.Internal {
		t.Fatalf("panic call code = %s, want %s", status.Code(err), codes.Internal)
	}
	if err := cc.Invoke(context.Background(), testsvc.UnaryMethod, wrapperspb.String("ok"), out); err != nil {
		t.Fatalf("second call failed (server down?): %v", err)
	}
}

func TestRoundTrip_RecoveryStream_ServerStaysUp(t *testing.T) {
	impl := &echoImpl{
		serverStream: func(s grpc.ServerStream) error {
			in := new(wrapperspb.StringValue)
			if err := s.RecvMsg(in); err != nil {
				return err
			}
			panic("boom")
		},
	}
	cc := startTestServer(t, impl)

	cs, err := cc.NewStream(context.Background(), testsvc.ServerStreamDesc, testsvc.ServerStreamMethod)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := cs.SendMsg(wrapperspb.String("x")); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	_ = cs.CloseSend()
	out := new(wrapperspb.StringValue)
	if err := cs.RecvMsg(out); status.Code(err) != codes.Internal {
		t.Fatalf("stream panic code = %s, want %s", status.Code(err), codes.Internal)
	}
	if err := cc.Invoke(context.Background(), testsvc.UnaryMethod, wrapperspb.String("ping"), out); err != nil {
		t.Fatalf("server down after stream panic: %v", err)
	}
}

func TestRoundTrip_WithKeepalive(t *testing.T) {
	scfg := ServerConfig{Keepalive: ServerKeepalive{
		Time: time.Second, Timeout: time.Second,
		MinTime: 100 * time.Millisecond, PermitWithoutStream: true,
	}}
	srv, lis := serve(t, scfg, &echoImpl{})
	defer srv.Stop()
	cc := connect(t, lis, ClientConfig{Keepalive: ClientKeepalive{
		Time: 500 * time.Millisecond, Timeout: time.Second, PermitWithoutStream: true,
	}})
	defer cc.Close()

	out := new(wrapperspb.StringValue)
	if err := cc.Invoke(context.Background(), testsvc.UnaryMethod, wrapperspb.String("ping"), out); err != nil {
		t.Fatalf("Invoke with keepalive configured: %v", err)
	}
	if out.GetValue() != "ping-pong" {
		t.Fatalf("got %q, want %q", out.GetValue(), "ping-pong")
	}
}

func TestShutdown_DrainsActiveStream(t *testing.T) {
	const n = 5
	started := make(chan struct{})
	impl := &echoImpl{
		serverStream: func(s grpc.ServerStream) error {
			in := new(wrapperspb.StringValue)
			if err := s.RecvMsg(in); err != nil {
				return err
			}
			close(started)
			for i := 0; i < n; i++ {
				if err := s.SendMsg(wrapperspb.String(fmt.Sprintf("d%d", i))); err != nil {
					return err
				}
				time.Sleep(20 * time.Millisecond)
			}
			return nil
		},
	}
	srv, lis := serve(t, ServerConfig{}, impl)
	cc := connect(t, lis, ClientConfig{})
	defer cc.Close()

	cs, err := cc.NewStream(context.Background(), testsvc.ServerStreamDesc, testsvc.ServerStreamMethod)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := cs.SendMsg(wrapperspb.String("go")); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	_ = cs.CloseSend()
	<-started // handler is mid-stream

	shutErr := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutErr <- srv.Shutdown(ctx)
	}()

	got := 0
	for {
		out := new(wrapperspb.StringValue)
		err := cs.RecvMsg(out)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("RecvMsg during drain: %v", err)
		}
		got++
	}
	if got != n {
		t.Fatalf("drained %d messages, want %d", got, n)
	}
	if err := <-shutErr; err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
}

func TestShutdown_DeadlineForcesStop(t *testing.T) {
	started := make(chan struct{})
	block := make(chan struct{})
	impl := &echoImpl{
		serverStream: func(s grpc.ServerStream) error {
			in := new(wrapperspb.StringValue)
			if err := s.RecvMsg(in); err != nil {
				return err
			}
			close(started)
			select {
			case <-block:
			case <-s.Context().Done():
			}
			return nil
		},
	}
	srv, lis := serve(t, ServerConfig{}, impl)
	cc := connect(t, lis, ClientConfig{})
	defer cc.Close()
	defer close(block)

	cs, err := cc.NewStream(context.Background(), testsvc.ServerStreamDesc, testsvc.ServerStreamMethod)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := cs.SendMsg(wrapperspb.String("go")); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	_ = cs.CloseSend()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already expired
	wantCode(t, srv.Shutdown(ctx), errorkit.ERR_SYSTEM_TIMEOUT_INTERNAL)
}
