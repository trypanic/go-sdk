package otlp

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"
)

// Writer is a zerolog io.Writer that converts each JSON log line into an
// OTEL LogRecord and emits it via the global LoggerProvider (set up by
// NewProvider or InitProvider, which own the exporter connection).
//
// Trace correlation works automatically: trace_id and span_id written by
// WithTrace() / ErrorCtx() are promoted to the record's first-class trace
// fields, which observability backends use to link a log line to its trace.
// It is safe for concurrent use.
type Writer struct {
	logger log.Logger
}

// NewWriter creates an Writer for the given service name, used as
// the instrumentation scope. Use the same service name as the log provider
// and the trace provider so the backend groups logs and traces under one
// service.
//
// Must be called AFTER the log provider has been created (NewProvider or
// InitProvider): the writer binds to the global provider at construction.
//
// Usage:
//
//	lp := otlp.InitProvider(ctx, "my-service", "otel-collector:4317")
//	defer lp.Shutdown(ctx)
//	l, cleanup := logger.Init("my-service", version, otlp.NewWriter("my-service"))
func NewWriter(serviceName string) *Writer {
	return &Writer{
		logger: otel.GetLoggerProvider().Logger(serviceName),
	}
}

// Write implements io.Writer. Each call receives exactly one complete JSON
// log line from zerolog (zerolog guarantees one Write per event).
func (w *Writer) Write(p []byte) (int, error) {
	// Parse the zerolog JSON so we can extract fields into the OTEL record.
	// UseNumber keeps integers above 2^53 exact (see anyToLogValue).
	var fields map[string]any
	decoder := json.NewDecoder(bytes.NewReader(p))
	decoder.UseNumber()
	if err := decoder.Decode(&fields); err != nil {
		// Malformed line — emit as plain text so nothing is silently lost.
		var r log.Record
		r.SetTimestamp(time.Now())
		r.SetBody(attribute.StringValue(string(p)))
		r.SetSeverity(log.SeverityError)
		w.logger.Emit(context.Background(), r)
		return len(p), nil
	}

	var r log.Record

	// ── Timestamp ────────────────────────────────────────────────────────────
	// SDK loggers write RFC 3339 with nanoseconds into the "time" field.
	r.SetTimestamp(parseLogTime(fields["time"]))

	// ── Severity ─────────────────────────────────────────────────────────────
	if lvl, ok := fields["level"].(string); ok {
		r.SetSeverity(zerologLevelToOTEL(lvl))
		r.SetSeverityText(lvl)
	}

	// ── Body: the human-readable message ─────────────────────────────────────
	if msg, ok := fields["message"].(string); ok {
		r.SetBody(attribute.StringValue(msg))
	}

	// ── Trace correlation ─────────────────────────────────────────────────────
	// log.Record has no SetTraceID / SetSpanID methods. The correct approach
	// is to pass a context.Context that carries the span to Emit — the SDK
	// then extracts TraceID + SpanID from that context automatically.
	//
	// We reconstruct a minimal SpanContext from the IDs written by
	// WithTrace() / ErrorCtx() / WarnCtx() in otel.go.
	emitCtx := context.Background()
	traceIDStr, hasTrace := fields["trace_id"].(string)
	spanIDStr, hasSpan := fields["span_id"].(string)
	if hasTrace && hasSpan {
		tid, tErr := trace.TraceIDFromHex(traceIDStr)
		sid, sErr := trace.SpanIDFromHex(spanIDStr)
		if tErr == nil && sErr == nil {
			sc := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    tid,
				SpanID:     sid,
				TraceFlags: trace.FlagsSampled,
				Remote:     true,
			})
			// ContextWithSpanContext injects the span into ctx; Emit reads it.
			emitCtx = trace.ContextWithSpanContext(emitCtx, sc)
		}
	}

	// ── Attributes: all remaining fields ─────────────────────────────────────
	// Emit every other zerolog field as a log attribute so the backend shows
	// them with the record (error_code, reason, stack_trace, etc.)
	attrs := make([]attribute.KeyValue, 0, len(fields))
	for k, v := range fields {
		if isRecordField(k) {
			continue
		}
		attrs = append(attrs, attribute.KeyValue{
			Key:   attribute.Key(k),
			Value: anyToLogValue(v),
		})
	}
	r.AddAttributes(attrs...)

	w.logger.Emit(emitCtx, r)
	return len(p), nil
}

// isRecordField reports whether key maps to a dedicated OTEL record field
// (or is dropped) instead of becoming an attribute.
func isRecordField(key string) bool {
	switch key {
	case "level", "time", "message", "trace_id", "span_id", "caller":
		return true
	default:
		return false
	}
}

// parseLogTime converts the "time" field to the record timestamp: an RFC 3339
// string (what SDK loggers write) or a number of Unix epoch seconds (the
// format of older SDK versions). Anything else yields the current time.
func parseLogTime(v any) time.Time {
	switch t := v.(type) {
	case string:
		if ts, err := time.Parse(time.RFC3339Nano, t); err == nil {
			return ts
		}
	case json.Number:
		if sec, err := t.Int64(); err == nil {
			return time.Unix(sec, 0)
		}
	}
	return time.Now()
}

// zerologLevelToOTEL maps zerolog level strings to OTEL log severity numbers.
func zerologLevelToOTEL(level string) log.Severity {
	switch level {
	case "trace":
		return log.SeverityTrace
	case "debug":
		return log.SeverityDebug
	case "info":
		return log.SeverityInfo
	case "warn":
		return log.SeverityWarn
	case "error":
		return log.SeverityError
	case "fatal", "panic":
		return log.SeverityFatal
	default:
		return log.SeverityUndefined
	}
}

// anyToLogValue converts a JSON-unmarshalled value to an OTEL attribute.Value.
func anyToLogValue(v any) attribute.Value {
	switch val := v.(type) {
	case string:
		return attribute.StringValue(val)
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return attribute.Int64Value(i)
		}
		if f, err := val.Float64(); err == nil {
			return attribute.Float64Value(f)
		}
		return attribute.StringValue(val.String())
	case float64:
		return attribute.Float64Value(val)
	case bool:
		return attribute.BoolValue(val)
	case map[string]any:
		b, _ := json.Marshal(val)
		return attribute.StringValue(string(b))
	case []any:
		b, _ := json.Marshal(val)
		return attribute.StringValue(string(b))
	case nil:
		return attribute.StringValue("null")
	default:
		b, _ := json.Marshal(val)
		return attribute.StringValue(string(b))
	}
}
