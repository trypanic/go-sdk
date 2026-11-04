package postgres

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConstructorCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	pool, err := NewPostgresClientContext(ctx, Config{DSN: "postgres://user@127.0.0.1:1/db"})
	require.Nil(t, pool)
	require.ErrorIs(t, err, context.Canceled)
}

func TestConstructorCancelsStalledHandshake(t *testing.T) {
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
		_, e := NewPostgresClientContext(ctx, Config{DSN: "postgres://user@" + listener.Addr().String() + "/db?sslmode=disable"})
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
