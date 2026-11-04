package telemetry

import (
	"errors"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRecordError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus codes.Code
		wantEvents int
	}{
		{name: "nil error leaves the span unset", err: nil, wantStatus: codes.Unset, wantEvents: 0},
		{name: "error marks the span failed", err: errors.New("boom"), wantStatus: codes.Error, wantEvents: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exporter := tracetest.NewInMemoryExporter()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			_, span := provider.Tracer("test").Start(t.Context(), "op")
			RecordError(span, tc.err, "failed")
			span.End()

			spans := exporter.GetSpans()
			if len(spans) != 1 {
				t.Fatalf("exported spans = %d, want 1", len(spans))
			}
			if got := spans[0].Status.Code; got != tc.wantStatus {
				t.Fatalf("status = %v, want %v", got, tc.wantStatus)
			}
			if got := len(spans[0].Events); got != tc.wantEvents {
				t.Fatalf("events = %d, want %d", got, tc.wantEvents)
			}
		})
	}
}
