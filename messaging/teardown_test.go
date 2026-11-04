package messaging

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// C5-013: a subscriber connection that does not close cleanly is reported by
// Subscribe instead of being dropped behind a nil return.
func TestSubscribeReportsTeardownCloseFailure(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	m, err := newPubSub("amqp://fake", b.dial, WithTopology(testTopology()))
	require.NoError(t, err)
	b.mu.Lock()
	b.connCloseErr = errors.New("close-ok never arrived")
	b.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- m.Subscribe(ctx, "q", "svc", func(context.Context, Message) error { return nil })
	}()
	require.Eventually(t, func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		return len(b.qos) == 1
	}, time.Second, time.Millisecond)
	cancel()

	err = <-done
	assert.Equal(t, ERR_MQ_CONNECTION_FAILED, requireAppError(t, err).Code())
}

// C5-013: closing a subscriber whose connection is already gone is not a
// failure, so a dropped link does not produce a spurious teardown error.
func TestSubscriberCloseAfterConnectionLoss(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	s := &Subscriber{
		amqpURL:     "amqp://fake",
		exchange:    Exchange{Name: "events", Type: "topic"},
		queue:       QueueTopology{Name: "q"},
		routingKeys: []string{"a"},
		dial:        b.dial,
	}
	require.NoError(t, s.connect())
	b.mu.Lock()
	b.conns[0].closed = true // the broker dropped the connection
	b.mu.Unlock()

	assert.NoError(t, s.Close())
}
