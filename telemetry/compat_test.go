package telemetry

import (
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func useGlobalTracerProvider(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		InitTracer("app")
	})
	return exporter
}

func TestInitTracerSetsCompatibilityScope(t *testing.T) {
	exporter := useGlobalTracerProvider(t)

	InitTracer("compat-scope")
	_, span := Job(t.Context(), "import")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	if got := spans[0].InstrumentationScope.Name; got != "compat-scope" {
		t.Fatalf("scope name = %q, want compat-scope", got)
	}
	if got := spans[0].Name; got != "job.import" {
		t.Fatalf("span name = %q, want job.import", got)
	}
}

// TestInitTracerConcurrentWithSpans must run under -race.
func TestInitTracerConcurrentWithSpans(t *testing.T) {
	useGlobalTracerProvider(t)
	ctx := t.Context()

	var wg sync.WaitGroup
	wg.Go(func() {
		for range 200 {
			InitTracer("svc")
		}
	})
	wg.Go(func() {
		for range 200 {
			_, span := Start(ctx, "op")
			span.End()
			var nilInstr *Instrumenter
			_, span = nilInstr.Job(ctx, "fallback")
			span.End()
		}
	})
	wg.Wait()
}
