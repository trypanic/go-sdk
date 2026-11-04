package mongodb_test

import (
	"testing"

	"github.com/trypanic/go-sdk/telemetry"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/stretchr/testify/require"
	mongodb "github.com/trypanic/go-sdk/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestBorrowedClientEscapeHatch(t *testing.T) {
	driver, err := mongo.Connect(options.Client().ApplyURI("mongodb://127.0.0.1:1").SetMaxPoolSize(7))
	require.NoError(t, err)
	t.Cleanup(func() { _ = driver.Disconnect(t.Context()) })
	client, err := mongodb.NewFromClient(driver, "orders", nil)
	require.NoError(t, err)
	require.Same(t, driver, client.Unwrap())
	require.Equal(t, "orders", client.Database().Name())
	require.NoError(t, client.Close(t.Context()))
	// The wrapper does not disconnect the borrowed driver.
	require.NoError(t, driver.Disconnect(t.Context()))
}

func TestBorrowedClientTracingAndNilOptOut(t *testing.T) {
	driver, err := mongo.Connect(options.Client().ApplyURI("mongodb://127.0.0.1:1"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = driver.Disconnect(t.Context()) })
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	instr := telemetry.NewInstrumenter(telemetry.InstrumenterConfig{TracerProvider: provider})
	client, err := mongodb.NewFromClient(driver, "db", instr)
	require.NoError(t, err)
	require.NoError(t, client.Close(t.Context()))
	require.Len(t, recorder.Ended(), 1)
	plain, err := mongodb.NewFromClient(driver, "db", nil)
	require.NoError(t, err)
	require.NoError(t, plain.Close(t.Context()))
	require.Len(t, recorder.Ended(), 1)
	require.Same(t, plain, mongodb.WrapWithInstrumenter(plain, nil))
}
