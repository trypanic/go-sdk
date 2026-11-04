package otlp

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/trypanic/go-sdk/errorkit"
)

// fakeCollector is an in-process gRPC server on loopback that accepts any
// RPC and records the method and request metadata, standing in for an OTLP
// collector without network traffic outside the process.
type fakeCollector struct {
	addr string

	mu      sync.Mutex
	methods []string
	headers []metadata.MD
}

func startFakeCollector(t *testing.T) *fakeCollector {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	c := &fakeCollector{addr: lis.Addr().String()}
	srv := grpc.NewServer(grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		method, _ := grpc.MethodFromServerStream(stream)
		md, _ := metadata.FromIncomingContext(stream.Context())
		if err := stream.RecvMsg(&emptypb.Empty{}); err != nil {
			return err
		}
		c.mu.Lock()
		c.methods = append(c.methods, method)
		c.headers = append(c.headers, md)
		c.mu.Unlock()
		return stream.SendMsg(&emptypb.Empty{})
	}))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return c
}

func (c *fakeCollector) calls() ([]string, []metadata.MD) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.methods...), append([]metadata.MD(nil), c.headers...)
}

func restoreGlobalLoggerProvider(t *testing.T) {
	t.Helper()
	previous := otel.GetLoggerProvider()
	t.Cleanup(func() { otel.SetLoggerProvider(previous) })
}

func TestNewLogProviderRejectsInvalidConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  LogProviderConfig
	}{
		{name: "missing service name", cfg: LogProviderConfig{Endpoint: "127.0.0.1:4317", Insecure: true}},
		{name: "missing endpoint", cfg: LogProviderConfig{ServiceName: "svc", Insecure: true}},
		{name: "insecure with TLS config", cfg: LogProviderConfig{
			ServiceName: "svc", Endpoint: "127.0.0.1:4317", Insecure: true, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		}},
		{name: "negative timeout", cfg: LogProviderConfig{
			ServiceName: "svc", Endpoint: "127.0.0.1:4317", Insecure: true, Timeout: -time.Second,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restoreGlobalLoggerProvider(t)
			before := otel.GetLoggerProvider()

			handle, err := NewLogProvider(t.Context(), tc.cfg)

			require.Error(t, err)
			assert.Nil(t, handle)
			assert.True(t, errorkit.HasCode(err, errorkit.ERR_SYSTEM_CONFIG_INVALID), "error %v", err)
			assert.Equal(t, before, otel.GetLoggerProvider(), "global provider must not change on error")
		})
	}
}

func TestNewLogProviderAppliesHeadersAndExports(t *testing.T) {
	restoreGlobalLoggerProvider(t)
	collector := startFakeCollector(t)

	handle, err := NewLogProvider(t.Context(), LogProviderConfig{
		ServiceName: "svc",
		Endpoint:    collector.addr,
		Insecure:    true,
		Headers:     map[string]string{"x-api-key": "k-123"},
		Timeout:     5 * time.Second,
	})
	require.NoError(t, err)
	require.NotNil(t, handle)
	assert.Same(t, handle.provider, otel.GetLoggerProvider())

	_, err = NewOTLPWriter("svc").Write([]byte(`{"level":"info","message":"shipped"}`))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, handle.Close(ctx))

	methods, headers := collector.calls()
	require.Len(t, methods, 1)
	assert.Equal(t, "/opentelemetry.proto.collector.logs.v1.LogsService/Export", methods[0])
	assert.Equal(t, []string{"k-123"}, headers[0].Get("x-api-key"))
}

func TestNewLogProviderUsesTLSUnlessInsecure(t *testing.T) {
	restoreGlobalLoggerProvider(t)
	collector := startFakeCollector(t) // plaintext only

	handle, err := NewLogProvider(t.Context(), LogProviderConfig{
		ServiceName: "svc",
		Endpoint:    collector.addr,
		Timeout:     500 * time.Millisecond,
	})
	require.NoError(t, err)

	_, err = NewOTLPWriter("svc").Write([]byte(`{"level":"info","message":"tls only"}`))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_ = handle.Close(ctx)

	methods, _ := collector.calls()
	assert.Empty(t, methods, "a TLS exporter must not reach a plaintext collector")
}

func TestLogProviderHandleCloseReportsErrors(t *testing.T) {
	restoreGlobalLoggerProvider(t)
	handle, err := NewLogProvider(t.Context(), LogProviderConfig{
		ServiceName: "svc", Endpoint: "127.0.0.1:1", Insecure: true,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	closeErr := handle.Close(ctx)
	require.Error(t, closeErr)
	assert.ErrorIs(t, closeErr, context.Canceled)
	_, ok := errorkit.As(closeErr)
	assert.True(t, ok, "Close must return an *errorkit.AppError, got %T", closeErr)
}

func TestLogProviderHandleNilSafe(t *testing.T) {
	var handle *LogProviderHandle
	assert.NoError(t, handle.Close(t.Context()))
	handle.Shutdown(t.Context())
}
