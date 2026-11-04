package llmclient

import (
	"context"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/trypanic/go-sdk/httprequest"
	"github.com/trypanic/go-sdk/telemetry"
)

// spanCapturingRequester records the span active when Do is called.
type spanCapturingRequester struct {
	captureRequester
	parent trace.SpanContext
}

func (s *spanCapturingRequester) Do(ctx context.Context, cfg httprequest.RequestConfig, out any) error {
	s.parent = trace.SpanContextFromContext(ctx)
	return s.captureRequester.Do(ctx, cfg, out)
}

var validCfg = Config{APIKey: "k", Endpoint: "https://llm.example/v1/chat"}

func TestTracingConstructorsOpenModelSpans(t *testing.T) {
	t.Parallel()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	instr := telemetry.NewInstrumenter(telemetry.InstrumenterConfig{TracerProvider: tp})

	stub := &spanCapturingRequester{}
	provider := NewWithInstrumenter(stub, validCfg, instr)
	if provider.instrumenter == nil {
		t.Fatalf("NewWithInstrumenter must wrap with tracing, got %T", provider)
	}
	if _, err := provider.Execute(t.Context(), RequestConfig{Model: "gpt-x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	spans := recorder.Ended()
	if len(spans) != 1 || spans[0].Name() != "external.llm.execute" {
		t.Fatalf("spans = %v, want one span named external.llm.execute", spans)
	}
	if stub.parent.SpanID() != spans[0].SpanContext().SpanID() {
		t.Fatal("the HTTP call must run inside the llm span")
	}

	if NewWithInstrumenter(stub, validCfg, nil).instrumenter != nil {
		t.Fatal("a nil instrumenter must return the plain client")
	}
	for name, p := range map[string]Provider{
		"New":             New(stub, validCfg),
		"WrapWithTracing": WrapWithTracing(NewClient(stub, validCfg)),
	} {
		switch client := p.(type) {
		case *Client:
			if client.instrumenter == nil {
				t.Fatal("constructor must enable tracing")
			}
		case *tracingLLMProvider:
			if client.instrumenter == nil {
				t.Fatal("wrapper must enable tracing")
			}
		default:
			t.Fatalf("%s: unexpected provider %T", name, p)
		}
		if _, err := p.Execute(t.Context(), RequestConfig{Model: "m"}); err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}
	}
}

func TestLLMSpanNamesStayBounded(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	instr := telemetry.NewInstrumenter(telemetry.InstrumenterConfig{TracerProvider: provider})
	for _, model := range []string{"m1", "m2", ""} {
		_, span := llmSpan(instr, t.Context(), model)
		span.End()
	}
	for _, span := range recorder.Ended() {
		if span.Name() != "external.llm.execute" {
			t.Fatalf("unbounded model span %s", span.Name())
		}
	}
}
