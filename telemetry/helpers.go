package telemetry

import (
	"errors"

	"github.com/trypanic/go-sdk/v2/errorkit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// --- Span start options (used at span creation time) ---

// WithString returns a span start option that sets a string attribute.
func WithString(key, value string) trace.SpanStartOption {
	return trace.WithAttributes(attribute.String(key, value))
}

// WithInt returns a span start option that sets an int attribute.
func WithInt(key string, value int) trace.SpanStartOption {
	return trace.WithAttributes(attribute.Int(key, value))
}

// WithBool returns a span start option that sets a bool attribute.
func WithBool(key string, value bool) trace.SpanStartOption {
	return trace.WithAttributes(attribute.Bool(key, value))
}

// WithSpanKind returns a span start option that sets the span kind, for
// example trace.SpanKindProducer for a publish or trace.SpanKindClient for
// an outbound call.
func WithSpanKind(kind trace.SpanKind) trace.SpanStartOption {
	return trace.WithSpanKind(kind)
}

// --- Post-creation span helpers (used after span is started) ---

// RecordError marks the span as failed with both RecordError and SetStatus.
// Always call this instead of setting them separately to ensure consistency.
// A nil err is a no-op: the span status is left unchanged.
func RecordError(span Span, err error, msg string) {
	if err == nil {
		return
	}
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

// End records a failure and ends span. Error events contain the registered error
// description, never wrapped diagnostic text, payloads or stack traces.
// Classified failures also carry error.code and error.retriable attributes.
func End(span Span, err error) {
	if span == nil {
		return
	}
	defer span.End()
	if err == nil {
		return
	}
	message := "operation failed"
	if appErr, ok := errorkit.As(err); ok {
		if appErr.Metadata.Description != "" {
			message = appErr.Metadata.Description
		}
		span.SetAttributes(attribute.String("error.code", string(appErr.Code())), attribute.Bool("error.retriable", appErr.Metadata.Retriable))
	}
	span.RecordError(errors.New(message))
	span.SetStatus(codes.Error, message)
}
