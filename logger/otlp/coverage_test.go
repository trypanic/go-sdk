package otlp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/noop"
	"go.opentelemetry.io/otel/trace"
)

var (
	testTraceID = trace.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36}
	testSpanID  = trace.SpanID{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7}
)

func TestOTLPWriterMapsRecordFields(t *testing.T) {
	exp := installRecordingProvider(t)
	w := NewOTLPWriter("svc")

	line := `{"level":"warn","message":"slow","time":"2026-11-03T10:00:00Z",` +
		`"trace_id":"` + testTraceID.String() + `","span_id":"` + testSpanID.String() + `",` +
		`"caller":"x.go:1","flag":true,"nested":{"a":1},"list":[1,"b"],"none":null,"name":"n"}`
	_, err := w.Write([]byte(line))
	require.NoError(t, err)

	records := exp.snapshot()
	require.Len(t, records, 1)
	r := records[0]
	assert.Equal(t, otellog.SeverityWarn, r.Severity())
	assert.Equal(t, "warn", r.SeverityText())
	assert.Equal(t, "slow", r.Body().AsString())
	assert.Equal(t, testTraceID, r.TraceID())
	assert.Equal(t, testSpanID, r.SpanID())

	attrs := map[string]attribute.Value{}
	r.WalkAttributes(func(kv attribute.KeyValue) bool {
		attrs[string(kv.Key)] = kv.Value
		return true
	})
	for _, dedicated := range []string{"level", "time", "message", "trace_id", "span_id", "caller"} {
		assert.NotContains(t, attrs, dedicated)
	}
	assert.Equal(t, attribute.BoolValue(true), attrs["flag"])
	assert.Equal(t, attribute.StringValue(`{"a":1}`), attrs["nested"])
	assert.Equal(t, attribute.StringValue(`[1,"b"]`), attrs["list"])
	assert.Equal(t, attribute.StringValue("null"), attrs["none"])
	assert.Equal(t, attribute.StringValue("n"), attrs["name"])
}

func TestOTLPWriterSeverityMapping(t *testing.T) {
	tests := []struct {
		level string
		want  otellog.Severity
	}{
		{level: "trace", want: otellog.SeverityTrace},
		{level: "debug", want: otellog.SeverityDebug},
		{level: "info", want: otellog.SeverityInfo},
		{level: "warn", want: otellog.SeverityWarn},
		{level: "error", want: otellog.SeverityError},
		{level: "fatal", want: otellog.SeverityFatal},
		{level: "panic", want: otellog.SeverityFatal},
		{level: "custom", want: otellog.SeverityUndefined},
	}
	for _, tc := range tests {
		t.Run(tc.level, func(t *testing.T) {
			assert.Equal(t, tc.want, zerologLevelToOTEL(tc.level))
		})
	}
}

func TestOTLPWriterEmitsMalformedLinesAsText(t *testing.T) {
	exp := installRecordingProvider(t)

	_, err := NewOTLPWriter("svc").Write([]byte("not json"))
	require.NoError(t, err)

	records := exp.snapshot()
	require.Len(t, records, 1)
	assert.Equal(t, "not json", records[0].Body().AsString())
	assert.Equal(t, otellog.SeverityError, records[0].Severity())
}

func TestOTLPWriterIgnoresInvalidTraceIDs(t *testing.T) {
	exp := installRecordingProvider(t)

	_, err := NewOTLPWriter("svc").Write([]byte(`{"level":"info","message":"m","trace_id":"zz","span_id":"yy"}`))
	require.NoError(t, err)

	records := exp.snapshot()
	require.Len(t, records, 1)
	assert.False(t, records[0].TraceID().IsValid())
}

func TestSetupOTLPReturnsHandleAndWriter(t *testing.T) {
	restoreGlobalLoggerProvider(t)

	handle, writer := SetupOTLP(t.Context(), "svc", "127.0.0.1:1")
	require.NotNil(t, handle)
	require.NotNil(t, writer)
	assert.Same(t, handle.provider, otel.GetLoggerProvider())

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	assert.NoError(t, handle.Close(ctx))
}

// TestInitLogProviderNeverFailsOnEmptyArguments: InitLogProvider and
// SetupOTLP keep their documented never-fail behavior. An empty service name
// or endpoint (which NewLogProvider rejects) still yields a working,
// globally registered provider, as before NewLogProvider existed.
func TestInitLogProviderNeverFailsOnEmptyArguments(t *testing.T) {
	t.Run("empty service name exports to the collector", func(t *testing.T) {
		restoreGlobalLoggerProvider(t)
		collector := startFakeCollector(t)

		handle := InitLogProvider(t.Context(), "", collector.addr)
		require.NotNil(t, handle)
		require.Same(t, handle.provider, otel.GetLoggerProvider())

		_, err := NewOTLPWriter("svc").Write([]byte(`{"level":"info","message":"no name"}`))
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		require.NoError(t, handle.Close(ctx))

		methods, _ := collector.calls()
		assert.Len(t, methods, 1, "records must still be exported")
	})

	t.Run("empty endpoint registers a provider", func(t *testing.T) {
		restoreGlobalLoggerProvider(t)

		// Nothing is emitted and the exporter connects lazily: no traffic.
		handle := InitLogProvider(t.Context(), "svc", "")
		require.NotNil(t, handle)
		assert.Same(t, handle.provider, otel.GetLoggerProvider())
		assert.NoError(t, handle.Close(t.Context()))
	})

	t.Run("SetupOTLP with empty arguments", func(t *testing.T) {
		restoreGlobalLoggerProvider(t)

		handle, writer := SetupOTLP(t.Context(), "", "")
		require.NotNil(t, writer)
		assert.Same(t, handle.provider, otel.GetLoggerProvider())
		assert.NoError(t, handle.Close(t.Context()))
	})
}

func BenchmarkOTLPWriterWrite(b *testing.B) {
	w := &OTLPWriter{logger: noop.NewLoggerProvider().Logger("bench")}
	line := []byte(`{"level":"info","message":"request served","time":"2026-11-03T10:00:00.123456789Z","request_id":"rid-42","status":200}`)
	b.ReportAllocs()
	for b.Loop() {
		_, _ = w.Write(line)
	}
}
