package hertz_test

import (
	"context"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/httpserver"
	"github.com/trypanic/go-sdk/httpserver/hertz"
)

var _ func(httpserver.ServerConfig) *hertz.Server = hertz.NewEngine

func TestEngineEscapeHatch(t *testing.T) {
	srv := hertz.NewEngine(httpserver.ServerConfig{})
	srv.Engine().GET("/raw", func(_ context.Context, c *app.RequestContext) { c.String(200, "raw") })
	response := ut.PerformRequest(srv.Engine().Engine, "GET", "/raw", nil).Result()
	require.Equal(t, "raw", string(response.Body()))
}
