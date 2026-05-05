package llmclient

import (
	"context"
	"testing"

	"github.com/trypanic/go-sdk/telemetry"
)

func TestWrapWithInstrumenterNilReturnsInner(t *testing.T) {
	t.Parallel()

	stub := &stubProvider{}
	if got := WrapWithInstrumenter(stub, nil); got != stub {
		t.Fatalf("WrapWithInstrumenter(nil) must return inner unchanged")
	}
}

func TestWrapWithInstrumenterAddsSpanWrapper(t *testing.T) {
	t.Parallel()

	wrapped := WrapWithInstrumenter(&stubProvider{}, telemetry.NewInstrumenter(telemetry.InstrumenterConfig{}))
	if _, ok := wrapped.(*tracingLLMProvider); !ok {
		t.Fatalf("expected tracingLLMProvider, got %T", wrapped)
	}
}

func TestNewWithoutTracingReturnsPlainClient(t *testing.T) {
	t.Parallel()

	provider := NewWithoutTracing(nil, Config{})
	if _, ok := provider.(*tracingLLMProvider); ok {
		t.Fatalf("NewWithoutTracing must not wrap with tracing")
	}
}

type stubProvider struct{}

func (s *stubProvider) Execute(context.Context, LLMRequestConfig) (string, error) { return "", nil }
