package logger

import (
	"github.com/trypanic/go-sdk/errorkit"

	"github.com/rs/zerolog"
)

// WithErrorKit enriches a zerolog.Event with all ErrorKit error fields.
//
// IMPORTANT: This does NOT emit trace_id / span_id. Those fields are owned
// by WithTrace (otel.go) to avoid field collisions when both are used together.
// If you need trace correlation, always call WithTrace first, then WithErrorKit.
//
// IMPORTANT: This does NOT add a message. Call .Msg() separately.
//
// Usage:
//
//	event := logger.WithTrace(ctx, log.Error())
//	logger.WithErrorKit(event, appErr).Msg("Database connection failed")
func WithErrorKit(event *zerolog.Event, appErr *errorkit.AppError) *zerolog.Event {
	// Always-present fields
	event = event.
		Str("error_code", string(appErr.Code())).
		Str("error_id", appErr.ID).
		Str("error_timestamp", appErr.Timestamp).
		Str("error_type", string(appErr.Metadata.Type)).
		Str("error_group", string(appErr.Metadata.Group)).
		Str("error_category", appErr.Metadata.Category).
		Int("http_status", appErr.Metadata.HTTPStatus).
		Bool("retriable", appErr.Metadata.Retriable)

	// FIX: trace_id was previously emitted here from appErr.TraceID, which
	// caused a field collision when WithTrace had already written trace_id from
	// the live OTel span context. The live span always wins, so this field is
	// no longer written here. Use logger.TraceIDFromCtx(ctx) if you need to
	// store the trace ID inside the AppError itself (e.g. for queue messages).

	if appErr.Reason != "" {
		event = event.Str("reason", appErr.Reason)
	}

	if appErr.Payload != nil {
		event = event.Interface("payload", appErr.Payload)
	}

	// Wrapped error - the ORIGINAL error, not the ErrorKit wrapper
	if appErr.Wrapped != nil {
		event = event.AnErr("wrapped_error", appErr.Wrapped)
	}

	// Stack trace as structured frames
	if len(appErr.Trace) > 0 {
		frames := make([]map[string]any, len(appErr.Trace))
		for i, t := range appErr.Trace {
			frames[i] = map[string]any{
				"file":     t.File,
				"line":     t.Line,
				"package":  t.Package,
				"function": t.Function,
			}
		}
		event = event.Interface("stack_trace", frames)
	}

	return event
}
