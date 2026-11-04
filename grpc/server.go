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
// register your generated services on Registrar(), then call ListenAndServe
// (or Serve with your own listener).
type Server struct {
	srv *grpc.Server
	cfg ServerConfig
}

// New builds a gRPC server from cfg and opts. By default tracing and panic
// recovery are enabled; see the With* options to adjust. It does not bind a
// port: ListenAndServe listens on cfg.Address, Serve takes a listener.
//
// Transport security comes from cfg.Creds. A nil cfg.Creds is an
// ERR_SYSTEM_CONFIG_INVALID error unless WithServerInsecure states that the
// server accepts plaintext connections on purpose.
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
	switch {
	case cfg.Creds != nil && b.insecure:
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("grpc: server Creds and WithServerInsecure are mutually exclusive"),
		)
	case cfg.Creds == nil && !b.insecure:
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("grpc: server Creds is nil; set it, or pass WithServerInsecure to accept plaintext"),
		)
	}

	var so []grpc.ServerOption
	if cfg.Creds != nil {
		so = append(so, grpc.Creds(cfg.Creds))
	}

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
	if cfg.MaxConcurrentStreams > 0 {
		so = append(so, grpc.MaxConcurrentStreams(cfg.MaxConcurrentStreams))
	}

	// Interceptor chains: recovery runs first (so it also catches panics
	// from the WithUnary/StreamInterceptors interceptors), then those in
	// order. A raw grpc.UnaryInterceptor/StreamInterceptor passed through
	// WithRawServerOptions is prepended by grpc-go and runs outside recovery.
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
		if b.publicEndpoint {
			hopts = append(hopts, otelgrpc.WithPublicEndpoint())
		}
		hopts = append(hopts, b.otelOptions...)
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

// ListenAndServe listens on TCP at ServerConfig.Address and serves on that
// listener, blocking until the server stops (it returns nil after Shutdown or
// Stop). A listen failure, such as a port already in use, is returned at once
// as an ERR_SYSTEM_UNEXPECTED error that wraps the net error.
//
// Use Serve instead to serve on a listener you build yourself (a bufconn in
// tests, a socket inherited from systemd, a non-TCP network).
func (s *Server) ListenAndServe() error {
	lis, err := net.Listen("tcp", s.cfg.Address)
	if err != nil {
		return errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).With(
			errorkit.WithReason("grpc: listen on %q: %v", s.cfg.Address, err),
			errorkit.WithWrapped(err),
		)
	}
	return s.Serve(lis)
}

// Serve starts serving on lis and blocks until the server stops. The caller
// owns the listener (e.g. a bufconn in tests); ServerConfig.Address is not
// used. ListenAndServe listens on ServerConfig.Address for you.
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
