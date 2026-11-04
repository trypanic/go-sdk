package messaging

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/telemetry"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestInstrumenterOptionRecordsThroughProvider(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	instr := telemetry.NewInstrumenter(telemetry.InstrumenterConfig{TracerProvider: provider})
	build := &pubSubBuild{}
	WithInstrumenter(instr)(build)
	_, span := build.tracer.Start(t.Context(), "operation")
	span.End()
	require.Len(t, recorder.Ended(), 1)
	WithInstrumenter(nil)(build)
	_, span = build.tracer.Start(t.Context(), "disabled")
	span.End()
	require.Len(t, recorder.Ended(), 1)
}
