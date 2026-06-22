// Package grpc provides reusable server and client factories built on
// google.golang.org/grpc, wired with the SDK's standard cross-cutting
// concerns: OpenTelemetry tracing (via the otelgrpc stats handler, which
// instruments unary AND every streaming mode), panic recovery on unary and
// streaming handlers, keepalive, graceful lifecycle, and errorkit-based
// error mapping to gRPC status codes.
//
// All four gRPC call modes — unary, server-streaming, client-streaming, and
// bidirectional-streaming — are supported equally; consumers register their
// generated service stubs and get the SDK wiring applied to all of them.
//
// # Codegen
//
// This package does not generate code. Generate your service stubs with the
// ecosystem-standard Go toolchain — protoc-gen-go + protoc-gen-go-grpc (or
// `buf generate`). No bespoke or non-Go binary (such as the old `kitex` CLI)
// is required beyond the standard protoc toolchain.
//
// # Import alias
//
// The package identifier is grpc, which collides with the upstream
// google.golang.org/grpc package. Consumers should alias this package, e.g.:
//
//	import sdkgrpc "github.com/trypanic/go-sdk/grpc"
package grpc
