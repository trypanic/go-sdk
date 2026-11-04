package messaging

import (
	"context"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAcknowledger records how a delivery was settled. It never talks to a
// broker.
type fakeAcknowledger struct {
	mu       sync.Mutex
	acks     int
	nacks    int
	requeues []bool
	ackErr   error
	nackErr  error
}

func (f *fakeAcknowledger) Ack(_ uint64, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acks++
	return f.ackErr
}

func (f *fakeAcknowledger) Nack(_ uint64, _ bool, requeue bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nacks++
	f.requeues = append(f.requeues, requeue)
	return f.nackErr
}

func (f *fakeAcknowledger) Reject(_ uint64, requeue bool) error {
	return f.Nack(0, false, requeue)
}

func (f *fakeAcknowledger) snapshot() (acks, nacks int, requeues []bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.acks, f.nacks, append([]bool(nil), f.requeues...)
}

func newTestDelivery(ack amqp.Acknowledger) amqp.Delivery {
	return amqp.Delivery{
		Acknowledger: ack,
		DeliveryTag:  1,
		Exchange:     "events",
		RoutingKey:   "order.created",
		Body:         []byte(`{}`),
	}
}

func TestHandleDeliveryAcksOnSuccess(t *testing.T) {
	t.Parallel()

	ack := &fakeAcknowledger{}
	p := &PubSub{}
	calls := 0
	p.handleDelivery(context.Background(), "q", newTestDelivery(ack), QueueTopology{Name: "q"},
		func(context.Context, Message) error {
			calls++
			return nil
		})

	acks, nacks, _ := ack.snapshot()
	assert.Equal(t, 1, calls)
	assert.Equal(t, 1, acks)
	assert.Equal(t, 0, nacks)
}

// C5-001: a handler that observes the cancelled subscription ctx must not
// dead-letter the message; it is requeued so another consumer can take it.
func TestHandleDeliveryRequeuesWhenHandlerSeesShutdown(t *testing.T) {
	t.Parallel()

	ack := &fakeAcknowledger{}
	p := &PubSub{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := 0
	queue := QueueTopology{Name: "q", Retry: &RetryConfig{MaxAttempts: 4, InitialDelay: 200, MaxDelay: 400, BackoffMult: 2}}
	start := time.Now()
	p.handleDelivery(ctx, "q", newTestDelivery(ack), queue, func(hctx context.Context, _ Message) error {
		calls++
		cancel()
		return hctx.Err()
	})

	acks, nacks, requeues := ack.snapshot()
	assert.Equal(t, 1, calls, "no retry once the subscription ctx is cancelled")
	assert.Less(t, time.Since(start), 150*time.Millisecond, "no backoff wait during shutdown")
	assert.Equal(t, 0, acks)
	require.Equal(t, 1, nacks)
	assert.Equal(t, []bool{true}, requeues, "message must be requeued, not dead-lettered")
}

// C5-001: a delivery picked up after the ctx is already cancelled is requeued
// without calling the handler or waiting for any backoff.
func TestHandleDeliveryRequeuesWhenCtxAlreadyCancelled(t *testing.T) {
	t.Parallel()

	ack := &fakeAcknowledger{}
	p := &PubSub{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	queue := QueueTopology{Name: "q", Retry: &RetryConfig{MaxAttempts: 4, InitialDelay: 200, MaxDelay: 400, BackoffMult: 2}}
	start := time.Now()
	p.handleDelivery(ctx, "q", newTestDelivery(ack), queue, func(hctx context.Context, _ Message) error {
		calls++
		return hctx.Err()
	})

	_, nacks, requeues := ack.snapshot()
	assert.Equal(t, 0, calls)
	assert.Less(t, time.Since(start), 150*time.Millisecond)
	require.Equal(t, 1, nacks)
	assert.Equal(t, []bool{true}, requeues)
}
