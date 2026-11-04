package grpc

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/grpc/internal/testsvc"
)

// Client interceptors added with WithClientUnaryInterceptors /
// WithClientStreamInterceptors run on every call, in the order given, and see
// the method being called.
func TestClientInterceptors_RunInOrder(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var trail []string
	record := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		trail = append(trail, s)
	}
	unary := func(name string) grpc.UnaryClientInterceptor {
		return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			record(name + " " + method)
			return invoker(ctx, method, req, reply, cc, opts...)
		}
	}
	stream := func(name string) grpc.StreamClientInterceptor {
		return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			record(name + " " + method)
			return streamer(ctx, desc, cc, method, opts...)
		}
	}
	srv, lis := serve(t, ServerConfig{}, &echoImpl{})
	t.Cleanup(srv.Stop)
	cc := connect(t, lis, ClientConfig{},
		WithClientUnaryInterceptors(unary("u1"), unary("u2")),
		WithClientStreamInterceptors(stream("s1"), stream("s2")),
	)
	t.Cleanup(func() { _ = cc.Close() })

	ctx := testCtx(t)
	if err := cc.Invoke(ctx, testsvc.UnaryMethod, wrapperspb.String("x"), new(wrapperspb.StringValue)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if err := drainStream(ctx, cc, testsvc.BidiStreamDesc, testsvc.BidiMethod, 1); err != nil {
		t.Fatalf("bidi: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{
		"u1 " + testsvc.UnaryMethod, "u2 " + testsvc.UnaryMethod,
		"s1 " + testsvc.BidiMethod, "s2 " + testsvc.BidiMethod,
	}
	if !slices.Equal(trail, want) {
		t.Fatalf("call trail = %v, want %v", trail, want)
	}
}

// panicObserver is a raw interceptor (WithRawServerOptions), which grpc-go
// runs outside the SDK chain: it reports whether a panic got past the chain.
func panicObserver(escaped chan<- any) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			r := recover()
			escaped <- r
			if r != nil {
				err = errors.New("recovered by the test observer")
			}
		}()
		return h(ctx, req)
	}
}

// Recovery is on by default (a handler panic never reaches an outer raw
// interceptor) and WithServerRecovery(false) removes it (the panic does).
// Together with TestTracing_DisabledRecordsNoSpans and
// TestServerInterceptors_RunInOrder it replaces a test that inspected the
// private serverBuild fields.
func TestServerRecovery_Toggle(t *testing.T) {
	t.Parallel()
	impl := &echoImpl{
		unary: func(context.Context, *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
			panic("handler panic")
		},
	}
	cases := []struct {
		name        string
		opts        []ServerOption
		wantEscaped bool
	}{
		{"default", nil, false},
		{"disabled", []ServerOption{WithServerRecovery(false)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			escaped := make(chan any, 1)
			opts := append([]ServerOption{WithRawServerOptions(grpc.UnaryInterceptor(panicObserver(escaped)))}, tc.opts...)
			cc := startTestServer(t, impl, opts...)

			ctx := testCtx(t)
			if err := cc.Invoke(ctx, testsvc.UnaryMethod, wrapperspb.String("x"), new(wrapperspb.StringValue)); err == nil {
				t.Fatal("Invoke succeeded, want an error from the panicking handler")
			}
			select {
			case r := <-escaped:
				if got := r != nil; got != tc.wantEscaped {
					t.Fatalf("panic escaped the SDK chain = %v (value %v), want %v", got, r, tc.wantEscaped)
				}
			case <-ctx.Done():
				t.Fatal("raw interceptor never ran")
			}
		})
	}
}

// A grpc.NewClient failure is returned as ERR_SYSTEM_UNEXPECTED wrapping the
// grpc-go error. Here: per-RPC credentials that require transport security
// on a plaintext connection.
func TestDial_NewClientErrorIsWrapped(t *testing.T) {
	t.Parallel()
	_, err := Dial(ClientConfig{Target: "passthrough:///bufnet"},
		WithClientInsecure(),
		WithRawDialOptions(grpc.WithPerRPCCredentials(secureOnlyCreds{})),
	)
	wantCode(t, err, errorkit.ERR_SYSTEM_UNEXPECTED)
	if errors.Unwrap(err) == nil {
		t.Fatalf("error %v does not wrap the grpc-go error", err)
	}
}

// secureOnlyCreds is a credentials.PerRPCCredentials that requires transport
// security.
type secureOnlyCreds struct{}

func (secureOnlyCreds) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return nil, nil
}
func (secureOnlyCreds) RequireTransportSecurity() bool { return true }

// Serve on a listener that fails to accept returns ERR_SYSTEM_UNEXPECTED
// wrapping the listener error.
func TestServe_ListenerErrorIsWrapped(t *testing.T) {
	t.Parallel()
	srv, err := New(ServerConfig{Address: "bufnet"}, WithServerInsecure())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(srv.Stop)
	lis := bufconn.Listen(1024)
	if err := lis.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	err = srv.Serve(lis)
	wantCode(t, err, errorkit.ERR_SYSTEM_UNEXPECTED)
	if errors.Unwrap(err) == nil {
		t.Fatalf("error %v does not wrap the listener error", err)
	}
}
