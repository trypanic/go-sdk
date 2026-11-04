package hertz

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpserver"
)

func plainOptions() httpserver.ServerOptions {
	o := testOptions()
	o.EnableAudit = false
	return o
}

func TestRoutingMethodsAndRequestAccessors(t *testing.T) {
	t.Parallel()

	s := newTestServer(t, plainOptions())
	echo := func(ctx context.Context, c httpserver.Context) {
		c.JSON(http.StatusOK, map[string]string{
			"id":     c.Param("id"),
			"q":      c.Query("q"),
			"header": c.GetHeader("X-Note"),
			"body":   string(c.GetBody()),
		})
	}
	s.GET("/items/:id", echo)
	s.POST("/items/:id", echo)
	s.PUT("/items/:id", echo)
	s.DELETE("/items/:id", echo)

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		res := perform(t.Context(), s, method, "/items/42?q=find", []byte("payload"), header{"X-Note", "hi"})
		require.Equal(t, http.StatusOK, res.StatusCode(), method)
		var got map[string]string
		require.NoError(t, json.Unmarshal(res.Body(), &got))
		require.Equal(t, map[string]string{"id": "42", "q": "find", "header": "hi", "body": "payload"}, got, method)
	}
}

func TestGroupsAndMiddlewareOrder(t *testing.T) {
	t.Parallel()

	s := newTestServer(t, plainOptions())
	var order []string
	mark := func(name string) httpserver.MiddlewareFunc {
		return func(ctx context.Context, c httpserver.Context, next httpserver.HandlerFunc) {
			order = append(order, name+">")
			next(ctx, c)
			order = append(order, "<"+name)
		}
	}
	s.Use(mark("global"))
	api := s.Group("/api")
	api.Use(mark("api"))
	v1 := api.Group("/v1")
	v1.Use(mark("v1"))
	handler := func(_ context.Context, c httpserver.Context) {
		order = append(order, "handler")
		c.Status(http.StatusNoContent)
	}
	v1.GET("/r", handler)
	v1.POST("/r", handler)
	v1.PUT("/r", handler)
	v1.DELETE("/r", handler)

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		order = nil
		res := perform(t.Context(), s, method, "/api/v1/r", nil)
		require.Equal(t, http.StatusNoContent, res.StatusCode(), method)
		require.Equal(t, []string{"global>", "api>", "v1>", "handler", "<v1", "<api", "<global"}, order, method)
	}
}

func TestReplyEnvelopeJSON(t *testing.T) {
	t.Parallel()

	o := plainOptions()
	s := newTestServer(t, o)
	s.GET("/reply", func(_ context.Context, c httpserver.Context) {
		c.Reply(http.StatusOK, c.WithMessage("done"), c.WithData(map[string]int{"n": 1}), c.WithMetadata(map[string]string{"page": "1"}))
	})

	res := perform(t.Context(), s, http.MethodGet, "/reply", nil)
	require.Equal(t, http.StatusOK, res.StatusCode())
	require.JSONEq(t, `{"message":"done","data":{"n":1},"metadata":{"page":"1"}}`, stripTimestamp(t, res.Body()))
}

func TestBindJSONReturnsBadRequestAppError(t *testing.T) {
	t.Parallel()

	s := newTestServer(t, plainOptions())
	var bindErr error
	var got struct{ Name string }
	s.POST("/bind", func(_ context.Context, c httpserver.Context) {
		bindErr = c.BindJSON(&got)
		c.Status(http.StatusOK)
	})

	perform(t.Context(), s, http.MethodPost, "/bind", []byte(`{"Name":"ok"}`), header{"Content-Type", "application/json"})
	require.NoError(t, bindErr)
	require.Equal(t, "ok", got.Name)

	perform(t.Context(), s, http.MethodPost, "/bind", []byte(`{not json`), header{"Content-Type", "application/json"})
	appErr, ok := bindErr.(*errorkit.AppError)
	require.True(t, ok, "want *errorkit.AppError, got %T", bindErr)
	require.Equal(t, errorkit.ERR_CLIENT_BAD_REQUEST, appErr.Code())
}

func TestRedirectAndHeaders(t *testing.T) {
	t.Parallel()

	s := newTestServer(t, plainOptions())
	s.GET("/old", func(_ context.Context, c httpserver.Context) {
		c.SetHeader("X-Moved", "yes")
		c.Redirect(http.StatusMovedPermanently, []byte("/new"))
	})

	res := perform(t.Context(), s, http.MethodGet, "/old", nil)
	require.Equal(t, http.StatusMovedPermanently, res.StatusCode())
	require.True(t, strings.HasSuffix(string(res.Header.Peek("Location")), "/new"))
	require.Equal(t, "yes", string(res.Header.Peek("X-Moved")))
}

func TestNewUsesDefaultOptions(t *testing.T) {
	t.Parallel()

	srv, ok := any(New(httpserver.ServerConfig{Host: "127.0.0.1", Port: 0})).(*Server)
	require.True(t, ok)
	require.Equal(t, httpserver.DateTimeLayout, srv.replyOpts.Layout)
	res := perform(t.Context(), srv, http.MethodGet, "/health", nil)
	require.Equal(t, http.StatusOK, res.StatusCode())
}
