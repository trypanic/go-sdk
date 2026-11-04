package messaging

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/v2/errorkit"
)

// newDroppingListener starts a 127.0.0.1:0 TCP listener that accepts every
// connection and closes it immediately, so an AMQP dial fails during the
// handshake. It returns an amqp:// URL for it and the number of accepted
// connections. No broker is involved.
func newDroppingListener(t *testing.T) (string, *atomic.Int32) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	var accepts atomic.Int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			accepts.Add(1)
			_ = conn.Close()
		}
	}()
	return "amqp://guest:guest@" + ln.Addr().String() + "/", &accepts
}

func requireAppError(t *testing.T, err error) *errorkit.AppError {
	t.Helper()
	require.Error(t, err)
	var appErr *errorkit.AppError
	require.True(t, errors.As(err, &appErr), "expected *errorkit.AppError, got %T: %v", err, err)
	return appErr
}

// assertNotDoubleWrapped fails when the AppError wraps another AppError, i.e.
// when an SDK error was re-encoded instead of being returned unchanged.
func assertNotDoubleWrapped(t *testing.T, appErr *errorkit.AppError) {
	t.Helper()
	var inner *errorkit.AppError
	assert.False(t, errors.As(appErr.Unwrap(), &inner),
		"%s wraps another AppError (%v); SDK errors must be returned unchanged", appErr.Code(), inner)
}

func testTopology() *Topology {
	return &Topology{
		Exchanges: []Exchange{{Name: "events", Type: "topic", Durable: true}},
		Queues:    []QueueTopology{{Name: "q", Durable: true}},
		Bindings:  []Binding{{Queue: "q", Exchange: "events", RoutingKeys: []string{"a"}}},
	}
}

// C5-006: a broker outage seen by Subscribe keeps its retriable MQ code
// instead of becoming a non-retriable ERR_INTERNAL.
func TestSubscribePreservesConnectionErrorCode(t *testing.T) {
	t.Parallel()

	url, _ := newDroppingListener(t)
	p := &PubSub{topology: testTopology(), amqpURL: url}

	err := p.Subscribe(context.Background(), "q", "svc", func(context.Context, Message) error { return nil })

	appErr := requireAppError(t, err)
	assert.Equal(t, ERR_MQ_CONNECTION_FAILED, appErr.Code())
	assert.True(t, appErr.Metadata.Retriable)
	assertNotDoubleWrapped(t, appErr)
}

func TestNewPublisherDoesNotRewrapConnectError(t *testing.T) {
	t.Parallel()

	url, _ := newDroppingListener(t)
	_, err := NewPublisher(url, nil)

	appErr := requireAppError(t, err)
	assert.Equal(t, ERR_MQ_CONNECTION_FAILED, appErr.Code())
	assertNotDoubleWrapped(t, appErr)
}

func TestSubscriberReconnectDoesNotRewrapConnectError(t *testing.T) {
	t.Parallel()

	url, _ := newDroppingListener(t)
	s := &Subscriber{amqpURL: url, queue: QueueTopology{Name: "q"}}

	appErr := requireAppError(t, s.Reconnect())
	assert.Equal(t, ERR_MQ_CONNECTION_FAILED, appErr.Code())
	assertNotDoubleWrapped(t, appErr)
}

// Publish returns the publisher's AppError unchanged (AGENTS §2.1) instead of
// re-encoding it as ERR_MQ_PUBLISH_FAILED.
func TestPublishPreservesPublisherErrorCode(t *testing.T) {
	t.Parallel()

	url, _ := newDroppingListener(t)
	p := &PubSub{topology: testTopology(), amqpURL: url, publisher: &Publisher{amqpURL: url}}

	err := p.Publish(context.Background(), "q", Event{})

	appErr := requireAppError(t, err)
	assert.Equal(t, ERR_MQ_CONNECTION_FAILED, appErr.Code())
	assertNotDoubleWrapped(t, appErr)
}

func newFakeSubscriber(b *fakeBroker) *Subscriber {
	return &Subscriber{
		amqpURL:     "amqp://fake",
		dial:        b.dial,
		exchange:    Exchange{Name: "events", Type: "topic"},
		queue:       QueueTopology{Name: "q"},
		consumer:    "svc",
		routingKeys: []string{"a"},
	}
}

// C5-006 (approval part): each AMQP failure path reports its specific code;
// access refused (403) is ERR_MQ_AUTH_FAILED and not retriable.
func TestSubscriberConnectErrorCodes(t *testing.T) {
	t.Parallel()

	refused := &amqp.Error{Code: amqp.AccessRefused, Reason: "ACCESS_REFUSED"}
	tests := []struct {
		name      string
		setup     func(b *fakeBroker)
		want      errorkit.ErrorCode
		retriable bool
	}{
		{"dial refused", func(b *fakeBroker) { b.dialErr = errors.New("connection refused") }, ERR_MQ_CONNECTION_FAILED, true},
		{"bad credentials", func(b *fakeBroker) { b.dialErr = amqp.ErrCredentials }, ERR_MQ_AUTH_FAILED, false},
		{"vhost denied", func(b *fakeBroker) { b.dialErr = amqp.ErrVhost }, ERR_MQ_AUTH_FAILED, false},
		{"channel open", func(b *fakeBroker) { b.channelErr = amqp.ErrClosed }, ERR_MQ_CHANNEL_ERROR, true},
		{"exchange declare", func(b *fakeBroker) { b.exchangeErr = errors.New("precondition failed") }, ERR_MQ_EXCHANGE_ERROR, true},
		{"exchange declare refused", func(b *fakeBroker) { b.exchangeErr = refused }, ERR_MQ_AUTH_FAILED, false},
		{"queue declare", func(b *fakeBroker) { b.queueErr = errors.New("precondition failed") }, ERR_MQ_QUEUE_ERROR, true},
		{"queue bind", func(b *fakeBroker) { b.bindErr = errors.New("not found") }, ERR_MQ_QUEUE_ERROR, true},
		{"qos", func(b *fakeBroker) { b.qosErr = errors.New("qos") }, ERR_MQ_CHANNEL_ERROR, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := newFakeBroker()
			tt.setup(b)

			appErr := requireAppError(t, newFakeSubscriber(b).connect())
			assert.Equal(t, tt.want, appErr.Code())
			assert.Equal(t, tt.retriable, appErr.Metadata.Retriable)
		})
	}
}

func TestSubscriberConsumeErrorCode(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	s := newFakeSubscriber(b)
	require.NoError(t, s.connect())
	b.consumeErr = errors.New("consumer tag in use")

	_, err := s.Consume()
	assert.Equal(t, ERR_MQ_CONSUME_FAILED, requireAppError(t, err).Code())
}

func TestPublisherConnectErrorCodes(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	b.dialErr = amqp.ErrCredentials
	_, err := newPublisherForTest(b)
	appErr := requireAppError(t, err)
	assert.Equal(t, ERR_MQ_AUTH_FAILED, appErr.Code())
	assert.False(t, appErr.Metadata.Retriable)

	b = newFakeBroker()
	b.channelErr = amqp.ErrClosed
	_, err = newPublisherForTest(b)
	assert.Equal(t, ERR_MQ_CHANNEL_ERROR, requireAppError(t, err).Code())
}

func newPublisherForTest(b *fakeBroker) (*Publisher, error) {
	p := &Publisher{amqpURL: "amqp://fake", exchanges: []Exchange{{Name: "events", Type: "topic"}}, dial: b.dial}
	if err := p.connect(); err != nil {
		return nil, err
	}
	return p, nil
}

// Topology lookups are configuration errors, not internal errors.
func TestTopologyLookupErrorCodes(t *testing.T) {
	t.Parallel()

	p := &PubSub{topology: &Topology{
		Queues:   []QueueTopology{{Name: "q"}},
		Bindings: []Binding{{Queue: "q", Exchange: "missing", RoutingKeys: []string{"a"}}},
	}}
	handler := func(context.Context, Message) error { return nil }

	err := p.Publish(context.Background(), "unknown", Event{})
	assert.Equal(t, errorkit.ERR_SYSTEM_CONFIG_INVALID, requireAppError(t, err).Code())

	err = p.Subscribe(context.Background(), "unknown", "svc", handler)
	assert.Equal(t, errorkit.ERR_SYSTEM_CONFIG_INVALID, requireAppError(t, err).Code())

	err = p.Subscribe(context.Background(), "q", "svc", handler)
	assert.Equal(t, errorkit.ERR_SYSTEM_CONFIG_INVALID, requireAppError(t, err).Code())
}

// A non-retriable reconnect failure (bad credentials) stops the reconnect
// loop instead of retrying until the ctx ends.
func TestReconnectWithBackoffStopsOnNonRetriableError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newFakeBroker()
		b.dialErr = amqp.ErrCredentials
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()

		err := reconnectWithBackoff(ctx, newFakeSubscriber(b), nil)

		assert.Equal(t, ERR_MQ_AUTH_FAILED, requireAppError(t, err).Code())
		assert.Equal(t, 1, b.dialCount())
		assert.NoError(t, ctx.Err(), "the loop must stop before the ctx deadline")
	})
}
