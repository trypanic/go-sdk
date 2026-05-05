package hertz

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/middlewares/server/recovery"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	hertztracing "github.com/hertz-contrib/obs-opentelemetry/tracing"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpserver"
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

	opts := []config.Option{server.WithHostPorts(cfg.Address())}
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

	return &wrapperHertzServer{
		engine:    engine,
		config:    cfg,
		replyOpts: srvOpts.Reply,
	}
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
		hctx := &hertzContext{requestCtx: c, replyOpts: replyOpts}
		h(ctx, hctx)
	}
}

func wrapMethodHandlerFunc(o httpserver.ServerOptions) app.HandlerFunc {
	h := o.NoMethodHandler
	replyOpts := o.Reply
	return func(ctx context.Context, c *app.RequestContext) {
		hctx := &hertzContext{requestCtx: c, replyOpts: replyOpts}
		h(ctx, hctx)
	}
}

// hertzInboundAuditMiddleware logs one structured entry per inbound request.
// Bodies pass through audit.BodyRedactor; CaptureBodies=false (default)
// combined with DefaultBodyRedactor logs `[REDACTED]` for any non-empty body.
func hertzInboundAuditMiddleware(audit httpserver.AuditOptions) app.HandlerFunc {
	redact := audit.BodyRedactor
	if redact == nil {
		redact = httpserver.DefaultBodyRedactor
	}
	return func(ctx context.Context, c *app.RequestContext) {
		start := time.Now()

		method := string(c.Method())
		path := string(c.Path())

		reqBodyRaw := c.Request.Body()
		reqBody := make([]byte, len(reqBodyRaw))
		copy(reqBody, reqBodyRaw)

		reqHeaders := make(map[string]string)
		c.Request.Header.VisitAll(func(key, value []byte) {
			if !isInboundSensitiveHeader(string(key)) {
				reqHeaders[string(key)] = string(value)
			}
		})

		c.Next(ctx)

		statusCode := c.Response.StatusCode()
		respBody := c.Response.Body()

		respHeaders := make(map[string]string)
		c.Response.Header.VisitAll(func(key, value []byte) {
			if !isInboundSensitiveHeader(string(key)) {
				respHeaders[string(key)] = string(value)
			}
		})

		reqHeadersJSON, _ := json.Marshal(reqHeaders)
		respHeadersJSON, _ := json.Marshal(respHeaders)

		reqBodyOut := redact(reqBody)
		respBodyOut := redact(respBody)

		logger.
			WithTrace(ctx, logger.CtxOrGlobal(ctx).Info()).
			Str("event", "http.audit.inbound").
			Str("method", method).
			Str("path", path).
			Int("status_code", statusCode).
			RawJSON("request.headers", reqHeadersJSON).
			Str("request.body", string(reqBodyOut)).
			RawJSON("response.headers", respHeadersJSON).
			Str("response.body", string(respBodyOut)).
			Int64("duration_ms", time.Since(start).Milliseconds()).
			Msg("http: inbound audit")
	}
}

var sensitiveInboundHeaderKeys = map[string]struct{}{
	"authorization":  {},
	"x-api-key":      {},
	"api-key":        {},
	"cookie":         {},
	"set-cookie":     {},
	"x-auth-token":   {},
	"x-access-token": {},
}

func isInboundSensitiveHeader(key string) bool {
	_, ok := sensitiveInboundHeaderKeys[strings.ToLower(key)]
	return ok
}

func health() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		c.JSON(200, map[string]any{"status": "healthy"})
	}
}
