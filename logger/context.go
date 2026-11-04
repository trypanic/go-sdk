package logger

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/trypanic/go-sdk/internal/logctx"
)

// Ctx extracts the logger from the context.
// This is a direct alias to zerolog.Ctx for convenience.
//
// If no logger exists in the context, it returns a disabled logger.
// To get a fallback to the global logger, use CtxOrGlobal instead.
//
// Usage:
//
//	logger.Ctx(ctx).Info().Msg("message")
func Ctx(ctx context.Context) *zerolog.Logger {
	return zerolog.Ctx(ctx)
}

// CtxOrGlobal extracts the logger from context, falling back to
// the global logger if none exists in the context.
//
// Usage:
//
//	logger.CtxOrGlobal(ctx).Info().Msg("message")
func CtxOrGlobal(ctx context.Context) *zerolog.Logger {
	return logctx.CtxOrGlobal(ctx)
}

// WithLogger stores a logger in the context.
//
// Usage:
//
//	ctx = logger.WithLogger(ctx, myLogger)
func WithLogger(ctx context.Context, l *zerolog.Logger) context.Context {
	return l.WithContext(ctx)
}

// Framework-specific middlewares (e.g. Hertz request logger) live in their
// respective adapter packages so this logger has zero web-framework imports
// and can be reused in any project.
