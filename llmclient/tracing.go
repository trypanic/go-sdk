package llmclient

import (
	"context"

	"go.opentelemetry.io/otel/trace"

	"github.com/trypanic/go-sdk/telemetry"
)

// WrapWithTracing adds external.llm.execute client spans. The model is an
// attribute, so selecting a different model does not create a new span name.
func WrapWithTracing(inner LLMProvider) LLMProvider {
	return WrapWithInstrumenter(inner, telemetry.NewInstrumenter(telemetry.InstrumenterConfig{}))
}

// WrapWithInstrumenter wraps an LLMProvider with an explicit telemetry instrumenter.
// Pass nil to skip tracing (returns inner unchanged).
func WrapWithInstrumenter(inner LLMProvider, instrumenter *telemetry.Instrumenter) LLMProvider {
	if instrumenter == nil {
		return inner
	}
	return &tracingLLMProvider{inner: inner, instrumenter: instrumenter}
}

type tracingLLMProvider struct {
	inner        LLMProvider
	instrumenter *telemetry.Instrumenter
}

func (t *tracingLLMProvider) Execute(ctx context.Context, reqConfig LLMRequestConfig) (result string, err error) {
	ctx, span := llmSpan(t.instrumenter, ctx, reqConfig.Model)
	defer func() { telemetry.End(span, err) }()
	return t.inner.Execute(ctx, reqConfig)
}

func llmSpan(i *telemetry.Instrumenter, ctx context.Context, model string) (context.Context, telemetry.Span) {
	return i.External(ctx, "llm", "execute", telemetry.WithSpanKind(trace.SpanKindClient), telemetry.WithString("gen_ai.request.model", model))
}

// Unwrap returns the provider enclosed by this decorator.
func (t *tracingLLMProvider) Unwrap() LLMProvider { return t.inner }
