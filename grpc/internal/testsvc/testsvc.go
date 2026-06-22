// Package testsvc provides a hand-authored grpc.ServiceDesc exercising all
// four gRPC call modes, using wrapperspb.StringValue as the request/response
// message so no protobuf codegen (and no protoc/buf binary) is required to
// test the parent grpc package.
package testsvc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Service and method names registered by Desc.
const (
	ServiceName        = "sdk.test.Echo"
	UnaryMethod        = "/sdk.test.Echo/Unary"
	ServerStreamMethod = "/sdk.test.Echo/ServerStream"
	ClientStreamMethod = "/sdk.test.Echo/ClientStream"
	BidiMethod         = "/sdk.test.Echo/Bidi"
)

// Stream descriptors for client-side cc.NewStream calls.
var (
	ServerStreamDesc = &grpc.StreamDesc{StreamName: "ServerStream", ServerStreams: true}
	ClientStreamDesc = &grpc.StreamDesc{StreamName: "ClientStream", ClientStreams: true}
	BidiStreamDesc   = &grpc.StreamDesc{StreamName: "Bidi", ServerStreams: true, ClientStreams: true}
)

// Impl is the server-side interface a test implements. Streaming methods get
// the raw grpc.ServerStream and read/write *wrapperspb.StringValue messages.
type Impl interface {
	Unary(ctx context.Context, in *wrapperspb.StringValue) (*wrapperspb.StringValue, error)
	ServerStream(stream grpc.ServerStream) error
	ClientStream(stream grpc.ServerStream) error
	Bidi(stream grpc.ServerStream) error
}

// Register installs the test service on reg backed by impl.
func Register(reg grpc.ServiceRegistrar, impl Impl) {
	reg.RegisterService(&desc, impl)
}

var desc = grpc.ServiceDesc{
	ServiceName: ServiceName,
	HandlerType: (*Impl)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "Unary", Handler: unaryHandler},
	},
	Streams: []grpc.StreamDesc{
		{StreamName: "ServerStream", Handler: serverStreamHandler, ServerStreams: true},
		{StreamName: "ClientStream", Handler: clientStreamHandler, ClientStreams: true},
		{StreamName: "Bidi", Handler: bidiHandler, ServerStreams: true, ClientStreams: true},
	},
	Metadata: "sdk/test/echo",
}

// unaryHandler mirrors the shape protoc-gen-go-grpc emits: decode the request,
// then call the impl through the interceptor chain so the parent package's
// recovery/tracing wiring runs.
// nosemgrep: context-must-be-first-param -- signature is dictated by grpc.MethodDesc.Handler (matches protoc-gen-go-grpc output); ctx cannot be first.
func unaryHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(wrapperspb.StringValue)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(Impl).Unary(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: UnaryMethod}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(Impl).Unary(ctx, req.(*wrapperspb.StringValue))
	}
	return interceptor(ctx, in, info, handler)
}

func serverStreamHandler(srv any, stream grpc.ServerStream) error {
	return srv.(Impl).ServerStream(stream)
}

func clientStreamHandler(srv any, stream grpc.ServerStream) error {
	return srv.(Impl).ClientStream(stream)
}

func bidiHandler(srv any, stream grpc.ServerStream) error {
	return srv.(Impl).Bidi(stream)
}
