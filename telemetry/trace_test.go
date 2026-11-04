package telemetry

import (
	"context"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestInstrumenterUsesExplicitTracerProvider(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	instrumenter := NewInstrumenter(InstrumenterConfig{
		ScopeName:      "test-scope",
		TracerProvider: provider,
	})

	ctx, span := instrumenter.Start(t.Context(), "operation.name")
	span.End()

	if ctx == nil {
		t.Fatal("Start returned nil context")
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	if spans[0].InstrumentationScope.Name != "test-scope" {
		t.Fatalf("scope name = %q, want test-scope", spans[0].InstrumentationScope.Name)
	}
	if spans[0].Name != "operation.name" {
		t.Fatalf("span name = %q, want operation.name", spans[0].Name)
	}
}

// spanOf discards the context returned by a span helper so a helper call
// can be used as a table value.
func spanOf(_ context.Context, span Span) Span { return span }

func TestInstrumenterNamingHelpers(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	instrumenter := NewInstrumenter(InstrumenterConfig{
		ScopeName:      "test-scope",
		TracerProvider: provider,
	})
	ctx := t.Context()

	tests := []struct {
		name string
		span Span
		want string
	}{
		{name: "job", span: spanOf(instrumenter.Job(ctx, "import")), want: "job.import"},
		{name: "batch", span: spanOf(instrumenter.Batch(ctx, "process")), want: "batch.process"},
		{name: "external", span: spanOf(instrumenter.External(ctx, "amazon", "fetch")), want: "external.amazon.fetch"},
		{name: "messaging", span: spanOf(instrumenter.Messaging(ctx, "publish.queue")), want: "messaging.publish.queue"},
		{name: "db", span: spanOf(instrumenter.DB(ctx, "postgres", "jobs", "insert")), want: "postgres.jobs.insert"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter.Reset()
			tt.span.End()

			spans := exporter.GetSpans()
			if len(spans) != 1 {
				t.Fatalf("exported spans = %d, want 1", len(spans))
			}
			if spans[0].Name != tt.want {
				t.Fatalf("span name = %q, want %q", spans[0].Name, tt.want)
			}
		})
	}
}
