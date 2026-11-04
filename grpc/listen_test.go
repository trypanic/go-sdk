package grpc

import (
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/grpc/internal/testsvc"
)

// freeLoopbackAddr returns a loopback address whose port was free a moment
// ago. Only loopback is used; no traffic leaves the host.
func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	addr := lis.Addr().String()
	if err := lis.Close(); err != nil {
		t.Fatalf("release loopback port: %v", err)
	}
	return addr
}

// ListenAndServe listens on ServerConfig.Address, so the configured address
// is the one the server actually serves on.
func TestServer_ListenAndServeUsesConfigAddress(t *testing.T) {
	t.Parallel()
	addr := freeLoopbackAddr(t)
	srv, err := New(ServerConfig{Address: addr}, WithServerInsecure())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	testsvc.Register(srv.Registrar(), &echoImpl{})

	served := make(chan error, 1)
	go func() { served <- srv.ListenAndServe() }()

	cc, err := Dial(ClientConfig{Target: "passthrough:///" + addr}, WithClientInsecure())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = cc.Close() })

	ctx := testCtx(t)
	out := new(wrapperspb.StringValue)
	if err := cc.Invoke(ctx, testsvc.UnaryMethod, wrapperspb.String("ping"), out, grpc.WaitForReady(true)); err != nil {
		t.Fatalf("Invoke on %s: %v", addr, err)
	}
	if out.GetValue() != "ping-pong" {
		t.Fatalf("got %q, want %q", out.GetValue(), "ping-pong")
	}

	srv.Stop()
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("ListenAndServe after Stop = %v, want nil", err)
		}
	case <-ctx.Done():
		t.Fatal("ListenAndServe did not return after Stop")
	}
}

// A listen failure (here: the port is taken) is returned at once as an
// errorkit error that wraps the net error.
func TestServer_ListenAndServeListenError(t *testing.T) {
	t.Parallel()
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy loopback port: %v", err)
	}
	t.Cleanup(func() { _ = taken.Close() })

	srv, err := New(ServerConfig{Address: taken.Addr().String()}, WithServerInsecure())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(srv.Stop)

	served := make(chan error, 1)
	go func() { served <- srv.ListenAndServe() }()
	select {
	case err := <-served:
		wantCode(t, err, errorkit.ERR_SYSTEM_UNEXPECTED)
		if _, ok := errors.AsType[*net.OpError](err); !ok {
			t.Fatalf("error %v does not wrap the *net.OpError", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ListenAndServe on a taken port did not return")
	}
}
