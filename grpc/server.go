package grpc

import (
	"context"
	"net"

	"github.com/cloudwego/kitex/pkg/endpoint"
	"github.com/cloudwego/kitex/pkg/serviceinfo"
	"github.com/cloudwego/kitex/server"
	"github.com/kitex-contrib/obs-opentelemetry/tracing"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/logger"
)

// Server wraps a Kitex server speaking the gRPC transport, wired with the
// SDK's tracing, panic recovery, and error conventions. Construct one per
// process and share it; Run blocks, Stop drains gracefully.
type Server struct {
	svr server.Server
	cfg ServerConfig
}

// New builds a Kitex gRPC server with DefaultServerOptions (tracing +
// recovery on) and registers the codegen-produced svcInfo/handler pair.
// Kitex auto-detects gRPC traffic for protobuf services, so no explicit
// transport switch is required server-side.
func New(cfg ServerConfig, svcInfo *serviceinfo.ServiceInfo, handler any) (*Server, error) {
	return NewWithOptions(cfg, svcInfo, handler, DefaultServerOptions())
}

// NewWithOptions is New with explicit middleware toggles.
func NewWithOptions(cfg ServerConfig, svcInfo *serviceinfo.ServiceInfo, handler any, opts ServerOptions) (*Server, error) {
	if svcInfo == nil || handler == nil {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("grpc: svcInfo and handler are required"),
		)
	}

	addr, err := net.ResolveTCPAddr("tcp", cfg.Address())
	if err != nil {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("grpc: invalid server address %q: %v", cfg.Address(), err),
			errorkit.WithWrapped(err),
		)
	}

	srvOpts := []server.Option{server.WithServiceAddr(addr)}
	if cfg.ReadWriteTimeout > 0 {
		srvOpts = append(srvOpts, server.WithReadWriteTimeout(cfg.ReadWriteTimeout))
	}
	if cfg.ExitWaitTime > 0 {
		srvOpts = append(srvOpts, server.WithExitWaitTime(cfg.ExitWaitTime))
	}
	if opts.EnableTracing {
		srvOpts = append(srvOpts, server.WithSuite(tracing.NewServerSuite()))
	}
	if opts.EnableRecovery {
		srvOpts = append(srvOpts, server.WithMiddleware(recoveryMiddleware()))
	}

	svr := server.NewServer(srvOpts...)
	if err := svr.RegisterService(svcInfo, handler); err != nil {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).With(
			errorkit.WithReason("grpc: register service: %v", err),
			errorkit.WithWrapped(err),
		)
	}

	return &Server{svr: svr, cfg: cfg}, nil
}

// Run starts the server and blocks until it stops or fails.
func (s *Server) Run() error {
	if err := s.svr.Run(); err != nil {
		return errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).With(
			errorkit.WithReason("grpc: server run: %v", err),
			errorkit.WithWrapped(err),
		)
	}
	return nil
}

// Stop drains in-flight RPCs (bounded by ServerConfig.ExitWaitTime) and
// shuts the server down. Kitex's Stop takes no context; configure the drain
// budget via ExitWaitTime.
func (s *Server) Stop() error {
	if err := s.svr.Stop(); err != nil {
		return errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).With(
			errorkit.WithReason("grpc: server stop: %v", err),
			errorkit.WithWrapped(err),
		)
	}
	return nil
}

// recoveryMiddleware recovers handler panics, logs via the SDK logger, and
// returns an errorkit error instead of letting the panic escape.
func recoveryMiddleware() endpoint.Middleware {
	return func(next endpoint.Endpoint) endpoint.Endpoint {
		return func(ctx context.Context, req, resp any) (err error) {
			defer func() {
				if r := recover(); r != nil {
					appErr := errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).With(
						errorkit.WithReason("panic recovered: %v", r),
					)
					logger.ErrorCtx(ctx, appErr, "grpc: panic recovered")
					err = appErr
				}
			}()
			return next(ctx, req, resp)
		}
	}
}
