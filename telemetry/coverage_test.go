package telemetry

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestCompatibilityHelpersUseInitTracerScope(t *testing.T) {
	exporter := useGlobalTracerProvider(t)
	InitTracer("compat-helpers")
	ctx := t.Context()

	tests := []struct {
		name string
		span Span
		want string
	}{
		{name: "start", span: spanOf(Start(ctx, "custom.op")), want: "custom.op"},
		{name: "job", span: spanOf(Job(ctx, "import")), want: "job.import"},
		{name: "batch", span: spanOf(Batch(ctx, "process")), want: "batch.process"},
		{name: "external", span: spanOf(External(ctx, "warehouse", "update")), want: "external.warehouse.update"},
		{name: "messaging", span: spanOf(Messaging(ctx, "publish.orders")), want: "messaging.publish.orders"},
		{name: "db", span: spanOf(DB(ctx, "mongo", "listings", "insert")), want: "mongo.listings.insert"},
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
				t.Errorf("span name = %q, want %q", spans[0].Name, tt.want)
			}
			if got := spans[0].InstrumentationScope.Name; got != "compat-helpers" {
				t.Errorf("scope = %q, want compat-helpers", got)
			}
		})
	}
}

func TestNilInstrumenterFallsBackToCompatibility(t *testing.T) {
	exporter := useGlobalTracerProvider(t)
	InitTracer("fallback-scope")

	var instr *Instrumenter
	_, span := instr.DB(t.Context(), "postgres", "jobs", "select")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].InstrumentationScope.Name != "fallback-scope" {
		t.Fatalf("spans = %+v, want one span from fallback-scope", spans)
	}
}

func TestNewInstrumenterDefaultsScopeAndGlobalProvider(t *testing.T) {
	exporter := useGlobalTracerProvider(t)

	_, span := NewInstrumenter(InstrumenterConfig{}).Start(t.Context(), "op")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].InstrumentationScope.Name != "app" {
		t.Fatalf("spans = %+v, want one span from the global provider with scope app", spans)
	}
}

func spanAttributes(s tracetest.SpanStub) map[attribute.Key]attribute.Value {
	attrs := make(map[attribute.Key]attribute.Value, len(s.Attributes))
	for _, kv := range s.Attributes {
		attrs[kv.Key] = kv.Value
	}
	return attrs
}

func TestSpanStartOptionsAndAttributeSetters(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	instr := NewInstrumenter(InstrumenterConfig{TracerProvider: provider})

	ctx, span := instr.Messaging(t.Context(), "publish.orders",
		WithString("s", "v"),
		WithInt("i", 7),
		WithBool("b", true),
		WithSpanKind(trace.SpanKindProducer),
	)
	current := SpanFromContext(ctx)
	if !current.SpanContext().Equal(span.SpanContext()) {
		t.Fatal("SpanFromContext did not return the span stored in ctx")
	}
	SetAttrString(current, "set.s", "x")
	SetAttrInt(current, "set.i", 3)
	SetAttrInt64(current, "set.i64", 1<<40)
	SetAttrStringSlice(current, "set.ss", []string{"a", "b"})
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	if spans[0].SpanKind != trace.SpanKindProducer {
		t.Errorf("span kind = %v, want producer", spans[0].SpanKind)
	}
	attrs := spanAttributes(spans[0])
	checks := []struct {
		key  attribute.Key
		want attribute.Value
	}{
		{key: "s", want: attribute.StringValue("v")},
		{key: "i", want: attribute.IntValue(7)},
		{key: "b", want: attribute.BoolValue(true)},
		{key: "set.s", want: attribute.StringValue("x")},
		{key: "set.i", want: attribute.IntValue(3)},
		{key: "set.i64", want: attribute.Int64Value(1 << 40)},
		{key: "set.ss", want: attribute.StringSliceValue([]string{"a", "b"})},
	}
	for _, c := range checks {
		if got, ok := attrs[c.key]; !ok || got != c.want {
			t.Errorf("attribute %s = %v (present %v), want %v", c.key, got.String(), ok, c.want.String())
		}
	}
}

func TestSpanFromContextWithoutSpanIsNoop(t *testing.T) {
	span := SpanFromContext(t.Context())
	if span.SpanContext().IsValid() {
		t.Fatal("SpanFromContext(empty ctx) returned a valid span context")
	}
	// Setters on the no-op span must not panic.
	SetAttrString(span, "k", "v")
	RecordError(span, nil, "noop")
}
