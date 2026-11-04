package otlp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// recordingExporter is an in-memory sdklog.Exporter used to observe the
// records an OTLPWriter emits without any network traffic.
type recordingExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *recordingExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range records {
		e.records = append(e.records, r.Clone())
	}
	return nil
}

func (e *recordingExporter) Shutdown(context.Context) error   { return nil }
func (e *recordingExporter) ForceFlush(context.Context) error { return nil }

func (e *recordingExporter) snapshot() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]sdklog.Record(nil), e.records...)
}

// installRecordingProvider registers a synchronous SDK LoggerProvider backed
// by a recordingExporter as the global OTel LoggerProvider.
func installRecordingProvider(t *testing.T) *recordingExporter {
	t.Helper()
	exp := &recordingExporter{}
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exp)))
	previous := otel.GetLoggerProvider()
	otel.SetLoggerProvider(provider)
	t.Cleanup(func() {
		otel.SetLoggerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	return exp
}

func TestNewOTLPWriterResolvesGlobalProvider(t *testing.T) {
	exp := installRecordingProvider(t)

	w := NewOTLPWriter("svc")
	_, err := w.Write([]byte(`{"level":"info","message":"hello"}` + "\n"))
	require.NoError(t, err)

	records := exp.snapshot()
	require.Len(t, records, 1)
	assert.Equal(t, "hello", records[0].Body().AsString())
	assert.Equal(t, "svc", records[0].InstrumentationScope().Name)
}

func TestInitLogProviderRegistersGlobalProvider(t *testing.T) {
	previous := otel.GetLoggerProvider()
	t.Cleanup(func() { otel.SetLoggerProvider(previous) })

	// The exporter connects lazily and no record is written, so no traffic
	// leaves the process.
	handle := InitLogProvider(t.Context(), "svc", "127.0.0.1:1")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		handle.Shutdown(ctx)
	})

	assert.Same(t, handle.provider, otel.GetLoggerProvider())
}
