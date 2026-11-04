// Package httpclient builds production-tuned *http.Client values: a shared
// connection pool, dial/TLS/response-header timeouts, a TLS 1.3 minimum, an
// explicit redirect policy, and an opt-in TransportWrapper seam for tracing
// or metrics middleware (the default build has no telemetry imports).
//
// Build one client per logical upstream at startup and share it; every
// NewClient call creates a new connection pool. DefaultConfig suits
// service-to-service calls (60 s per-attempt timeout); SetupForLLM suits
// slow LLM providers and leaves the overall bound to the caller's context.
//
// Generic external-service error codes are owned by errorkit. This package
// retains equal constants for compatibility.
package httpclient
