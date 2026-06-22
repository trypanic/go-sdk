package grpc

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/trypanic/go-sdk/errorkit"
)

// Dial creates a *grpc.ClientConn for cfg.Target, wired with the SDK's tracing
// and keepalive conventions. The caller wraps the returned connection with a
// generated client (e.g. pb.NewEchoClient(cc)) and is responsible for
// cc.Close().
//
// Transport security defaults to insecure when cfg.Creds is nil; set cfg.Creds
// to wire TLS. The connection is lazy — grpc.NewClient does not connect until
// the first RPC.
func Dial(cfg ClientConfig, opts ...ClientOption) (*grpc.ClientConn, error) {
	if cfg.Target == "" {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("grpc: client target is required"),
		)
	}

	b := defaultClientBuild()
	for _, o := range opts {
		o(b)
	}

	creds := cfg.Creds
	if creds == nil {
		creds = insecure.NewCredentials()
	}

	do := []grpc.DialOption{grpc.WithTransportCredentials(creds)}

	if cfg.Keepalive.isSet() {
		do = append(do, grpc.WithKeepaliveParams(cfg.Keepalive.clientParameters()))
	}
	if cfg.MaxRecvMsgSize > 0 || cfg.MaxSendMsgSize > 0 {
		var co []grpc.CallOption
		if cfg.MaxRecvMsgSize > 0 {
			co = append(co, grpc.MaxCallRecvMsgSize(cfg.MaxRecvMsgSize))
		}
		if cfg.MaxSendMsgSize > 0 {
			co = append(co, grpc.MaxCallSendMsgSize(cfg.MaxSendMsgSize))
		}
		do = append(do, grpc.WithDefaultCallOptions(co...))
	}
	if len(b.unary) > 0 {
		do = append(do, grpc.WithChainUnaryInterceptor(b.unary...))
	}
	if len(b.stream) > 0 {
		do = append(do, grpc.WithChainStreamInterceptor(b.stream...))
	}
	if b.tracing {
		var hopts []otelgrpc.Option
		if b.tracerProvider != nil {
			hopts = append(hopts, otelgrpc.WithTracerProvider(b.tracerProvider))
		}
		if b.propagator != nil {
			hopts = append(hopts, otelgrpc.WithPropagators(b.propagator))
		}
		do = append(do, grpc.WithStatsHandler(otelgrpc.NewClientHandler(hopts...)))
	}

	do = append(do, b.raw...)

	cc, err := grpc.NewClient(cfg.Target, do...)
	if err != nil {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).With(
			errorkit.WithReason("grpc: dial %q: %v", cfg.Target, err),
			errorkit.WithWrapped(err),
		)
	}
	return cc, nil
}
