package grpc

import (
	"context"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/trypanic/go-sdk/v2/errorkit"
)

// Dial creates a *grpc.ClientConn for cfg.Target, wired with the SDK's tracing
// and keepalive conventions. The caller wraps the returned connection with a
// generated client (e.g. pb.NewEchoClient(cc)) and is responsible for
// cc.Close().
//
// Transport security comes from cfg.Creds. A nil cfg.Creds is an
// ERR_SYSTEM_CONFIG_INVALID error unless WithClientInsecure states that the
// connection is plaintext on purpose. The connection is lazy — grpc.NewClient
// does not connect until the first RPC.
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
	switch {
	case cfg.Creds != nil && b.insecure:
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("grpc: client Creds and WithClientInsecure are mutually exclusive"),
		)
	case cfg.Creds == nil && !b.insecure:
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("grpc: client Creds is nil; set it, or pass WithClientInsecure for plaintext"),
		)
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
	unary, stream := b.unary, b.stream
	if b.defaultTimeout > 0 {
		unary = append([]grpc.UnaryClientInterceptor{defaultTimeoutUnary(b.defaultTimeout)}, unary...)
		stream = append([]grpc.StreamClientInterceptor{defaultTimeoutStream(b.defaultTimeout)}, stream...)
	}
	if len(unary) > 0 {
		do = append(do, grpc.WithChainUnaryInterceptor(unary...))
	}
	if len(stream) > 0 {
		do = append(do, grpc.WithChainStreamInterceptor(stream...))
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

// defaultTimeoutUnary applies timeout d to unary calls whose context has no
// deadline.
func defaultTimeoutUnary(d time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if _, ok := ctx.Deadline(); ok {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		ctx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// defaultTimeoutStream applies timeout d to streams whose context has no
// deadline. The timeout's cancel runs when the stream ends.
func defaultTimeoutStream(d time.Duration) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		if _, ok := ctx.Deadline(); ok {
			return streamer(ctx, desc, cc, method, opts...)
		}
		ctx, cancel := context.WithTimeout(ctx, d)
		cs, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			cancel()
			return nil, err
		}
		return &timeoutClientStream{ClientStream: cs, cancel: cancel, serverStreams: desc.ServerStreams}, nil
	}
}

// timeoutClientStream releases the default-timeout context once the stream
// is finished: on any RecvMsg error (io.EOF included) or, for streams with a
// single response, after that response is received.
type timeoutClientStream struct {
	grpc.ClientStream
	cancel        context.CancelFunc
	serverStreams bool
}

func (s *timeoutClientStream) RecvMsg(m any) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil || !s.serverStreams {
		s.cancel()
	}
	return err
}
