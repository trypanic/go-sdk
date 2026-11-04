package hertz

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/rs/zerolog"

	"github.com/trypanic/go-sdk/httpserver"
	"github.com/trypanic/go-sdk/logger"
)

// RequestIDHeader carries the request ID in and out: an inbound value is
// reused (when valid) and the ID in use is echoed in the response.
const RequestIDHeader = "X-Request-ID"

// maxRequestIDLen bounds an inbound request ID so a client cannot inflate
// every log line.
const maxRequestIDLen = 128

// RequestLogger returns an httpserver.MiddlewareFunc, installable with
// HTTPServer.Use, that injects a request-scoped logger into ctx. See
// RequestLoggerMiddleware for the fields and the request ID rules.
func RequestLogger(base *logger.Logger) httpserver.MiddlewareFunc {
	return func(ctx context.Context, c httpserver.HTTPContext, next httpserver.HandlerFunc) {
		hc, ok := c.(*hertzContext)
		if !ok {
			next(ctx, c)
			return
		}
		next(withRequestLogger(ctx, hc.requestCtx, base), c)
	}
}

// RequestLoggerMiddleware injects a request-scoped logger into ctx, with
// request_id, method, path and remote_addr fields. The request ID is the
// inbound X-Request-ID when it is 1-128 characters of [A-Za-z0-9._:-],
// otherwise a new random one; it is echoed in the X-Request-ID response
// header. Pass nil for base to derive from the context or global logger.
// Use RequestLogger to install it through HTTPServer.Use.
func RequestLoggerMiddleware(base *logger.Logger) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		c.Next(withRequestLogger(ctx, c, base))
	}
}

// withRequestLogger resolves the request ID, echoes it and returns ctx
// carrying the request-scoped logger.
func withRequestLogger(ctx context.Context, c *app.RequestContext, base *logger.Logger) context.Context {
	reqID := string(c.GetHeader(RequestIDHeader))
	if !validRequestID(reqID) {
		reqID = generateRequestID()
	}
	c.Response.Header.Set(RequestIDHeader, reqID)

	var withCtx zerolog.Context
	if base != nil {
		withCtx = base.Zerolog().With()
	} else {
		withCtx = logger.CtxOrGlobal(ctx).With()
	}

	reqLogger := withCtx.
		Str("request_id", reqID).
		Str("method", string(c.Method())).
		Str("path", string(c.Path())).
		Str("remote_addr", c.RemoteAddr().String()).
		Logger()

	return reqLogger.WithContext(ctx)
}

// validRequestID accepts short IDs made of URL- and log-safe characters.
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == ':':
		default:
			return false
		}
	}
	return true
}

func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}
