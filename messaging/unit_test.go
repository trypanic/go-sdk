package messaging

import (
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestBuildQueueArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		queue QueueTopology
		want  amqp.Table
	}{
		{"empty", QueueTopology{}, amqp.Table{}},
		{
			"limits and mode",
			QueueTopology{TTL: 60000, MaxLength: 100, Overflow: "reject-publish", QueueMode: "lazy"},
			amqp.Table{"x-message-ttl": int64(60000), "x-max-length": 100, "x-overflow": "reject-publish", "x-queue-mode": "lazy"},
		},
		{
			"dlx with routing key",
			QueueTopology{DLX: &DLXConfig{Exchange: "dlx", RoutingKey: "q.dead"}},
			amqp.Table{"x-dead-letter-exchange": "dlx", "x-dead-letter-routing-key": "q.dead"},
		},
		{
			"dlx without routing key",
			QueueTopology{DLX: &DLXConfig{Exchange: "dlx"}},
			amqp.Table{"x-dead-letter-exchange": "dlx"},
		},
		{
			"whole JSON numbers become int64, others are kept",
			QueueTopology{Arguments: map[string]any{"x-max-priority": float64(10), "x-ratio": 0.5, "x-name": "n"}},
			amqp.Table{"x-max-priority": int64(10), "x-ratio": 0.5, "x-name": "n"},
		},
		{
			"custom arguments override the typed fields",
			QueueTopology{MaxLength: 100, Arguments: map[string]any{"x-max-length": float64(5)}},
			amqp.Table{"x-max-length": int64(5)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, buildQueueArgs(tc.queue))
		})
	}
}

func TestCarrierGet(t *testing.T) {
	t.Parallel()

	c := newCarrier(amqp.Table{"s": "v", "b": []byte("bytes"), "i": int32(7), "nil": nil})

	tests := []struct {
		key  string
		want string
	}{
		{"s", "v"},
		{"b", "bytes"},
		{"i", "7"},
		{"nil", ""},
		{"missing", ""},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, c.Get(tc.key), tc.key)
	}
	assert.ElementsMatch(t, []string{"s", "b", "i", "nil"}, c.Keys())
}

func TestCarrierSetOnNilTable(t *testing.T) {
	t.Parallel()

	c := newCarrier(nil)
	c.Set("k", "v")

	assert.Equal(t, "v", c.Get("k"))
	assert.Equal(t, []string{"k"}, c.Keys())
}

// The carrier round-trips W3C trace context through AMQP headers.
func TestCarrierPropagatesTraceContext(t *testing.T) {
	t.Parallel()

	tp := sdktrace.NewTracerProvider()
	ctx, span := tp.Tracer("test").Start(context.Background(), "producer")
	defer span.End()
	headers := amqp.Table{}
	prop := propagation.TraceContext{}

	prop.Inject(ctx, newCarrier(headers))
	got := trace.SpanContextFromContext(prop.Extract(context.Background(), newCarrier(headers)))

	assert.Equal(t, span.SpanContext().TraceID(), got.TraceID())
	assert.Equal(t, span.SpanContext().SpanID(), got.SpanID())
}

// A handler that fails and then succeeds is retried per the queue policy
// and the message is acked.
func TestHandleDeliveryRetriesThenAcks(t *testing.T) {
	t.Parallel()

	ack := &fakeAcknowledger{}
	p := &PubSub{}
	calls := 0
	queue := QueueTopology{Name: "q", Retry: &RetryConfig{MaxAttempts: 3, InitialDelay: 1, MaxDelay: 1, BackoffMult: 1}}

	p.handleDelivery(context.Background(), "q", newTestDelivery(ack), queue, func(context.Context, Message) error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})

	acks, nacks, _ := ack.snapshot()
	assert.Equal(t, 3, calls)
	assert.Equal(t, 1, acks)
	assert.Equal(t, 0, nacks)
}

// A handler that keeps failing is called MaxAttempts times and the message
// is nacked without requeue (to the DLX).
func TestHandleDeliveryNacksAfterRetriesExhausted(t *testing.T) {
	t.Parallel()

	ack := &fakeAcknowledger{}
	p := &PubSub{}
	calls := 0
	queue := QueueTopology{Name: "q", Retry: &RetryConfig{MaxAttempts: 3, InitialDelay: 1, MaxDelay: 1, BackoffMult: 1}}

	p.handleDelivery(context.Background(), "q", newTestDelivery(ack), queue, func(context.Context, Message) error {
		calls++
		return errors.New("permanent")
	})

	acks, nacks, requeues := ack.snapshot()
	assert.Equal(t, 3, calls)
	assert.Equal(t, 0, acks)
	require.Equal(t, 1, nacks)
	assert.Equal(t, []bool{false}, requeues)
}

// Without a retry policy a failing handler runs once.
func TestHandleDeliveryWithoutRetryRunsOnce(t *testing.T) {
	t.Parallel()

	ack := &fakeAcknowledger{}
	p := &PubSub{}
	calls := 0

	p.handleDelivery(context.Background(), "q", newTestDelivery(ack), QueueTopology{Name: "q"}, func(context.Context, Message) error {
		calls++
		return errors.New("fail")
	})

	_, nacks, requeues := ack.snapshot()
	assert.Equal(t, 1, calls)
	require.Equal(t, 1, nacks)
	assert.Equal(t, []bool{false}, requeues)
}

// failDials makes the first n dials of b fail with a retriable error and
// records the virtual time of every dial since start.
func failDials(b *fakeBroker, n int, start time.Time, times *[]time.Duration) {
	b.dialHook = func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		*times = append(*times, time.Since(start))
		if len(*times) <= n {
			b.dialErr = errors.New("connection refused")
		} else {
			b.dialErr = nil
		}
	}
}

// Reconnect delays sample below an exponentially growing cap.
func TestReconnectWithBackoffSchedule(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newFakeBroker()
		var times []time.Duration
		failDials(b, 3, time.Now(), &times)

		err := reconnectWithBackoff(t.Context(), newFakeSubscriber(b), newTestLogger(io.Discard))

		require.NoError(t, err)
		require.Len(t, times, 4)
		previous := time.Duration(0)
		for i, cap := range []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second} {
			require.GreaterOrEqual(t, times[i]-previous, time.Duration(0))
			require.Less(t, times[i]-previous, cap)
			previous = times[i]
		}
	})
}

// The delay is capped at MaxReconnectDelay.
func TestReconnectWithBackoffCapsDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newFakeBroker()
		var times []time.Duration
		failDials(b, 6, time.Now(), &times)

		err := reconnectWithBackoff(t.Context(), newFakeSubscriber(b), newTestLogger(io.Discard))

		require.NoError(t, err)
		require.Len(t, times, 7)
		gaps := make([]time.Duration, 0, len(times))
		prev := time.Duration(0)
		for _, at := range times {
			gaps = append(gaps, at-prev)
			prev = at
		}
		for i, cap := range []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, MaxReconnectDelay, MaxReconnectDelay, MaxReconnectDelay} {
			require.GreaterOrEqual(t, gaps[i], time.Duration(0))
			require.Less(t, gaps[i], cap)
		}
	})
}

// Cancelling the ctx ends the loop with a retriable connection error that
// wraps the ctx error, without another dial.
func TestReconnectWithBackoffStopsOnCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newFakeBroker()
		b.dialErr = errors.New("connection refused")
		ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second) // full-jitter retry count varies within this budget
		defer cancel()

		err := reconnectWithBackoff(ctx, newFakeSubscriber(b), newTestLogger(io.Discard))

		appErr := requireAppError(t, err)
		assert.Equal(t, ERR_MQ_CONNECTION_FAILED, appErr.Code())
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Positive(t, b.dialCount())
		calls := b.dialCount()
		time.Sleep(time.Minute)
		assert.Equal(t, calls, b.dialCount(), "no reconnect can outlive cancellation")
	})
}
