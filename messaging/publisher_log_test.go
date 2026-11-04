package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// logLines decodes the JSON log lines in buf.
func logLines(t *testing.T, buf *syncBuffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if raw == "" {
			continue
		}
		var line map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &line), raw)
		lines = append(lines, line)
	}
	return lines
}

// C5-011: the publisher's reconnect diagnostics go to the WithLogger logger,
// at Warn level, with structured fields and the trace IDs of the publish.
func TestPublisherLogsThroughInjectedLogger(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	b.publishErr = func(attempt int, _, _ string) error {
		if attempt == 1 {
			return errors.New("channel died")
		}
		return nil
	}
	logs := &syncBuffer{}
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(tracetest.NewSpanRecorder()))
	m, err := newPubSub("amqp://fake", b.dial, WithTopology(testTopology()),
		WithLogger(newTestLogger(logs)), WithTracer(tp.Tracer("test")))
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, m.Publish(context.Background(), "q", Event{}))

	lines := logLines(t, logs)
	require.Len(t, lines, 1, "the publish failure must be logged through WithLogger")
	line := lines[0]
	assert.Equal(t, "warn", line["level"])
	assert.Equal(t, "events", line["exchange"])
	assert.Equal(t, "a", line["routing_key"])
	assert.Contains(t, line["error"], "channel died")
	assert.NotEmpty(t, line["trace_id"], "the log line carries the publish span's trace ID")
	assert.NotContains(t, line["message"], "channel died", "the error is a field, not part of the message")
}

// C5-011: subscriber reconnect attempts are logged with structured fields;
// retry_in is a duration string, not raw nanoseconds.
func TestWarnReconnectIsStructured(t *testing.T) {
	t.Parallel()

	logs := &syncBuffer{}
	warnReconnect(context.Background(), newTestLogger(logs), "q", errors.New("dial refused"), 5*time.Second)

	lines := logLines(t, logs)
	require.Len(t, lines, 1)
	assert.Equal(t, "warn", lines[0]["level"])
	assert.Equal(t, "q", lines[0]["queue"])
	assert.Equal(t, "5s", lines[0]["retry_in"])
	assert.Contains(t, lines[0]["error"], "dial refused")
}
