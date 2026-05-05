package telemetry

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// --- Span start options (used at span creation time) ---

func WithString(key, value string) trace.SpanStartOption {
	return trace.WithAttributes(attribute.String(key, value))
}

func WithInt(key string, value int) trace.SpanStartOption {
	return trace.WithAttributes(attribute.Int(key, value))
}

func WithBool(key string, value bool) trace.SpanStartOption {
	return trace.WithAttributes(attribute.Bool(key, value))
}

func WithSpanKind(kind trace.SpanKind) trace.SpanStartOption {
	return trace.WithSpanKind(kind)
}

// --- Post-creation span helpers (used after span is started) ---

// RecordError marks the span as failed with both RecordError and SetStatus.
// Always call this instead of setting them separately to ensure consistency.
func RecordError(span Span, err error, msg string) {
	span.RecordError(err)
	span.SetStatus(codes.Error, msg)
}

// SetAttrString sets a string attribute on an existing span.
func SetAttrString(span Span, key, value string) {
	span.SetAttributes(attribute.String(key, value))
}

// SetAttrInt sets an int attribute on an existing span.
func SetAttrInt(span Span, key string, value int) {
	span.SetAttributes(attribute.Int(key, value))
}

// SetAttrInt64 sets an int64 attribute on an existing span.
func SetAttrInt64(span Span, key string, value int64) {
	span.SetAttributes(attribute.Int64(key, value))
}

// SetAttrStringSlice sets a string slice attribute on an existing span.
func SetAttrStringSlice(span Span, key string, values []string) {
	span.SetAttributes(attribute.StringSlice(key, values))
}
