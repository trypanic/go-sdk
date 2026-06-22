package grpc

// ServerOptions controls which built-in middlewares the SDK installs on a
// Kitex server. The zero value is not meaningful; start from
// DefaultServerOptions and override fields.
type ServerOptions struct {
	// EnableTracing installs the obs-opentelemetry server suite (global OTel
	// tracer + propagator). Spans are produced automatically per RPC.
	EnableTracing bool
	// EnableRecovery installs a middleware that recovers handler panics,
	// logs via the SDK logger, and returns an errorkit error.
	EnableRecovery bool
}

// DefaultServerOptions returns the compatibility profile: tracing and
// recovery both on.
func DefaultServerOptions() ServerOptions {
	return ServerOptions{
		EnableTracing:  true,
		EnableRecovery: true,
	}
}

// ClientOptions controls observability toggles on a Kitex client.
type ClientOptions struct {
	// EnableTracing installs the obs-opentelemetry client suite.
	EnableTracing bool
}

// DefaultClientOptions returns the compatibility profile: tracing on.
func DefaultClientOptions() ClientOptions {
	return ClientOptions{EnableTracing: true}
}
