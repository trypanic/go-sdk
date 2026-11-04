package messaging

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/errorkit"
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
