// Package grpc provides server and client factories for CloudWeGo Kitex
// speaking the gRPC transport, wired with the SDK's tracing
// (kitex-contrib/obs-opentelemetry), panic recovery, and errorkit
// conventions. It is independent of httpserver/hertz.
//
// Kitex is codegen-bound: the *serviceinfo.ServiceInfo and handler passed to
// New come from `kitex` IDL generation. This package does not generate code;
// it standardizes how the generated server/client are constructed.
package grpc

import (
	"fmt"
	"time"
)

// ServerConfig holds Kitex (gRPC transport) server connection settings.
type ServerConfig struct {
	Host string
	Port int

	// ReadWriteTimeout bounds per-connection read/write. Zero = Kitex default.
	ReadWriteTimeout time.Duration
	// ExitWaitTime bounds the graceful drain performed on Stop. Zero = Kitex default.
	ExitWaitTime time.Duration
}

// Address returns the server address in host:port form.
func (c ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// ClientConfig holds Kitex (gRPC transport) client connection settings.
type ClientConfig struct {
	// Hosts are target host:port endpoints. Required (no service discovery
	// is wired here — pass resolved addresses).
	Hosts []string

	// RPCTimeout bounds a single call. Zero = Kitex default.
	RPCTimeout time.Duration
	// ConnectTimeout bounds dialing. Zero = Kitex default.
	ConnectTimeout time.Duration
}
