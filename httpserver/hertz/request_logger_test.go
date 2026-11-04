package hertz

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/httpserver"
	"github.com/trypanic/go-sdk/logger"
)

var generatedID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// requestLoggerServer installs the middleware through the SDK interface
// (useSDK) or directly on the engine, plus a handler that logs one line.
func requestLoggerServer(t *testing.T, useSDK bool) *wrapperHertzServer {
	t.Helper()
	o := testOptions()
	o.EnableAudit = false
	s := newTestServer(t, o)
	if useSDK {
		s.Use(RequestLogger(nil))
	} else {
		s.engine.Use(RequestLoggerMiddleware(nil))
	}
	s.GET("/work", func(ctx context.Context, c httpserver.HTTPContext) {
		logger.CtxOrGlobal(ctx).Info().Msg("working")
		c.String(http.StatusOK, "ok")
	})
	return s
}

func loggedRequestID(t *testing.T, logs string) string {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m), line)
		if m["message"] == "working" {
			id, _ := m["request_id"].(string)
			return id
		}
	}
	t.Fatalf("handler log line not found in %q", logs)
	return ""
}

func TestRequestLoggerHonorsAndEchoesRequestID(t *testing.T) {
	t.Parallel()

	for name, useSDK := range map[string]bool{"SDK middleware": true, "Hertz middleware": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tests := []struct {
				name    string
				inbound string
				keep    bool
			}{
				{name: "inbound id is kept", inbound: "req-42.abc_DEF:1", keep: true},
				{name: "missing id is generated"},
				{name: "id with spaces is replaced", inbound: "bad id"},
				{name: "overlong id is replaced", inbound: strings.Repeat("a", 129)},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					s := requestLoggerServer(t, useSDK)
					ctx, logs := logContext(t)
					var headers []header
					if tt.inbound != "" {
						headers = append(headers, header{RequestIDHeader, tt.inbound})
					}

					res := perform(ctx, s, http.MethodGet, "/work", nil, headers...)

					require.Equal(t, http.StatusOK, res.StatusCode())
					echoed := responseHeader(res, RequestIDHeader)
					require.Equal(t, echoed, loggedRequestID(t, logs.String()), "logged and echoed ids match")
					if tt.keep {
						require.Equal(t, tt.inbound, echoed)
					} else {
						require.Regexp(t, generatedID, echoed)
					}
				})
			}
		})
	}
}

func responseHeader(res *protocol.Response, key string) string {
	return string(res.Header.Peek(key))
}
