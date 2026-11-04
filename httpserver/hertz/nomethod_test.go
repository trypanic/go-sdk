package hertz

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/v2/httpserver"
)

func TestNoRouteAndNoMethodDefaults(t *testing.T) {
	t.Parallel()

	s := newTestServer(t, plainOptions())
	s.GET("/only-get", func(_ context.Context, c httpserver.Context) { c.Status(http.StatusOK) })

	require.Equal(t, http.StatusNotFound, perform(t.Context(), s, http.MethodGet, "/missing", nil).StatusCode())
	require.Equal(t, http.StatusMethodNotAllowed, perform(t.Context(), s, http.MethodPost, "/only-get", nil).StatusCode())
}

func TestNoRouteAndNoMethodCustomHandlers(t *testing.T) {
	t.Parallel()

	o := plainOptions()
	o.NoRouteHandler = func(_ context.Context, c httpserver.Context) {
		c.Reply(http.StatusNotFound, c.WithMessage("no such route"))
	}
	o.NoMethodHandler = func(_ context.Context, c httpserver.Context) {
		c.Reply(http.StatusMethodNotAllowed, c.WithMessage("method not allowed here"))
	}
	s := newTestServer(t, o)
	s.GET("/only-get", func(_ context.Context, c httpserver.Context) { c.Status(http.StatusOK) })

	res := perform(t.Context(), s, http.MethodGet, "/missing", nil)
	require.Equal(t, http.StatusNotFound, res.StatusCode())
	require.Contains(t, string(res.Body()), "no such route")

	res = perform(t.Context(), s, http.MethodPost, "/only-get", nil)
	require.Equal(t, http.StatusMethodNotAllowed, res.StatusCode())
	require.Contains(t, string(res.Body()), "method not allowed here")
}
