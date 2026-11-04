package hertz

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpserver"
)

func loopbackConfig(t *testing.T, port int) httpserver.ServerConfig {
	t.Helper()
	return httpserver.ServerConfig{Host: "127.0.0.1", Port: port}
}

// freePort returns a loopback port that was free a moment ago.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	return port
}

func TestRunReturnsBindErrorInsteadOfPanicking(t *testing.T) {
	t.Parallel()

	busy, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = busy.Close() })
	port := busy.Addr().(*net.TCPAddr).Port

	srv := NewWithOptions(loopbackConfig(t, port), httpserver.ServerOptions{})

	done := make(chan error, 1)
	go func() { done <- srv.Run() }()

	select {
	case err := <-done:
		require.Error(t, err)
		appErr, ok := err.(*errorkit.AppError)
		require.True(t, ok, "want *errorkit.AppError, got %T (%v)", err, err)
		require.Equal(t, errorkit.ERR_SYSTEM_CONFIG_INVALID, appErr.Code())
		require.Contains(t, appErr.Reason, strconv.Itoa(port))
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return although the port is taken")
	}
}

func TestRunServesUntilShutdownAndReturnsNil(t *testing.T) {
	t.Parallel()

	port := freePort(t)
	srv := NewWithOptions(loopbackConfig(t, port), httpserver.ServerOptions{EnableHealth: true})

	done := make(chan error, 1)
	go func() { done <- srv.Run() }()

	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	require.Eventually(t, func() bool {
		res, err := http.Get(url) //nolint:noctx // test probe
		if err != nil {
			return false
		}
		_ = res.Body.Close()
		return res.StatusCode == http.StatusOK
	}, 5*time.Second, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, srv.Shutdown(ctx))

	select {
	case err := <-done:
		require.NoError(t, err, "Run returns nil after a graceful Shutdown")
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}
}
