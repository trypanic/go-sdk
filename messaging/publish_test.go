package messaging

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFakePubSub returns a PubSub whose publisher dials the fake broker
// lazily (no connection yet).
func newFakePubSub(b *fakeBroker, topo *Topology) *PubSub {
	return &PubSub{
		topology:  topo,
		amqpURL:   "amqp://fake",
		publisher: &Publisher{amqpURL: "amqp://fake", exchanges: topo.Exchanges, dial: b.dial},
	}
}

// blockDials makes every dial wait until the returned release func is
// called (or the safety timeout elapses, so a regression fails instead of
// hanging). started is closed when the first dial begins.
func blockDials(t *testing.T, b *fakeBroker) (started <-chan struct{}, release func()) {
	t.Helper()
	gate := make(chan struct{})
	first := make(chan struct{})
	var once, releaseOnce sync.Once
	b.dialHook = func() {
		once.Do(func() { close(first) })
		select {
		case <-gate:
		case <-time.After(3 * time.Second):
		}
	}
	release = func() { releaseOnce.Do(func() { close(gate) }) }
	t.Cleanup(release)
	return first, release
}

// C5-007: Publish with an already-cancelled ctx returns a ctx error and
// never dials the broker.
func TestPublishWithCancelledCtxDoesNotDial(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	p := newFakePubSub(b, testTopology())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.Publish(ctx, "q", Event{})

	_ = requireAppError(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, b.dialCount())
}

// C5-007: a caller waiting behind a slow reconnect gives up at its own
// deadline instead of queueing on the publisher mutex.
func TestPublishHonoursDeadlineWhileAnotherPublishReconnects(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	p := newFakePubSub(b, testTopology())
	started, release := blockDials(t, b)

	go func() { _ = p.Publish(context.Background(), "q", Event{}) }()
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	begin := time.Now()
	err := p.Publish(ctx, "q", Event{})
	elapsed := time.Since(begin)
	release()

	assert.Less(t, elapsed, time.Second, "Publish must return at its deadline")
	assert.Equal(t, ERR_MQ_TIMEOUT, requireAppError(t, err).Code())
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

// C5-007: the dial runs outside the publisher's state mutex, so Close is not
// blocked by a reconnect in progress.
func TestPublisherCloseDoesNotWaitForDial(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	p := newFakePubSub(b, testTopology())
	started, release := blockDials(t, b)

	go func() { _ = p.Publish(context.Background(), "q", Event{}) }()
	<-started

	begin := time.Now()
	_ = p.publisher.Close()
	elapsed := time.Since(begin)
	release()

	assert.Less(t, elapsed, time.Second, "Close must not wait for the dial")
}

// C5-007: the ctx deadline also bounds the AMQP handshake against a peer
// that accepts the TCP connection but never answers.
func TestPublishDeadlineBoundsHandshake(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	var mu sync.Mutex
	var held []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			held = append(held, c) // keep it open, never answer
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		for _, c := range held {
			_ = c.Close()
		}
	})

	url := "amqp://guest:guest@" + ln.Addr().String() + "/"
	p := &PubSub{topology: testTopology(), amqpURL: url, publisher: &Publisher{amqpURL: url}}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	begin := time.Now()
	err = p.Publish(ctx, "q", Event{})

	assert.Less(t, time.Since(begin), 2*time.Second)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "got %v", err)
}
