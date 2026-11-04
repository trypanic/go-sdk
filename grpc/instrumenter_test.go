package grpc

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/v2/telemetry"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestInstrumenterOptionsPassProvider(t *testing.T) {
	provider := sdktrace.NewTracerProvider()
	instr := telemetry.NewInstrumenter(telemetry.InstrumenterConfig{TracerProvider: provider})
	server, client := defaultServerBuild(), defaultClientBuild()
	WithServerInstrumenter(instr)(server)
	WithClientInstrumenter(instr)(client)
	require.Same(t, provider, server.tracerProvider)
	require.Same(t, provider, client.tracerProvider)
}
