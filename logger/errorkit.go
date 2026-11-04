package logger

import (
	"github.com/rs/zerolog"

	"github.com/trypanic/go-sdk/v2/errorkit"
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
//
// A nil appErr does not panic: the event gets error_code=ERR_UNKNOWN and no
// other error fields.
func WithErrorKit(event *zerolog.Event, appErr *errorkit.AppError) *zerolog.Event {
	if appErr == nil {
		return event.Str("error_code", string(errorkit.ERR_UNKNOWN))
	}

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

	// trace_id is deliberately not written from appErr.TraceID: WithTrace
	// owns it and takes it from the live OTel span, so the two never
	// collide. Use logger.TraceIDFromCtx(ctx) to store the trace ID inside
	// the AppError itself (e.g. for queue messages).

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

// withError enriches event with the errorkit fields of err.
//
// When err's chain contains an *errorkit.AppError (also below fmt.Errorf
// %w wrappers), that error's code and metadata are logged; if it is not err
// itself, err's full message is attached as the "error" field for context.
// A plain error is wrapped once in ERR_INTERNAL; a nil error is logged as
// ERR_UNKNOWN.
func withError(event *zerolog.Event, err error) *zerolog.Event {
	if err == nil {
		return WithErrorKit(event, nil)
	}
	if appErr, ok := err.(*errorkit.AppError); ok && appErr == nil {
		return WithErrorKit(event, nil)
	}
	if appErr, ok := errorkit.As(err); ok {
		if error(appErr) != err {
			event = event.Err(err)
		}
		return WithErrorKit(event, appErr)
	}
	return WithErrorKit(event, errorkit.NewError(errorkit.ERR_INTERNAL).With(errorkit.WithWrapped(err)))
}
