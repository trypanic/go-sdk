package messaging

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/logger"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// baseTestLogger is built once, before any test runs: logger.New writes
// zerolog globals (C1-001), which would race with parallel tests.
var baseTestLogger = logger.New(logger.Config{Env: logger.Prod, Writer: io.Discard})

// newTestLogger returns a logger writing to w, derived from baseTestLogger
// without touching zerolog globals.
func newTestLogger(w io.Writer) *logger.Logger {
	l := baseTestLogger.WithFields(nil)
	*l.Zerolog() = l.Zerolog().Output(w)
	return l
}

// newObservedPubSub returns a PubSub whose spans go to the returned recorder
// and whose logs go to the returned buffer.
func newObservedPubSub() (*PubSub, *tracetest.SpanRecorder, *syncBuffer) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	logs := &syncBuffer{}
	p := &PubSub{
		tracer: tp.Tracer("test"),
		log:    newTestLogger(logs),
	}
	return p, rec, logs
}

func receiveSpan(t *testing.T, rec *tracetest.SpanRecorder) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, s := range rec.Ended() {
		if s.Name() == "messaging.receive q" {
			return s
		}
	}
	t.Fatal("no receive span recorded")
	return nil
}

// C5-005: a failed Ack is recorded on the receive span and logged through
// the injected logger, since the broker will redeliver the message.
func TestHandleDeliveryReportsAckFailure(t *testing.T) {
	t.Parallel()

	p, rec, logs := newObservedPubSub()
	ack := &fakeAcknowledger{ackErr: errors.New("channel/connection is not open")}

	p.handleDelivery(context.Background(), "q", newTestDelivery(ack), QueueTopology{Name: "q"},
		func(context.Context, Message) error { return nil })

	span := receiveSpan(t, rec)
	assert.Equal(t, codes.Error, span.Status().Code)
	require.NotEmpty(t, span.Events(), "the ack error must be recorded on the span")
	assert.Equal(t, "exception", span.Events()[0].Name)
	assert.Contains(t, logs.String(), "ack failed")
	assert.Contains(t, logs.String(), `"level":"warn"`)
}

// C5-005: a failed Nack is reported the same way.
func TestHandleDeliveryReportsNackFailure(t *testing.T) {
	t.Parallel()

	p, rec, logs := newObservedPubSub()
	ack := &fakeAcknowledger{nackErr: errors.New("channel/connection is not open")}

	p.handleDelivery(context.Background(), "q", newTestDelivery(ack), QueueTopology{Name: "q"},
		func(context.Context, Message) error { return errors.New("handler failed") })

	span := receiveSpan(t, rec)
	var found bool
	for _, e := range span.Events() {
		for _, a := range e.Attributes {
			if string(a.Key) == "exception.message" && bytes.Contains([]byte(a.Value.AsString()), []byte("nack failed")) {
				found = true
			}
		}
	}
	assert.True(t, found, "the nack error must be recorded on the span")
	assert.Contains(t, logs.String(), "nack failed")
}

// C5-005: a requeue Nack that fails during shutdown is logged too.
func TestHandleDeliveryReportsShutdownNackFailure(t *testing.T) {
	t.Parallel()

	p, _, logs := newObservedPubSub()
	ack := &fakeAcknowledger{nackErr: errors.New("channel/connection is not open")}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p.handleDelivery(ctx, "q", newTestDelivery(ack), QueueTopology{Name: "q"},
		func(context.Context, Message) error { return nil })

	assert.Contains(t, logs.String(), "nack failed")
}
