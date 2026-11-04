package hertz

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/v2/httpserver"
)

func healthOptions() httpserver.ServerOptions {
	o := testOptions()
	o.EnableAudit = false
	return o
}

func TestHealthDefaultPath(t *testing.T) {
	t.Parallel()

	s := newTestServer(t, healthOptions())
	res := perform(t.Context(), s, http.MethodGet, "/health", nil)
	require.Equal(t, http.StatusOK, res.StatusCode())
	require.JSONEq(t, `{"status":"healthy"}`, string(res.Body()))
}

func TestHealthCustomPathFreesHealthForTheConsumer(t *testing.T) {
	t.Parallel()

	o := healthOptions()
	o.HealthPath = "/livez"
	s := newTestServer(t, o)
	require.NotPanics(t, func() {
		s.GET("/health", func(_ context.Context, c httpserver.Context) {
			c.String(http.StatusOK, "mine")
		})
	})

	res := perform(t.Context(), s, http.MethodGet, "/livez", nil)
	require.Equal(t, http.StatusOK, res.StatusCode())
	res = perform(t.Context(), s, http.MethodGet, "/health", nil)
	require.Equal(t, "mine", string(res.Body()))
}

func TestHealthDisabledLeavesThePathFree(t *testing.T) {
	t.Parallel()

	o := healthOptions()
	o.EnableHealth = false
	s := newTestServer(t, o)
	require.NotPanics(t, func() {
		s.GET("/health", func(_ context.Context, c httpserver.Context) { c.String(http.StatusOK, "mine") })
	})
}

func TestHealthCheckHookReportsReadiness(t *testing.T) {
	t.Parallel()

	ready := true
	o := healthOptions()
	o.HealthCheck = func(context.Context) error {
		if ready {
			return nil
		}
		return errors.New("db pool exhausted at 10.0.0.7")
	}
	s := newTestServer(t, o)

	res := perform(t.Context(), s, http.MethodGet, "/health", nil)
	require.Equal(t, http.StatusOK, res.StatusCode())

	ready = false
	res = perform(t.Context(), s, http.MethodGet, "/health", nil)
	require.Equal(t, http.StatusServiceUnavailable, res.StatusCode())
	var body map[string]any
	require.NoError(t, json.Unmarshal(res.Body(), &body))
	require.Equal(t, "unhealthy", body["status"])
	require.NotContains(t, string(res.Body()), "10.0.0.7", "the check error is not sent to the client")
}
