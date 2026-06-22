package grpc

import (
	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/serviceinfo"
	"github.com/cloudwego/kitex/transport"
	"github.com/kitex-contrib/obs-opentelemetry/tracing"

	"github.com/trypanic/go-sdk/errorkit"
)

// DialOptions assembles the Kitex client options for a gRPC-transport client
// following the SDK conventions (gRPC transport, configured timeouts, opt-in
// OTel tracing). Pass the result straight to a codegen client constructor:
//
//	cli, err := echo.NewClient("echo-svc", sdkgrpc.DialOptions(cfg, sdkgrpc.DefaultClientOptions())...)
//
// This is the reusable unit; NewClient is a thin generic wrapper over it.
func DialOptions(cfg ClientConfig, opts ClientOptions) []client.Option {
	out := []client.Option{
		client.WithTransportProtocol(transport.GRPC),
		client.WithHostPorts(cfg.Hosts...),
	}
	if cfg.RPCTimeout > 0 {
		out = append(out, client.WithRPCTimeout(cfg.RPCTimeout))
	}
	if cfg.ConnectTimeout > 0 {
		out = append(out, client.WithConnectTimeout(cfg.ConnectTimeout))
	}
	if opts.EnableTracing {
		out = append(out, client.WithSuite(tracing.NewClientSuite()))
	}
	return out
}

// NewClient builds a generic Kitex client.Client for svcInfo over the gRPC
// transport with DefaultClientOptions. Most callers use a codegen-typed
// client instead — feed DialOptions to its NewClient. Use this for generic
// (reflection-style) calls.
func NewClient(svcInfo *serviceinfo.ServiceInfo, cfg ClientConfig) (client.Client, error) {
	return NewClientWithOptions(svcInfo, cfg, DefaultClientOptions())
}

// NewClientWithOptions is NewClient with explicit toggles.
func NewClientWithOptions(svcInfo *serviceinfo.ServiceInfo, cfg ClientConfig, opts ClientOptions) (client.Client, error) {
	if svcInfo == nil {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("grpc: svcInfo is required"),
		)
	}
	if len(cfg.Hosts) == 0 {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("grpc: at least one host is required"),
		)
	}

	cli, err := client.NewClient(svcInfo, DialOptions(cfg, opts)...)
	if err != nil {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).With(
			errorkit.WithReason("grpc: new client: %v", err),
			errorkit.WithWrapped(err),
		)
	}
	return cli, nil
}
