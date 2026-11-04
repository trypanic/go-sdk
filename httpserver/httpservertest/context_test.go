package httpservertest_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpserver"
	"github.com/trypanic/go-sdk/httpserver/httpservertest"
)

func TestHandlerWithRecorder(t *testing.T) {
	c := httpservertest.NewContext(httptest.NewRequest("POST", "/users?sort=name", strings.NewReader(`{"name":"Ada"}`)))
	c.Params["id"] = "123"
	handler := httpserver.HandlerFunc(func(_ context.Context, c httpserver.HTTPContext) {
		var body struct{ Name string }
		require.NoError(t, c.BindJSON(&body))
		require.Equal(t, "Ada", body.Name)
		require.Equal(t, "123", c.Param("id"))
		require.Equal(t, "name", c.Query("sort"))
		c.SetHeader("X-Test", "yes")
		httpserver.ReplyError(c, errorkit.NewError(errorkit.ERR_CLIENT_NOT_FOUND))
	})
	handler(t.Context(), c)
	require.NoError(t, c.Err)
	require.Equal(t, 404, c.Response.Code)
	require.Equal(t, "yes", c.Response.Header().Get("X-Test"))
	require.Contains(t, c.Response.Body.String(), "ERR_CLIENT_NOT_FOUND")
}

func TestRecorderRetainsEncodingFailure(t *testing.T) {
	c := httpservertest.NewContext(nil)
	c.JSON(200, func() {})
	require.Error(t, c.Err)
	require.Equal(t, 500, c.Response.Code)
	require.Empty(t, c.Response.Body.String())
}
