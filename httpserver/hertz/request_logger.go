package hertz

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/rs/zerolog"

	"github.com/trypanic/go-sdk/logger"
)

// RequestLoggerMiddleware injects a request-scoped logger into ctx.
// Pass nil for base to derive from the global logger.
func RequestLoggerMiddleware(base *logger.Logger) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		reqID := generateRequestID()

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

		ctx = reqLogger.WithContext(ctx)
		c.Next(ctx)
	}
}

func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}
