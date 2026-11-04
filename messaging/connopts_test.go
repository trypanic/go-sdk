package messaging

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// C5-014: connection options reach the AMQP config of the publisher and of
// every subscriber.
func TestConnectionOptionsApplied(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	tlsCfg := &tls.Config{ServerName: "broker.internal", MinVersion: tls.VersionTLS13}
	m, err := newPubSub("amqp://fake", b.dial,
		WithTopology(testTopology()),
		WithTLSConfig(tlsCfg),
		WithHeartbeat(10*time.Second),
		WithConnectionName("order-service"),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- m.Subscribe(ctx, "q", "svc", func(context.Context, Message) error { return nil })
	}()
	require.Eventually(t, func() bool { return b.dialCount() == 2 }, time.Second, time.Millisecond)
	cancel()
	require.NoError(t, <-done)
	require.NoError(t, m.Close())

	b.mu.Lock()
	defer b.mu.Unlock()
	require.Len(t, b.configs, 2, "publisher and subscriber")
	for _, cfg := range b.configs {
		assert.Same(t, tlsCfg, cfg.TLSClientConfig)
		assert.Equal(t, 10*time.Second, cfg.Heartbeat)
		assert.Equal(t, "order-service", cfg.Properties["connection_name"])
		assert.NotNil(t, cfg.Properties["product"], "default client properties are kept")
	}
}

// Without options the package defaults apply.
func TestConnectionOptionsDefaults(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	m, err := newPubSub("amqp://fake", b.dial, WithTopology(testTopology()),
		WithHeartbeat(0), WithDialTimeout(-time.Second)) // non-positive keeps the default
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })

	b.mu.Lock()
	defer b.mu.Unlock()
	require.Len(t, b.configs, 1)
	assert.Equal(t, DefaultHeartbeat, b.configs[0].Heartbeat)
	assert.Nil(t, b.configs[0].TLSClientConfig)
	assert.Nil(t, b.configs[0].Properties)
}

// C5-014: WithDialTimeout bounds the connect and handshake of NewPubSub
// against a peer that accepts TCP but never answers.
func TestWithDialTimeoutBoundsHandshake(t *testing.T) {
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

	begin := time.Now()
	_, err = NewPubSub("amqp://guest:guest@"+ln.Addr().String()+"/",
		WithTopology(testTopology()), WithDialTimeout(100*time.Millisecond))

	assert.Equal(t, ERR_MQ_CONNECTION_FAILED, requireAppError(t, err).Code())
	assert.Less(t, time.Since(begin), 2*time.Second)
}
