package hertz

import (
	"context"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/httpserver"
)

func TestExtendedRoutingMethods(t *testing.T) {
	for _, method := range []string{"PATCH", "HEAD", "OPTIONS", "TRACE"} {
		t.Run(method, func(t *testing.T) {
			srv := NewWithOptions(httpserver.ServerConfig{}, httpserver.ServerOptions{})
			handler := func(_ context.Context, c httpserver.Context) { c.SetHeader("X-Matched", "yes"); c.Status(204) }
			group := srv.Group("/v1")
			switch method {
			case "PATCH":
				srv.PATCH("/resource", handler)
				group.PATCH("/resource", handler)
			case "HEAD":
				srv.HEAD("/resource", handler)
				group.HEAD("/resource", handler)
			case "OPTIONS":
				srv.OPTIONS("/resource", handler)
				group.OPTIONS("/resource", handler)
			default:
				srv.Handle(method, "/resource", handler)
				group.Handle(method, "/resource", handler)
			}
			engine := any(srv).(*Server).engine
			for _, path := range []string{"/resource", "/v1/resource"} {
				response := ut.PerformRequest(engine.Engine, method, path, nil).Result()
				require.Equal(t, 204, response.StatusCode())
				require.Equal(t, "yes", string(response.Header.Peek("X-Matched")))
			}
		})
	}
}
