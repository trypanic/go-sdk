package telemetry_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/telemetry"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestEndRecordsClassifiedFailureWithoutPrivateDetails(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	_, span := provider.Tracer("test").Start(t.Context(), "operation")
	cause := errorkit.NewError(errorkit.ERR_CLIENT_RATE_LIMIT).With(errorkit.WithReason("password=private"))
	telemetry.End(span, fmt.Errorf("wrapped secret: %w", cause))
	ended := recorder.Ended()
	require.Len(t, ended, 1)
	require.Equal(t, codes.Error, ended[0].Status().Code)
	require.Len(t, ended[0].Events(), 1)
	require.NotContains(t, fmt.Sprint(ended[0].Events()), "private")
	require.NotContains(t, ended[0].Status().Description, "secret")
	attributes := map[string]any{}
	for _, a := range ended[0].Attributes() {
		attributes[string(a.Key)] = a.Value.AsInterface()
	}
	require.Equal(t, string(errorkit.ERR_CLIENT_RATE_LIMIT), attributes["error.code"])
	require.Equal(t, true, attributes["error.retriable"])
}
