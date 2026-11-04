package messaging

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/trypanic/go-sdk/logger"
)

// warnEvent starts a Warn event on l, the WithLogger logger, or on the
// ctx/global logger when l is nil, with the trace IDs of ctx attached.
// Callers add structured fields and end it with a constant message.
func warnEvent(ctx context.Context, l *logger.Logger) *zerolog.Event {
	zl := logger.CtxOrGlobal(ctx)
	if l != nil {
		zl = l.Zerolog()
	}
	return logger.WithTrace(ctx, zl.Warn())
}

// errorEvent is warnEvent at Error level.
func errorEvent(ctx context.Context, l *logger.Logger) *zerolog.Event {
	zl := logger.CtxOrGlobal(ctx)
	if l != nil {
		zl = l.Zerolog()
	}
	return logger.WithTrace(ctx, zl.Error())
}
