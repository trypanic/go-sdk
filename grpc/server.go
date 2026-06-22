package grpc

import (
	"context"
	"net"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"

	"github.com/trypanic/go-sdk/errorkit"
)

// Server wraps a *grpc.Server pre-wired with the SDK's tracing, panic
// recovery, keepalive, and lifecycle conventions. Construct one with New,
// register your generated services on Registrar(), then Serve a listener.
type Server struct {
	srv *grpc.Server
	cfg ServerConfig
}

// New builds a gRPC server from cfg and opts. By default tracing and panic
// recovery are enabled; see the With* options to adjust. It does not bind a
// port — pass a listener to Serve.
func New(cfg ServerConfig, opts ...ServerOption) (*Server, error) {
	if cfg.Address == "" {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("grpc: server address is required"),
		)
	}

	b := defaultServerBuild()
	for _, o := range opts {
		o(b)
	}

	var so []grpc.ServerOption

	if cfg.Keepalive.hasParams() {
		so = append(so, grpc.KeepaliveParams(cfg.Keepalive.serverParameters()))
	}
	if cfg.Keepalive.hasEnforcement() {
		so = append(so, grpc.KeepaliveEnforcementPolicy(cfg.Keepalive.enforcementPolicy()))
	}
	if cfg.MaxRecvMsgSize > 0 {
		so = append(so, grpc.MaxRecvMsgSize(cfg.MaxRecvMsgSize))
	}
	if cfg.MaxSendMsgSize > 0 {
		so = append(so, grpc.MaxSendMsgSize(cfg.MaxSendMsgSize))
	}

	// Interceptor chains: recovery runs outermost (so it also catches panics
	// from user interceptors), then the user-supplied interceptors in order.
	var unary []grpc.UnaryServerInterceptor
	var stream []grpc.StreamServerInterceptor
	if b.recovery {
		unary = append(unary, recoveryUnaryInterceptor(b.log))
		stream = append(stream, recoveryStreamInterceptor(b.log))
	}
	unary = append(unary, b.unary...)
	stream = append(stream, b.stream...)
	if len(unary) > 0 {
		so = append(so, grpc.ChainUnaryInterceptor(unary...))
	}
	if len(stream) > 0 {
		so = append(so, grpc.ChainStreamInterceptor(stream...))
	}

	// Tracing is a stats handler (instruments all four call modes), wired
	// outside the interceptor chain.
	if b.tracing {
		var hopts []otelgrpc.Option
		if b.tracerProvider != nil {
			hopts = append(hopts, otelgrpc.WithTracerProvider(b.tracerProvider))
		}
		if b.propagator != nil {
			hopts = append(hopts, otelgrpc.WithPropagators(b.propagator))
		}
		so = append(so, grpc.StatsHandler(otelgrpc.NewServerHandler(hopts...)))
	}

	so = append(so, b.raw...)

	return &Server{srv: grpc.NewServer(so...), cfg: cfg}, nil
}

// Registrar exposes the underlying server as a grpc.ServiceRegistrar so
// consumers can register their generated services:
//
//	pb.RegisterEchoServer(srv.Registrar(), impl)
func (s *Server) Registrar() grpc.ServiceRegistrar { return s.srv }

// Serve starts serving on lis and blocks until the server stops. The caller
// owns the listener (e.g. net.Listen("tcp", cfg.Address) or a bufconn).
func (s *Server) Serve(lis net.Listener) error {
	if err := s.srv.Serve(lis); err != nil {
		return errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).With(
			errorkit.WithReason("grpc: serve: %v", err),
			errorkit.WithWrapped(err),
		)
	}
	return nil
}

// Shutdown gracefully stops the server, draining in-flight RPCs and streams.
// If ctx expires first, it falls back to a hard Stop and returns a timeout
// error. Long-lived streams must be cancelled by the application for a
// graceful drain to complete within the deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.srv.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		s.srv.Stop()
		return errorkit.NewError(errorkit.ERR_SYSTEM_TIMEOUT_INTERNAL).With(
			errorkit.WithReason("grpc: graceful shutdown deadline exceeded; forced stop"),
			errorkit.WithWrapped(ctx.Err()),
		)
	}
}

// Stop hard-stops the server immediately, cancelling in-flight RPCs.
func (s *Server) Stop() { s.srv.Stop() }
