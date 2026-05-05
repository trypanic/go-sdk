package logger

import (
	"context"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"

	"github.com/trypanic/go-sdk/errorkit"
)

// WithTrace enriches a zerolog.Event with trace_id and span_id from
// the active OpenTelemetry span in the context.
//
// Usage:
//
//	logger.WithTrace(ctx, logger.Info()).Msg("message")
func WithTrace(ctx context.Context, event *zerolog.Event) *zerolog.Event {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if sc.HasTraceID() {
		event = event.Str("trace_id", sc.TraceID().String())
	}
	if sc.HasSpanID() {
		event = event.Str("span_id", sc.SpanID().String())
	}
	return event
}

// LogInfoWithTrace logs an Info message enriched with OTel trace context.
//
// Usage:
//
//	logger.LogInfoWithTrace(ctx, "Order received")
func LogInfoWithTrace(ctx context.Context, msg string) {
	WithTrace(ctx, CtxOrGlobal(ctx).Info()).Msg(msg)
}

// LogErrorWithTrace logs an ErrorKit error at Error level with trace context.
//
// Trace fields come exclusively from the live OTel span in ctx — the TraceID
// stored inside appErr (if any) is intentionally ignored here to avoid
// overwriting the authoritative span context injected by WithTrace.
//
// Usage:
//
//	logger.LogErrorWithTrace(ctx, appErr, "Payment failed")
func LogErrorWithTrace(ctx context.Context, appErr *errorkit.AppError, msg string) {
	// FIX: Previously called WithErrorKit after WithTrace, which let
	// appErr.TraceID overwrite the trace_id field already set by WithTrace.
	// WithErrorKit no longer emits trace_id; the field is owned by WithTrace.
	event := WithTrace(ctx, CtxOrGlobal(ctx).Error())
	WithErrorKit(event, appErr).Msg(msg)
}

// LogWarnWithTrace logs an ErrorKit error at Warn level with trace context.
//
// Usage:
//
//	logger.LogWarnWithTrace(ctx, appErr, "DB timeout")
func LogWarnWithTrace(ctx context.Context, appErr *errorkit.AppError, msg string) {
	event := WithTrace(ctx, CtxOrGlobal(ctx).Warn())
	WithErrorKit(event, appErr).Msg(msg)
}

// TraceIDFromCtx returns the W3C trace-id string from the active OTel span,
// or an empty string if no span is present in ctx.
//
// Use this when you need to attach the trace ID to an AppError that will
// be passed outside a context-carrying call chain (e.g. stored in a queue
// message, returned in an HTTP response body).
//
// Usage:
//
//	appErr := errorkit.NewError(errorkit.ERR_DB_MONGO_ERROR).
//	    With(errorkit.WithTraceID(logger.TraceIDFromCtx(ctx)))
func TraceIDFromCtx(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if sc.HasTraceID() {
		return sc.TraceID().String()
	}
	return ""
}
