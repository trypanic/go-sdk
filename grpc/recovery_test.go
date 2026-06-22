package grpc

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
	interceptor := recoveryUnaryInterceptor(nil)
	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/M"},
		func(context.Context, any) (any, error) { panic("boom") })

	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %s, want %s", status.Code(err), codes.Internal)
	}
}

func TestRecoveryUnaryInterceptor_NoPanicPassesThrough(t *testing.T) {
	want := errors.New("handler error")
	interceptor := recoveryUnaryInterceptor(nil)
	resp, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{FullMethod: "/svc/M"},
		func(_ context.Context, req any) (any, error) { return req, want })

	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if resp != "req" {
		t.Fatalf("resp = %v, want %q", resp, "req")
	}
}

func TestRecoveryStreamInterceptor_PanicReturnsInternal(t *testing.T) {
	interceptor := recoveryStreamInterceptor(nil)
	ss := &fakeServerStream{ctx: context.Background()}
	err := interceptor(nil, ss, &grpc.StreamServerInfo{FullMethod: "/svc/S"},
		func(any, grpc.ServerStream) error { panic("boom") })

	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %s, want %s", status.Code(err), codes.Internal)
	}
}

func TestRecoveryStreamInterceptor_NoPanicPassesThrough(t *testing.T) {
	want := errors.New("stream error")
	interceptor := recoveryStreamInterceptor(nil)
	ss := &fakeServerStream{ctx: context.Background()}
	err := interceptor(nil, ss, &grpc.StreamServerInfo{FullMethod: "/svc/S"},
		func(any, grpc.ServerStream) error { return want })

	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}
