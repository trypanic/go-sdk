package grpc

import (
	"context"
	"testing"

	"github.com/cloudwego/kitex/pkg/endpoint"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestRecoveryMiddlewareConvertsPanic(t *testing.T) {
	mw := recoveryMiddleware()
	wrapped := mw(func(ctx context.Context, req, resp any) error {
		panic("boom")
	})

	err := wrapped(context.Background(), nil, nil) // must not propagate the panic
	wantCode(t, err, errorkit.ERR_SYSTEM_UNEXPECTED)
}

func TestRecoveryMiddlewarePassesThrough(t *testing.T) {
	sentinel := errorkit.NewError(errorkit.ERR_VALIDATION)
	mw := recoveryMiddleware()

	called := false
	wrapped := mw(func(ctx context.Context, req, resp any) error {
		called = true
		return sentinel
	})

	if err := wrapped(context.Background(), nil, nil); err != sentinel {
		t.Fatalf("middleware altered a non-panic return: got %v", err)
	}
	if !called {
		t.Fatal("middleware did not invoke the next endpoint")
	}
	var _ endpoint.Endpoint = wrapped
}
