package messaging

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// C5-008: Close is terminal; a later Publish fails and never re-dials.
func TestPublisherPublishAfterCloseFailsWithoutDial(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	p := &Publisher{amqpURL: "amqp://fake", exchanges: testTopology().Exchanges, dial: b.dial}
	require.NoError(t, p.connect())
	require.NoError(t, p.Close())

	err := p.PublishWithContext(context.Background(), "events", "a", []byte(`{}`), nil)

	appErr := requireAppError(t, err)
	assert.Equal(t, ERR_MQ_CLOSED, appErr.Code())
	assert.False(t, appErr.Metadata.Retriable)
	assert.Equal(t, 1, b.dialCount(), "Publish after Close must not dial")
}

// C5-008: PubSub.Publish after Close fails at once, without dialing and
// without walking the queue's retry policy.
func TestPubSubPublishAfterCloseIsNotRetried(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	topo := testTopology()
	topo.Queues[0].Retry = &RetryConfig{MaxAttempts: 5, InitialDelay: 200, MaxDelay: 200, BackoffMult: 1}
	m, err := newPubSub("amqp://fake", b.dial, WithTopology(topo))
	require.NoError(t, err)
	require.NoError(t, m.Close())

	begin := time.Now()
	err = m.Publish(context.Background(), "q", Event{})

	assert.Equal(t, ERR_MQ_CLOSED, requireAppError(t, err).Code())
	assert.Less(t, time.Since(begin), 150*time.Millisecond, "a closed publisher is not retried")
	assert.Equal(t, 1, b.dialCount())
}

// C5-008: Close is idempotent.
func TestPublisherCloseTwice(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	p := &Publisher{amqpURL: "amqp://fake", exchanges: testTopology().Exchanges, dial: b.dial}
	require.NoError(t, p.connect())
	require.NoError(t, p.Close())
	assert.NoError(t, p.Close())
}

// C5-008: a reconnect that completes after Close does not leak its
// connection or revive the publisher.
func TestPublisherCloseDuringReconnectDoesNotLeak(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	p := newFakePubSub(b, testTopology())
	started, release := blockDials(t, b)

	done := make(chan error, 1)
	go func() { done <- p.Publish(context.Background(), "q", Event{}) }()
	<-started
	require.NoError(t, p.Close())
	release()

	err := <-done
	assert.Equal(t, ERR_MQ_CLOSED, requireAppError(t, err).Code())
	b.mu.Lock()
	defer b.mu.Unlock()
	require.Len(t, b.conns, 1)
	assert.True(t, b.conns[0].closed, "the connection dialed during Close must be closed")
}

// C5-008: PubSub.Close stops the subscriptions it started, and Subscribe
// after Close fails.
func TestPubSubCloseStopsSubscribers(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	m, err := newPubSub("amqp://fake", b.dial, WithTopology(testTopology()))
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		done <- m.Subscribe(context.Background(), "q", "svc", func(context.Context, Message) error { return nil })
	}()
	require.Eventually(t, func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		return len(b.qos) == 1
	}, time.Second, time.Millisecond, "subscriber did not start")

	require.NoError(t, m.Close())

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Subscribe kept running after Close")
	}

	err = m.Subscribe(context.Background(), "q", "svc", func(context.Context, Message) error { return nil })
	assert.Equal(t, ERR_MQ_CLOSED, requireAppError(t, err).Code())
}
