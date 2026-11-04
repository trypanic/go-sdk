package hertz

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/middlewares/server/recovery"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	hertztracing "github.com/hertz-contrib/obs-opentelemetry/tracing"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpserver"
	"github.com/trypanic/go-sdk/internal/redact"
	"github.com/trypanic/go-sdk/logger"
)

// New builds a Hertz-backed httpserver.Server using the compatibility
// profile (every built-in middleware on, audit redacted by default,
// 404/405 for unknown routes/methods, wall-clock Reply timestamp).
func New(cfg httpserver.ServerConfig) *Server {
	return NewWithOptions(cfg, httpserver.DefaultServerOptions())
}

// NewWithOptions builds a Hertz-backed httpserver.Server with explicit
// middleware switches. Pass DefaultServerOptions() and override fields for
// fine-grained control.
func NewWithOptions(cfg httpserver.ServerConfig, srvOpts httpserver.ServerOptions) *Server {
	return newServer(cfg, srvOpts, nil)
}

func newServer(cfg httpserver.ServerConfig, srvOpts httpserver.ServerOptions, base *logger.Logger) *Server {
	srvOpts = applyOptionDefaults(srvOpts)

	opts := []config.Option{
		server.WithHostPorts(cfg.Address()),
		// The caller's context bounds Shutdown; Hertz's own cap (5 s by
		// default) would cut the drain short and still report success.
		server.WithExitWaitTime(shutdownWaitCeiling),
		// Without it Hertz answers an unknown method with the NoRoute
		// response and never calls the NoMethod handler.
		server.WithHandleMethodNotAllowed(srvOpts.EnableNoMethod),
	}
	if cfg.ReadTimeout > 0 {
		opts = append(opts, server.WithReadTimeout(cfg.ReadTimeout))
	}
	if cfg.WriteTimeout > 0 {
		opts = append(opts, server.WithWriteTimeout(cfg.WriteTimeout))
	}

	var tracerCfg *hertztracing.Config
	if srvOpts.EnableTracing {
		tracerOpt, cfgT := hertztracing.NewServerTracer()
		tracerCfg = cfgT
		opts = append([]config.Option{tracerOpt}, opts...)
	}
	// server.New, not server.Default: Default always adds Hertz's own
	// recovery, which would ignore EnableRecovery and log panics through
	// hlog instead of the SDK logger.
	engine := server.New(opts...)

	// The SDK recovery is the outermost middleware, so it also covers
	// panics in the built-in middlewares below.
	if srvOpts.EnableRecovery {
		handler := RecoveryHandler
		if base != nil {
			handler = func(ctx context.Context, c *app.RequestContext, err any, stack []byte) {
				RecoveryHandler(base.IntoContext(ctx), c, err, stack)
			}
		}
		engine.Use(recovery.Recovery(recovery.WithRecoveryHandler(handler)))
	}
	if base != nil {
		engine.Use(RequestLoggerMiddleware(base))
	}
	if srvOpts.EnableTracing {
		engine.Use(hertztracing.ServerMiddleware(tracerCfg))
	}

	w := &Server{
		engine:    engine,
		config:    cfg,
		replyOpts: srvOpts.Reply,
	}
	engine.Use(w.trackInFlight)
	if srvOpts.EnableAudit {
		engine.Use(hertzInboundAuditMiddleware(srvOpts.Audit))
	}

	if srvOpts.EnableNoRoute {
		engine.NoRoute(noRouteHandler(srvOpts))
	}
	if srvOpts.EnableNoMethod {
		engine.NoMethod(noMethodHandler(srvOpts))
	}
	if srvOpts.EnableHealth {
		engine.GET(srvOpts.HealthPath, health(srvOpts.HealthCheck))
	}

	return w
}

// applyOptionDefaults fills non-boolean defaults. Boolean zero values remain
// disabled in NewWithOptions; NewServer starts with all defaults enabled.
func applyOptionDefaults(o httpserver.ServerOptions) httpserver.ServerOptions {
	d := httpserver.DefaultServerOptions()
	if o.NoRouteStatus == 0 {
		o.NoRouteStatus = d.NoRouteStatus
	}
	if o.NoMethodStatus == 0 {
		o.NoMethodStatus = d.NoMethodStatus
	}
	if o.Audit.BodyRedactor == nil {
		o.Audit.BodyRedactor = d.Audit.BodyRedactor
	}
	if o.HealthPath == "" {
		o.HealthPath = d.HealthPath
	}
	if o.Reply.Layout == "" {
		o.Reply.Layout = d.Reply.Layout
	}
	if o.Reply.Clock == nil {
		o.Reply.Clock = d.Reply.Clock
	}
	return o
}

// RecoveryHandler is the Hertz panic-recovery handler installed, as the
// outermost middleware, when ServerOptions.EnableRecovery is true. It logs
// one structured error line (with trace_id/span_id and the panic stack in
// "panic_stack") through logger.CtxOrGlobal(ctx) and aborts with HTTP 500.
func RecoveryHandler(ctx context.Context, c *app.RequestContext, err any, stack []byte) {
	appErr := errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).With(
		errorkit.WithReason("panic recovered: %v", err),
	)
	event := logger.WithTrace(ctx, logger.CtxOrGlobal(ctx).Error())
	logger.WithErrorKit(event, appErr).
		Str("panic_stack", string(stack)).
		Msg("httpserver: panic recovered")
	c.AbortWithStatus(consts.StatusInternalServerError)
}

func noRouteHandler(o httpserver.ServerOptions) app.HandlerFunc {
	if o.NoRouteHandler != nil {
		return wrapHandlerFunc(o)
	}
	status := o.NoRouteStatus
	return func(ctx context.Context, c *app.RequestContext) {
		c.AbortWithStatus(status)
	}
}

func noMethodHandler(o httpserver.ServerOptions) app.HandlerFunc {
	if o.NoMethodHandler != nil {
		return wrapMethodHandlerFunc(o)
	}
	status := o.NoMethodStatus
	return func(ctx context.Context, c *app.RequestContext) {
		c.AbortWithStatus(status)
	}
}

func wrapHandlerFunc(o httpserver.ServerOptions) app.HandlerFunc {
	h := o.NoRouteHandler
	replyOpts := o.Reply
	return func(ctx context.Context, c *app.RequestContext) {
		hctx := &hertzContext{ctx: ctx, requestCtx: c, replyOpts: replyOpts}
		h(ctx, hctx)
	}
}

func wrapMethodHandlerFunc(o httpserver.ServerOptions) app.HandlerFunc {
	h := o.NoMethodHandler
	replyOpts := o.Reply
	return func(ctx context.Context, c *app.RequestContext) {
		hctx := &hertzContext{ctx: ctx, requestCtx: c, replyOpts: replyOpts}
		h(ctx, hctx)
	}
}

// hertzInboundAuditMiddleware logs one structured entry per inbound request
// through logger.CtxOrGlobal(ctx), with trace_id/span_id from the active
// span. It records the method, the path (never the query string), the
// status, the headers with credential values masked by the shared redact
// rules, the bodies and the duration.
//
// Bodies follow AuditOptions: with CaptureBodies=false (default) every
// non-empty body is logged as "[REDACTED]" whatever BodyRedactor is set;
// with CaptureBodies=true they pass through BodyRedactor. Streamed bodies
// are never read and are logged as "[STREAM]".
func hertzInboundAuditMiddleware(audit httpserver.AuditOptions) app.HandlerFunc {
	redactBody := httpserver.BodyRedactor(httpserver.DefaultBodyRedactor)
	if audit.CaptureBodies && audit.BodyRedactor != nil {
		redactBody = audit.BodyRedactor
	}
	return func(ctx context.Context, c *app.RequestContext) {
		start := time.Now()

		c.Next(ctx)

		reqHeaders := map[string]string{}
		c.Request.Header.VisitAll(func(key, value []byte) {
			reqHeaders[string(key)] = inboundHeaderValue(string(key), string(value))
		})
		respHeaders := map[string]string{}
		c.Response.Header.VisitAll(func(key, value []byte) {
			respHeaders[string(key)] = inboundHeaderValue(string(key), string(value))
		})
		// Hertz keeps Set-Cookie outside VisitAll.
		c.Response.Header.VisitAllCookie(func(key, _ []byte) {
			respHeaders["Set-Cookie"] = redact.Placeholder
		})
		reqHeadersJSON, _ := json.Marshal(reqHeaders)
		respHeadersJSON, _ := json.Marshal(respHeaders)

		reqBody := auditBody(c.Request.IsBodyStream(), c.Request.BodyBytes(), redactBody)
		respBody := auditBody(c.Response.IsBodyStream(), c.Response.BodyBytes(), redactBody)

		logger.
			WithTrace(ctx, logger.CtxOrGlobal(ctx).Info()).
			Str("event", "http.audit.inbound").
			Str("method", string(c.Method())).
			Str("path", string(c.Path())).
			Int("status_code", c.Response.StatusCode()).
			RawJSON("request.headers", reqHeadersJSON).
			Str("request.body", reqBody).
			RawJSON("response.headers", respHeadersJSON).
			Str("response.body", respBody).
			Int64("duration_ms", time.Since(start).Milliseconds()).
			Msg("http: inbound audit")
	}
}

// auditBody returns the log form of a body. Streams are not consumed.
func auditBody(stream bool, body []byte, redactBody httpserver.BodyRedactor) string {
	if stream {
		return "[STREAM]"
	}
	return string(redactBody(body))
}

// isInboundSensitiveHeader reports whether an inbound header may carry a
// credential; it applies the rules shared with httprequest.
func isInboundSensitiveHeader(key string) bool {
	return redact.IsSensitiveHeader(key)
}

// inboundHeaderValue returns the log form of an inbound header value.
func inboundHeaderValue(key, value string) string {
	return redact.HeaderValue(key, value)
}

// health answers the built-in health endpoint. With a check it is a
// readiness probe; the check error is logged, never sent to the client.
func health(check func(context.Context) error) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if check != nil {
			if err := check(ctx); err != nil {
				appErr := errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).With(
					errorkit.WithReason("health check failed"),
					errorkit.WithWrapped(err),
				)
				logger.WarnCtx(ctx, appErr, "httpserver: health check failed")
				c.JSON(consts.StatusServiceUnavailable, map[string]any{"status": "unhealthy"})
				return
			}
		}
		c.JSON(consts.StatusOK, map[string]any{"status": "healthy"})
	}
}

// NewEngine creates a concrete server with direct access to its Hertz engine.
func NewEngine(cfg httpserver.ServerConfig) *Server { return New(cfg) }
