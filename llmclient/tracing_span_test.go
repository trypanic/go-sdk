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
	if _, ok := provider.(*tracingLLMProvider); !ok {
		t.Fatalf("NewWithInstrumenter must wrap with tracing, got %T", provider)
	}
	if _, err := provider.Execute(t.Context(), LLMRequestConfig{Model: "gpt-x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	spans := recorder.Ended()
	if len(spans) != 1 || spans[0].Name() != "llm.gpt-x" {
		t.Fatalf("spans = %v, want one span named llm.gpt-x", spans)
	}
	if stub.parent.SpanID() != spans[0].SpanContext().SpanID() {
		t.Fatal("the HTTP call must run inside the llm span")
	}

	if _, ok := NewWithInstrumenter(stub, validCfg, nil).(*Client); !ok {
		t.Fatal("a nil instrumenter must return the plain client")
	}
	for name, p := range map[string]LLMProvider{
		"New":             New(stub, validCfg),
		"WrapWithTracing": WrapWithTracing(NewClient(stub, validCfg)),
	} {
		if _, ok := p.(*tracingLLMProvider); !ok {
			t.Fatalf("%s must wrap with tracing, got %T", name, p)
		}
		if _, err := p.Execute(t.Context(), LLMRequestConfig{Model: "m"}); err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}
	}
}

func TestLLMSpanName(t *testing.T) {
	t.Parallel()

	if got := llmSpanName("m1"); got != "llm.m1" {
		t.Fatalf("llmSpanName(m1) = %q", got)
	}
	if got := llmSpanName(""); got != "llm.execute" {
		t.Fatalf("llmSpanName(\"\") = %q", got)
	}
}
