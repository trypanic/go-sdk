package httpserver

import "time"

// BodyRedactor turns a raw HTTP body into a representation safe for logs.
// The default redactor replaces any non-empty body with `[REDACTED]`.
type BodyRedactor func([]byte) []byte

// DefaultBodyRedactor returns `[REDACTED]` for any non-empty input.
func DefaultBodyRedactor(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return []byte("[REDACTED]")
}

// RawBodyRedactor returns its input verbatim. Use only when bodies are
// known not to contain secrets or PII.
func RawBodyRedactor(b []byte) []byte { return b }

// AuditOptions configures the inbound audit middleware.
//
// CaptureBodies: false (default) means request/response bodies are passed
// through BodyRedactor before being logged. Set to true with a custom
// BodyRedactor to capture bodies under an explicit policy. Setting
// CaptureBodies=true with the default redactor still logs `[REDACTED]`.
type AuditOptions struct {
	CaptureBodies bool
	BodyRedactor  BodyRedactor
}

// ReplyOptions configures the JSON envelope produced by HTTPContext.Reply.
type ReplyOptions struct {
	// Layout is the time.Format layout used for Reply.Timestamp.
	Layout string
	// Clock returns the time used for Reply.Timestamp. Override in tests
	// or to use a non-wall clock.
	Clock func() time.Time
}

// ServerOptions controls which built-in middlewares the SDK installs and
// how built-in defaults behave. The zero value is not meaningful; callers
// should start from DefaultServerOptions and override fields.
type ServerOptions struct {
	EnableTracing  bool
	EnableRecovery bool
	EnableAudit    bool
	EnableHealth   bool
	EnableNoRoute  bool
	EnableNoMethod bool

	// NoRouteStatus is the HTTP status returned by the default NoRoute
	// handler. Defaults to 404. Ignored when NoRouteHandler is non-nil.
	NoRouteStatus int
	// NoMethodStatus is the HTTP status returned by the default NoMethod
	// handler. Defaults to 405. Ignored when NoMethodHandler is non-nil.
	NoMethodStatus int
	// NoRouteHandler overrides the built-in NoRoute response.
	NoRouteHandler HandlerFunc
	// NoMethodHandler overrides the built-in NoMethod response.
	NoMethodHandler HandlerFunc

	Audit AuditOptions
	Reply ReplyOptions
}

// DefaultServerOptions returns the compatibility profile: every built-in
// middleware enabled, audit body capture OFF (redaction by default),
// 404/405 for unknown routes/methods, and a wall-clock Reply timestamp.
func DefaultServerOptions() ServerOptions {
	return ServerOptions{
		EnableTracing:  true,
		EnableRecovery: true,
		EnableAudit:    true,
		EnableHealth:   true,
		EnableNoRoute:  true,
		EnableNoMethod: true,
		NoRouteStatus:  404,
		NoMethodStatus: 405,
		Audit: AuditOptions{
			CaptureBodies: false,
			BodyRedactor:  DefaultBodyRedactor,
		},
		Reply: ReplyOptions{
			Layout: DateTimeLayout,
			Clock:  time.Now,
		},
	}
}
