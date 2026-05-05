package llmclient

import (
	"context"

	"github.com/trypanic/go-sdk/telemetry"
)

// WrapWithTracing wraps an LLMProvider with automatic span creation.
//
// Each call to Execute opens a span named after the model:
//
//	"llm.<model>"
//
// The span becomes the parent of the httprequest span produced by the
// underlying HTTP call, making LLM calls immediately identifiable in
// the trace tree without any instrumentation in call sites.
//
// Usage: automatically applied by New().
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

func (t *tracingLLMProvider) Execute(ctx context.Context, reqConfig LLMRequestConfig) (string, error) {
	ctx, span := t.instrumenter.Start(ctx, llmSpanName(reqConfig.Model))
	defer span.End()
	return t.inner.Execute(ctx, reqConfig)
}

func llmSpanName(model string) string {
	if model != "" {
		return "llm." + model
	}
	return "llm.execute"
}
