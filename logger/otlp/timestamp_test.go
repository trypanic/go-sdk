package otlp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
)

func TestOTLPWriterKeepsTimestampAndIntegerPrecision(t *testing.T) {
	exp := installRecordingProvider(t)
	w := NewWriter("svc")

	line := `{"level":"info","message":"m","time":"2026-11-03T10:00:00.123456789Z",` +
		`"snowflake":9007199254740993,"ratio":0.5,"negative":-42}` + "\n"
	_, err := w.Write([]byte(line))
	require.NoError(t, err)

	records := exp.snapshot()
	require.Len(t, records, 1)
	want := time.Date(2026, 11, 3, 10, 0, 0, 123456789, time.UTC)
	assert.True(t, records[0].Timestamp().Equal(want), "timestamp = %s, want %s", records[0].Timestamp(), want)

	attrs := map[string]attribute.Value{}
	records[0].WalkAttributes(func(kv attribute.KeyValue) bool {
		attrs[string(kv.Key)] = kv.Value
		return true
	})
	require.Contains(t, attrs, "snowflake")
	assert.Equal(t, attribute.INT64, attrs["snowflake"].Type())
	assert.Equal(t, int64(9007199254740993), attrs["snowflake"].AsInt64())
	assert.Equal(t, int64(-42), attrs["negative"].AsInt64())
	assert.Equal(t, attribute.FLOAT64, attrs["ratio"].Type())
	assert.InDelta(t, 0.5, attrs["ratio"].AsFloat64(), 0)
}

func TestOTLPWriterAcceptsLegacyUnixSeconds(t *testing.T) {
	exp := installRecordingProvider(t)
	w := NewWriter("svc")

	_, err := w.Write([]byte(`{"level":"info","message":"m","time":1793753281}`))
	require.NoError(t, err)

	records := exp.snapshot()
	require.Len(t, records, 1)
	assert.True(t, records[0].Timestamp().Equal(time.Unix(1793753281, 0)))
}
