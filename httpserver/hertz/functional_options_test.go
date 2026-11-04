package hertz_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/httpserver"
	"github.com/trypanic/go-sdk/httpserver/hertz"
	"github.com/trypanic/go-sdk/logger"
)

func TestNewServerDefaultsAndExplicitLogger(t *testing.T) {
	var logs bytes.Buffer
	base := logger.New(logger.Config{Env: logger.Prod, Writer: &logs})
	srv := hertz.NewServer(httpserver.ServerConfig{}, hertz.WithLogger(base))
	srv.GET("/panic", func(context.Context, httpserver.HTTPContext) { panic("private failure") })
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/health", 200}, {"GET", "/missing", 404}, {"POST", "/health", 405}, {"GET", "/panic", 500},
	} {
		response := ut.PerformRequest(srv.Engine().Engine, tc.method, tc.path, nil).Result()
		require.Equal(t, tc.status, response.StatusCode())
	}
	require.Contains(t, logs.String(), "http.audit.inbound")
	require.Contains(t, logs.String(), "panic recovered")
}

func TestNewServerCanDisableHealth(t *testing.T) {
	srv := hertz.NewServer(httpserver.ServerConfig{}, hertz.WithoutTracing(), hertz.WithoutAudit(), hertz.WithoutHealth())
	response := ut.PerformRequest(srv.Engine().Engine, "GET", "/health", nil).Result()
	require.Equal(t, 404, response.StatusCode())
}
