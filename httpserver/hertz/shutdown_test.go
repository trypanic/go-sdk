package hertz

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpserver"
)

// startServer runs srv on a free loopback port with a /slow route that
// waits for release (or d when release is nil) and returns its base URL.
func startServer(t *testing.T, d time.Duration, release <-chan struct{}) (httpserver.HTTPServer, string, chan error) {
	t.Helper()
	port := freePort(t)
	srv := NewWithOptions(loopbackConfig(t, port), httpserver.ServerOptions{EnableHealth: true})
	srv.GET("/slow", func(ctx context.Context, c httpserver.HTTPContext) {
		if release != nil {
			<-release
		} else {
			time.Sleep(d)
		}
		c.JSON(http.StatusOK, map[string]string{"ok": "yes"})
	})
	done := make(chan error, 1)
	go func() { done <- srv.Run() }()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	require.Eventually(t, func() bool {
		res, err := http.Get(base + "/health") //nolint:noctx // test probe
		if err != nil {
			return false
		}
		_ = res.Body.Close()
		return true
	}, 5*time.Second, 10*time.Millisecond)
	return srv, base, done
}

// inflight starts a GET to url and reports its status code.
func inflight(url string) <-chan int {
	status := make(chan int, 1)
	go func() {
		c := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
		res, err := c.Get(url) //nolint:noctx // test request
		if err != nil {
			status <- 0
			return
		}
		_ = res.Body.Close()
		status <- res.StatusCode
	}()
	return status
}

func TestShutdownIsNotCappedByHertzExitWaitTime(t *testing.T) {
	t.Parallel()

	srv := NewWithOptions(loopbackConfig(t, 0), httpserver.ServerOptions{})
	w := srv.(*wrapperHertzServer)
	require.GreaterOrEqual(t, w.engine.GetOptions().ExitWaitTimeout, time.Hour,
		"the caller's context, not Hertz's 5 s default, must bound Shutdown")
}

func TestShutdownWaitsForInFlightRequests(t *testing.T) {
	t.Parallel()

	srv, base, done := startServer(t, 300*time.Millisecond, nil)
	status := inflight(base + "/slow")
	time.Sleep(100 * time.Millisecond) // let the request reach the handler

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	require.NoError(t, srv.Shutdown(ctx))
	require.Equal(t, http.StatusOK, <-status, "the in-flight request completes before Shutdown returns")
	require.NoError(t, <-done)
}

func TestShutdownReportsTimeoutWhileRequestsAreInFlight(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	srv, base, _ := startServer(t, 0, release)
	t.Cleanup(func() { close(release) })
	_ = inflight(base + "/slow")
	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	err := srv.Shutdown(ctx)

	appErr, ok := err.(*errorkit.AppError)
	require.True(t, ok, "want *errorkit.AppError, got %T (%v)", err, err)
	require.Equal(t, errorkit.ERR_SYSTEM_TIMEOUT_INTERNAL, appErr.Code())
}

func TestShutdownDoesNotWaitForIdleKeepAliveConnections(t *testing.T) {
	t.Parallel()

	srv, base, done := startServer(t, 0, nil)
	// http.Get above left an idle keep-alive connection open.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	start := time.Now()
	require.NoError(t, srv.Shutdown(ctx))
	require.Less(t, time.Since(start), 2*time.Second, "an idle connection is not an in-flight request")
	require.NoError(t, <-done)
	_ = base
}

func TestShutdownDrainsRequestLongerThanFiveSeconds(t *testing.T) {
	if testing.Short() {
		t.Skip("takes 6 s; TestShutdownIsNotCappedByHertzExitWaitTime covers the cap in -short runs")
	}
	t.Parallel()

	srv, base, _ := startServer(t, 6*time.Second, nil)
	status := inflight(base + "/slow")
	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	start := time.Now()
	require.NoError(t, srv.Shutdown(ctx))
	require.GreaterOrEqual(t, time.Since(start), 5*time.Second)
	require.Equal(t, http.StatusOK, <-status)
}
