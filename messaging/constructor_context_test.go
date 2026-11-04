package messaging

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/stretchr/testify/require"
)

func TestPubSubConstructorCancelsStalledHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, e := listener.Accept()
		if e == nil {
			accepted <- conn
		}
	}()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, e := NewPubSubContext(ctx, "amqp://guest:guest@"+listener.Addr().String()+"/", WithTopology(testTopology()))
		result <- e
	}()
	select {
	case conn := <-accepted:
		t.Cleanup(func() { _ = conn.Close() })
	case <-time.After(time.Second):
		t.Fatal("constructor did not dial")
	}
	cancel()
	select {
	case err = <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt handshake")
	}
}

func TestConstructionContextDoesNotOwnPubSubLifetime(t *testing.T) {
	broker := newFakeBroker()
	ctx, cancel := context.WithCancel(t.Context())
	pubsub, err := newPubSubContext(ctx, "amqp://fake", broker.dial, WithTopology(testTopology()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pubsub.Close()) })
	cancel()
	require.NoError(t, pubsub.Publish(t.Context(), "q", Event{}))
}

type stalledSetupConn struct {
	amqpConnection
	opened chan struct{}
	closed chan struct{}
	once   sync.Once
}

func (c *stalledSetupConn) Channel() (amqpChannel, error) {
	close(c.opened)
	<-c.closed
	return nil, errors.New("connection closed during setup")
}
func (c *stalledSetupConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func TestPubSubCancellationInterruptsChannelSetup(t *testing.T) {
	conn := &stalledSetupConn{opened: make(chan struct{}), closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := newPubSubContext(ctx, "amqp://fake", func(string, amqp.Config) (amqpConnection, error) { return conn, nil }, WithTopology(testTopology()))
		done <- err
	}()
	<-conn.opened
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("cancellation did not interrupt channel setup")
	}
}
