package messaging

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// C5-002: the publisher channel runs in confirm mode and every message is
// published as mandatory, so loss is reported instead of ignored.
func TestPublishUsesConfirmModeAndMandatory(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	p := newFakePubSub(b, testTopology())

	require.NoError(t, p.Publish(context.Background(), "q", Event{}))

	b.mu.Lock()
	confirms := b.confirms
	b.mu.Unlock()
	assert.Equal(t, 1, confirms, "publisher channel must be in confirm mode")
	got := b.publishes()
	require.Len(t, got, 1)
	assert.True(t, got[0].Mandatory, "messages must be published as mandatory")
}

// C5-002: a broker nack is a publish failure.
func TestPublishReturnsErrorOnNack(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	b.nack = func(int) bool { return true }
	p := newFakePubSub(b, testTopology())

	err := p.Publish(context.Background(), "q", Event{})

	assert.Equal(t, ERR_MQ_PUBLISH_FAILED, requireAppError(t, err).Code())
}

// C5-002: an unroutable message (basic.return) is a publish failure, even
// though RabbitMQ acks it afterwards.
func TestPublishReturnsErrorWhenUnroutable(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	b.unroutable = func(int) bool { return true }
	p := newFakePubSub(b, testTopology())

	err := p.Publish(context.Background(), "q", Event{})

	appErr := requireAppError(t, err)
	assert.Equal(t, ERR_MQ_PUBLISH_FAILED, appErr.Code())
	assert.Contains(t, appErr.Error(), "NO_ROUTE")
}

// C5-002: a return left over from an earlier publish is not attributed to
// the next one. A return stays unread only when the broker nacks the
// returned message (awaitConfirm stops at the nack).
func TestPublishIgnoresStaleReturn(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	b.unroutable = func(attempt int) bool { return attempt == 1 }
	b.nack = func(attempt int) bool { return attempt == 1 }
	p := newFakePubSub(b, testTopology())

	require.Error(t, p.Publish(context.Background(), "q", Event{}))
	assert.NoError(t, p.Publish(context.Background(), "q", Event{}))
}

// C5-002: waiting for the confirm is bounded by ctx; the channel whose
// confirm was abandoned is not reused.
func TestPublishConfirmWaitHonoursDeadline(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	b.holdConfirms = true
	p := newFakePubSub(b, testTopology())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	begin := time.Now()
	err := p.Publish(ctx, "q", Event{})

	assert.Less(t, time.Since(begin), time.Second)
	assert.Equal(t, ERR_MQ_TIMEOUT, requireAppError(t, err).Code())
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	b.mu.Lock()
	b.holdConfirms = false
	b.mu.Unlock()
	require.NoError(t, p.Publish(context.Background(), "q", Event{}))
	assert.Equal(t, 2, b.dialCount(), "a channel with an abandoned confirm must be replaced")
}

// C5-002: WithDeclareTopology declares every queue (with its arguments) and
// binding on the publisher connection, so messages published before the
// first Subscribe are not dropped.
func TestDeclareTopologyDeclaresQueuesAndBindings(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	topo := &Topology{
		Exchanges: []Exchange{{Name: "events", Type: "topic"}, {Name: "dlx", Type: "direct"}},
		Queues: []QueueTopology{
			{Name: "q", DLX: &DLXConfig{Exchange: "dlx", RoutingKey: "q.dead"}},
			{Name: "q.dlq"},
		},
		Bindings: []Binding{
			{Queue: "q", Exchange: "events", RoutingKeys: []string{"a", "b"}},
			{Queue: "q.dlq", Exchange: "dlx", RoutingKeys: []string{"q.dead"}},
		},
	}

	m, err := newPubSub("amqp://fake", b.dial, WithTopology(topo), WithDeclareTopology())
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })

	b.mu.Lock()
	defer b.mu.Unlock()
	assert.Equal(t, []string{"events", "dlx"}, b.exchanges)
	assert.Equal(t, []string{"q", "q.dlq"}, b.queues)
	assert.Equal(t, []string{"events->q:a", "events->q:b", "dlx->q.dlq:q.dead"}, b.bindings)
}

// Without WithDeclareTopology queues stay lazily declared by Subscribe.
func TestNewPubSubDeclaresOnlyExchangesByDefault(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	m, err := newPubSub("amqp://fake", b.dial, WithTopology(testTopology()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })

	b.mu.Lock()
	defer b.mu.Unlock()
	assert.Equal(t, []string{"events"}, b.exchanges)
	assert.Empty(t, b.queues)
	assert.Empty(t, b.bindings)
}
