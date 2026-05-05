package database

import (
	"context"
	"testing"

	"github.com/trypanic/go-sdk/telemetry"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestWrapWithInstrumenterUsesExplicitInstrumenter(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	instrumenter := telemetry.NewInstrumenter(telemetry.InstrumenterConfig{
		ScopeName:      "database-test",
		TracerProvider: provider,
	})

	storedProcedure := WrapWithInstrumenter[int](&fakeStoredProcedurer[int]{result: 1}, instrumenter)

	result, err := storedProcedure.QueryRow(context.Background(), "SELECT * FROM amz_test_proc($1)", 1)
	if err != nil {
		t.Fatalf("QueryRow returned error: %v", err)
	}
	if result != 1 {
		t.Fatalf("result = %d, want 1", result)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	if spans[0].Name != "postgres.amz_test_proc" {
		t.Fatalf("span name = %q, want postgres.amz_test_proc", spans[0].Name)
	}
	if spans[0].InstrumentationScope.Name != "database-test" {
		t.Fatalf("scope name = %q, want database-test", spans[0].InstrumentationScope.Name)
	}
}

type fakeStoredProcedurer[T any] struct {
	result T
}

func (f *fakeStoredProcedurer[T]) QueryRow(context.Context, string, ...any) (T, error) {
	return f.result, nil
}

func (f *fakeStoredProcedurer[T]) Query(context.Context, string, ...any) ([]T, error) {
	return []T{f.result}, nil
}

func (f *fakeStoredProcedurer[T]) QueryRowJSON(context.Context, string, any, ...any) (T, error) {
	return f.result, nil
}

func (f *fakeStoredProcedurer[T]) QueryJSON(context.Context, string, any, ...any) ([]T, error) {
	return []T{f.result}, nil
}
