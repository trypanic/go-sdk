package logger

import (
	"context"
	"os"

	"github.com/trypanic/go-sdk/errorkit"
)

// Error logs an ErrorKit error at Error level using the global logger.
//
// The msg parameter should describe WHAT HAPPENED, not repeat the error details.
//
// Good:
//
//	logger.Error(appErr, "Database connection failed")
//	logger.Error(appErr, "Failed to process order")
//
// Bad:
//
//	logger.Error(appErr, appErr.Error())  // ← Redundant with reason field
//	logger.Error(appErr, "Validation failed: invalid email")  // ← Details in reason already
//
// Usage:
//
//	logger.Error(appErr, "Database initialization failed")
func Error(appErr error, msg ...string) error {
	errorMsg := ""
	if len(msg) > 0 {
		errorMsg = msg[0]
	}

	if errkit, ok := appErr.(*errorkit.AppError); ok {
		// FIX: was .Msg("") — the msg argument was silently dropped for *AppError.
		WithErrorKit(globalLogger().Error(), errkit).Msg(errorMsg)
	} else {
		WithErrorKit(
			globalLogger().Error(),
			errorkit.NewError(errorkit.ERR_INTERNAL).With(errorkit.WithWrapped(appErr)),
		).Msg(errorMsg)
	}
	return appErr
}

func LogAndReturn(appErr error, msg ...string) error {
	Error(appErr, msg...)
	return appErr
}

func Panic(appErr error, msg ...string) {
	Error(appErr, msg...)
	os.Exit(1) // nosemgrep: no-os-exit-in-library -- named fatal helper; caller opts in by name
}

// Warn logs a warning message using the global logger.
//
// Usage:
//
//	logger.Warn("Cache miss — using fallback")
func Warn(msg string, args ...any) {
	globalLogger().Warn().Msgf(msg, args...)
}

// ErrorCtx logs an ErrorKit error at Error level using the logger from context,
// enriched with the active OpenTelemetry trace_id and span_id.
//
// This is the primary error logging function for service handlers.
// All log lines will be linkable to the corresponding trace in SigNoz.
//
// Usage:
//
//	logger.ErrorCtx(ctx, appErr, "Order processing failed")
func ErrorCtx(ctx context.Context, appErr *errorkit.AppError, msg string) {
	// FIX: WithTrace was missing — trace_id/span_id were never injected.
	event := WithTrace(ctx, CtxOrGlobal(ctx).Error())
	WithErrorKit(event, appErr).Msg(msg)
}

// WarnCtx logs an ErrorKit error at Warn level using the logger from context,
// enriched with the active OpenTelemetry trace_id and span_id.
//
// Use this for retriable / transient errors (e.g. timeouts, cache misses).
//
// Usage:
//
//	logger.WarnCtx(ctx, appErr, "Cache miss — using fallback")
func WarnCtx(ctx context.Context, appErr *errorkit.AppError, msg string) {
	// FIX: WithTrace was missing — trace_id/span_id were never injected.
	event := WithTrace(ctx, CtxOrGlobal(ctx).Warn())
	WithErrorKit(event, appErr).Msg(msg)
}

func Info(msg string, args ...any) {
	globalLogger().Info().Msgf(msg, args...)
}
