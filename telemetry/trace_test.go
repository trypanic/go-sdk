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

	ctx, span := instrumenter.Start(context.Background(), "operation.name")
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

func TestInstrumenterNamingHelpers(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	instrumenter := NewInstrumenter(InstrumenterConfig{
		ScopeName:      "test-scope",
		TracerProvider: provider,
	})

	tests := []struct {
		name string
		run  func()
		want string
	}{
		{
			name: "job",
			run: func() {
				ctx, span := instrumenter.Job(context.Background(), "import")
				defer span.End()
				_ = ctx
			},
			want: "job.import",
		},
		{
			name: "batch",
			run: func() {
				ctx, span := instrumenter.Batch(context.Background(), "process")
				defer span.End()
				_ = ctx
			},
			want: "batch.process",
		},
		{
			name: "external",
			run: func() {
				ctx, span := instrumenter.External(context.Background(), "amazon", "fetch")
				defer span.End()
				_ = ctx
			},
			want: "external.amazon.fetch",
		},
		{
			name: "messaging",
			run: func() {
				ctx, span := instrumenter.Messaging(context.Background(), "publish.queue")
				defer span.End()
				_ = ctx
			},
			want: "messaging.publish.queue",
		},
		{
			name: "db",
			run: func() {
				ctx, span := instrumenter.DB(context.Background(), "postgres", "jobs", "insert")
				defer span.End()
				_ = ctx
			},
			want: "postgres.jobs.insert",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter.Reset()
			tt.run()

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
