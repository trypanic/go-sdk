package telemetry_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/v2/telemetry"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestInstrumenterExposesProvider(t *testing.T) {
	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(t.Context())) })
	instr := telemetry.NewInstrumenter(telemetry.InstrumenterConfig{TracerProvider: provider})
	require.Same(t, provider, instr.TracerProvider())
	var disabled *telemetry.Instrumenter
	_, span := disabled.TracerProvider().Tracer("test").Start(t.Context(), "disabled")
	defer span.End()
	require.False(t, span.IsRecording())
}
