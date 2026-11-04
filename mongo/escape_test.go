package mongo_test

import (
	"testing"

	"github.com/trypanic/go-sdk/telemetry"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/mongo"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestBorrowedClientEscapeHatch(t *testing.T) {
	driver, err := mongodriver.Connect(options.Client().ApplyURI("mongodb://127.0.0.1:1").SetMaxPoolSize(7))
	require.NoError(t, err)
	t.Cleanup(func() { _ = driver.Disconnect(t.Context()) })
	client, err := mongo.NewFromClient(driver, "orders", nil)
	require.NoError(t, err)
	require.Same(t, driver, client.Unwrap())
	require.Equal(t, "orders", client.Database().Name())
	require.NoError(t, client.Close(t.Context()))
	// The wrapper does not disconnect the borrowed driver.
	require.NoError(t, driver.Disconnect(t.Context()))
}

func TestBorrowedClientTracingAndNilOptOut(t *testing.T) {
	driver, err := mongodriver.Connect(options.Client().ApplyURI("mongodb://127.0.0.1:1"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = driver.Disconnect(t.Context()) })
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	instr := telemetry.NewInstrumenter(telemetry.InstrumenterConfig{TracerProvider: provider})
	client, err := mongo.NewFromClient(driver, "db", instr)
	require.NoError(t, err)
	require.NoError(t, client.Close(t.Context()))
	require.Len(t, recorder.Ended(), 1)
	plain, err := mongo.NewFromClient(driver, "db", nil)
	require.NoError(t, err)
	require.NoError(t, plain.Close(t.Context()))
	require.Len(t, recorder.Ended(), 1)
	require.Same(t, plain, mongo.WrapWithInstrumenter(plain, nil))
}
