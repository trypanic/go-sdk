package logger

import (
	"context"
	"encoding/json"
	"time"

	"go.opentelemetry.io/otel/log"
	otelglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/trace"
)

// OTLPWriter is a zerolog io.Writer that converts each JSON log line into an
// OTEL LogRecord and emits it via the global LoggerProvider.
//
// It ships logs over the same gRPC connection already used for traces,
// so no extra network config is required.
//
// Trace correlation works automatically: trace_id and span_id written by
// WithTrace() / ErrorCtx() are promoted to OTEL first-class fields, which
// is what SigNoz uses to link a log line to its trace waterfall.
type OTLPWriter struct {
	logger log.Logger
}

// NewOTLPWriter creates an OTLPWriter for the given service name.
// The service name should match provider.WithServiceName() in main.go
// so SigNoz groups logs and traces under the same service.
//
// Must be called AFTER the OTEL LoggerProvider has been initialised
// (i.e. after initLogProvider in main.go).
//
// Usage:
//
//	lp := logger.InitLogProvider(ctx, "my-service", "otel-collector:4317")
//	defer lp.Shutdown(ctx)
//	l, cleanup := logger.Init("my-service", version, logger.NewOTLPWriter("my-service"))
func NewOTLPWriter(serviceName string) *OTLPWriter {
	return &OTLPWriter{
		logger: otelglobal.GetLoggerProvider().Logger(serviceName),
	}
}

// Write implements io.Writer. Each call receives exactly one complete JSON
// log line from zerolog (zerolog guarantees one Write per event).
func (w *OTLPWriter) Write(p []byte) (int, error) {
	// Parse the zerolog JSON so we can extract fields into the OTEL record.
	var fields map[string]any
	if err := json.Unmarshal(p, &fields); err != nil {
		// Malformed line — emit as plain text so nothing is silently lost.
		var r log.Record
		r.SetTimestamp(time.Now())
		r.SetBody(log.StringValue(string(p)))
		r.SetSeverity(log.SeverityError)
		w.logger.Emit(context.Background(), r)
		return len(p), nil
	}

	var r log.Record

	// ── Timestamp ────────────────────────────────────────────────────────────
	// zerolog writes Unix epoch seconds (TimeFormatUnix) into the "time" field.
	if ts, ok := fields["time"].(float64); ok {
		r.SetTimestamp(time.Unix(int64(ts), 0))
	} else {
		r.SetTimestamp(time.Now())
	}

	// ── Severity ─────────────────────────────────────────────────────────────
	if lvl, ok := fields["level"].(string); ok {
		r.SetSeverity(zerologLevelToOTEL(lvl))
		r.SetSeverityText(lvl)
	}

	// ── Body: the human-readable message ─────────────────────────────────────
	if msg, ok := fields["message"].(string); ok {
		r.SetBody(log.StringValue(msg))
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
	// Emit every other zerolog field as a log attribute so SigNoz surfaces
	// them in the log detail panel (error_code, reason, stack_trace, etc.)
	skip := map[string]bool{
		"level": true, "time": true, "message": true,
		"trace_id": true, "span_id": true, "caller": true,
	}
	attrs := make([]log.KeyValue, 0, len(fields))
	for k, v := range fields {
		if skip[k] {
			continue
		}
		attrs = append(attrs, log.KeyValue{
			Key:   k,
			Value: anyToLogValue(v),
		})
	}
	r.AddAttributes(attrs...)

	w.logger.Emit(emitCtx, r)
	return len(p), nil
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

// anyToLogValue converts a JSON-unmarshalled value to an OTEL log.Value.
func anyToLogValue(v any) log.Value {
	switch val := v.(type) {
	case string:
		return log.StringValue(val)
	case float64:
		return log.Float64Value(val)
	case bool:
		return log.BoolValue(val)
	case map[string]any:
		b, _ := json.Marshal(val)
		return log.StringValue(string(b))
	case []any:
		b, _ := json.Marshal(val)
		return log.StringValue(string(b))
	case nil:
		return log.StringValue("null")
	default:
		b, _ := json.Marshal(val)
		return log.StringValue(string(b))
	}
}
