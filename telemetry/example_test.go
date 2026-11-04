package telemetry_test

import (
	"context"
	"errors"
	"fmt"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/trypanic/go-sdk/telemetry"
)

// newExampleInstrumenter returns an Instrumenter backed by an in-memory
// exporter so the examples can print the spans they create.
func newExampleInstrumenter() (*telemetry.Instrumenter, *tracetest.InMemoryExporter) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	return telemetry.NewInstrumenter(telemetry.InstrumenterConfig{
		ScopeName:      "example-service",
		TracerProvider: provider,
	}), exporter
}

func ExampleNewInstrumenter() {
	instr, exporter := newExampleInstrumenter()

	ctx, job := instr.Job(context.Background(), "import")
	ctx, batch := instr.Batch(ctx, "process")
	_, call := instr.External(ctx, "catalog", "fetchProducts")
	call.End()
	batch.End()
	job.End()

	for _, span := range exporter.GetSpans() {
		fmt.Println(span.Name)
	}
	// Output:
	// external.catalog.fetchProducts
	// batch.process
	// job.import
}

func ExampleInstrumenter_Messaging() {
	instr, exporter := newExampleInstrumenter()

	_, span := instr.Messaging(context.Background(), "publish.orders",
		telemetry.WithSpanKind(trace.SpanKindProducer),
		telemetry.WithString("messaging.destination.name", "orders"),
	)
	span.End()

	got := exporter.GetSpans()[0]
	fmt.Println(got.Name, got.SpanKind)
	// Output: messaging.publish.orders producer
}

func ExampleInstrumenter_DB() {
	instr, exporter := newExampleInstrumenter()

	_, span := instr.DB(context.Background(), "postgres", "jobs", "insert")
	span.End()

	fmt.Println(exporter.GetSpans()[0].Name)
	// Output: postgres.jobs.insert
}

func ExampleRecordError() {
	instr, exporter := newExampleInstrumenter()

	_, span := instr.External(context.Background(), "payments", "charge")
	telemetry.SetAttrString(span, "payment.currency", "EUR")
	telemetry.RecordError(span, errors.New("card declined"), "charge failed")
	span.End()

	got := exporter.GetSpans()[0]
	fmt.Println(got.Name, got.Status.Code, got.Status.Description)
	// Output: external.payments.charge Error charge failed
}
