package hertz

import (
	"context"

	"github.com/trypanic/go-sdk/v2/httpserver"
	"github.com/trypanic/go-sdk/v2/logger"
)

// ServerOption customizes the default-on server profile.
type ServerOption func(*serverSettings)
type serverSettings struct {
	profile httpserver.ServerOptions
	logger  *logger.Logger
}

// NewServer builds a concrete Hertz adapter. With no options all middleware,
// health and 404/405 handling are enabled; bodies are redacted by default.
func NewServer(cfg httpserver.ServerConfig, opts ...ServerOption) *Server {
	settings := serverSettings{profile: httpserver.DefaultServerOptions()}
	for _, opt := range opts {
		opt(&settings)
	}
	return newServer(cfg, settings.profile, settings.logger)
}

// WithLogger installs a request logger and uses it for recovery and audit.
func WithLogger(base *logger.Logger) ServerOption {
	return func(s *serverSettings) { s.logger = base }
}

// WithHealth sets the readiness path and optional readiness check.
func WithHealth(path string, check func(context.Context) error) ServerOption {
	return func(s *serverSettings) {
		s.profile.HealthPath = path
		s.profile.HealthCheck = check
		s.profile.EnableHealth = true
	}
}

// WithAudit configures inbound capture and enables audit logging.
func WithAudit(options httpserver.AuditOptions) ServerOption {
	return func(s *serverSettings) { s.profile.Audit = options; s.profile.EnableAudit = true }
}

// WithReply configures response timestamps.
func WithReply(options httpserver.ReplyOptions) ServerOption {
	return func(s *serverSettings) { s.profile.Reply = options }
}

// WithoutTracing disables the built-in tracing behavior.
func WithoutTracing() ServerOption {
	return func(s *serverSettings) { s.profile.EnableTracing = false }
}

// WithoutRecovery disables the built-in recovery behavior.
func WithoutRecovery() ServerOption {
	return func(s *serverSettings) { s.profile.EnableRecovery = false }
}

// WithoutAudit disables the built-in audit behavior.
func WithoutAudit() ServerOption { return func(s *serverSettings) { s.profile.EnableAudit = false } }

// WithoutHealth disables the built-in health behavior.
func WithoutHealth() ServerOption { return func(s *serverSettings) { s.profile.EnableHealth = false } }

// WithoutNoRoute disables the built-in noroute behavior.
func WithoutNoRoute() ServerOption {
	return func(s *serverSettings) { s.profile.EnableNoRoute = false }
}

// WithoutNoMethod disables the built-in nomethod behavior.
func WithoutNoMethod() ServerOption {
	return func(s *serverSettings) { s.profile.EnableNoMethod = false }
}
