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

// New builds a Hertz-backed httpserver.HTTPServer using the compatibility
// profile (every built-in middleware on, audit redacted by default,
// 404/405 for unknown routes/methods, wall-clock Reply timestamp).
func New(cfg httpserver.ServerConfig) httpserver.HTTPServer {
	return NewWithOptions(cfg, httpserver.DefaultServerOptions())
}

// NewWithOptions builds a Hertz-backed httpserver.HTTPServer with explicit
// middleware switches. Pass DefaultServerOptions() and override fields for
// fine-grained control.
func NewWithOptions(cfg httpserver.ServerConfig, srvOpts httpserver.ServerOptions) httpserver.HTTPServer {
	srvOpts = applyOptionDefaults(srvOpts)

	opts := []config.Option{
		server.WithHostPorts(cfg.Address()),
		// The caller's context bounds Shutdown; Hertz's own cap (5 s by
		// default) would cut the drain short and still report success.
		server.WithExitWaitTime(shutdownWaitCeiling),
	}
	if cfg.ReadTimeout > 0 {
		opts = append(opts, server.WithReadTimeout(cfg.ReadTimeout))
	}
	if cfg.WriteTimeout > 0 {
		opts = append(opts, server.WithWriteTimeout(cfg.WriteTimeout))
	}

	var (
		engine    *server.Hertz
		tracerCfg *hertztracing.Config
	)
	if srvOpts.EnableTracing {
		tracerOpt, cfgT := hertztracing.NewServerTracer()
		tracerCfg = cfgT
		engine = server.Default(append([]config.Option{tracerOpt}, opts...)...)
		engine.Use(hertztracing.ServerMiddleware(tracerCfg))
	} else {
		engine = server.Default(opts...)
	}

	w := &wrapperHertzServer{
		engine:    engine,
		config:    cfg,
		replyOpts: srvOpts.Reply,
	}
	engine.Use(w.trackInFlight)

	if srvOpts.EnableRecovery {
		engine.Use(recovery.Recovery(recovery.WithRecoveryHandler(RecoveryHandler)))
	}
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
		engine.GET("/health", health())
	}

	return w
}

// applyOptionDefaults backfills any zero-valued fields in srvOpts with
// the values from DefaultServerOptions. This keeps NewWithOptions safe
// when callers construct ServerOptions partially.
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
	if o.Reply.Layout == "" {
		o.Reply.Layout = d.Reply.Layout
	}
	if o.Reply.Clock == nil {
		o.Reply.Clock = d.Reply.Clock
	}
	return o
}

// RecoveryHandler is the Hertz panic-recovery handler installed when
// ServerOptions.EnableRecovery is true.
func RecoveryHandler(ctx context.Context, c *app.RequestContext, err any, stack []byte) {
	appErr := errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).With(
		errorkit.WithReason("panic recovered: %v", err),
	)
	logger.ErrorCtx(ctx, appErr, "httpserver: panic recovered")
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
	if isInboundSensitiveHeader(key) {
		return redact.Placeholder
	}
	return value
}

func health() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		c.JSON(200, map[string]any{"status": "healthy"})
	}
}
