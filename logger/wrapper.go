package logger

import (
	"context"
	"os"

	"github.com/trypanic/go-sdk/errorkit"
)

// Error logs an ErrorKit error at Error level using the global logger.
//
// The first *errorkit.AppError found in appErr's chain (errors.As semantics)
// is logged with its own code; any other error is wrapped once in
// ERR_INTERNAL. A nil error is logged as ERR_UNKNOWN.
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

	withError(globalLogger().Error(), appErr).Msg(errorMsg)
	return appErr
}

// LogAndReturn logs appErr like Error and returns it unchanged.
func LogAndReturn(appErr error, msg ...string) error {
	return Error(appErr, msg...)
}

// Panic logs appErr like Error and exits the process with status 1. Deferred
// functions do not run and buffered OTLP records are not flushed: close the
// LogProviderHandle first if they matter.
func Panic(appErr error, msg ...string) {
	_ = Error(appErr, msg...)
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
// Every line carries the trace and span IDs, so the tracing backend can link
// it to the request's trace.
//
// Usage:
//
//	logger.ErrorCtx(ctx, appErr, "Order processing failed")
func ErrorCtx(ctx context.Context, appErr *errorkit.AppError, msg string) {
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
	event := WithTrace(ctx, CtxOrGlobal(ctx).Warn())
	WithErrorKit(event, appErr).Msg(msg)
}

// Info logs a printf-style Info message using the global logger.
func Info(msg string, args ...any) {
	globalLogger().Info().Msgf(msg, args...)
}
