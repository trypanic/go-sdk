package hertz

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/httpserver"
)

// auditLines returns the decoded inbound audit entries found in logs.
func auditLines(t *testing.T, logs string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m), line)
		if m["event"] == "http.audit.inbound" {
			out = append(out, m)
		}
	}
	return out
}

func echoServer(t *testing.T, opts httpserver.ServerOptions) *Server {
	t.Helper()
	s := newTestServer(t, opts)
	s.POST("/echo", func(ctx context.Context, c httpserver.Context) {
		c.SetHeader("Set-Cookie", "session=resp-cookie")
		c.String(http.StatusOK, "response-secret")
	})
	return s
}

func TestInboundAuditLogsRedactedLineByDefault(t *testing.T) {
	t.Parallel()

	s := echoServer(t, testOptions())
	ctx, logs := logContext(t)

	res := perform(ctx, s, http.MethodPost, "/echo?token=query-secret", []byte("request-secret"),
		header{"Authorization", "Bearer header-secret"},
		header{"X-Trace-Note", "visible"},
	)
	require.Equal(t, http.StatusOK, res.StatusCode())

	lines := auditLines(t, logs.String())
	require.Len(t, lines, 1, "one audit line per request")
	line := lines[0]
	require.Equal(t, "POST", line["method"])
	require.Equal(t, "/echo", line["path"], "the query string is not logged")
	require.EqualValues(t, http.StatusOK, line["status_code"])
	require.Equal(t, "[REDACTED]", line["request.body"])
	require.Equal(t, "[REDACTED]", line["response.body"])
	require.Contains(t, line, "duration_ms")

	reqHeaders := line["request.headers"].(map[string]any)
	require.Equal(t, "[REDACTED]", reqHeaders["Authorization"])
	require.Equal(t, "visible", reqHeaders["X-Trace-Note"])
	respHeaders := line["response.headers"].(map[string]any)
	require.Equal(t, "[REDACTED]", respHeaders["Set-Cookie"])

	for _, secret := range []string{"request-secret", "response-secret", "header-secret", "query-secret", "resp-cookie"} {
		require.NotContains(t, logs.String(), secret)
	}
}

func TestInboundAuditBodyCapturePolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		capture  bool
		redactor httpserver.BodyRedactor
		wantReq  string
		wantResp string
	}{
		{"capture off ignores raw redactor", false, httpserver.RawBodyRedactor, "[REDACTED]", "[REDACTED]"},
		{"capture on with default redactor", true, httpserver.DefaultBodyRedactor, "[REDACTED]", "[REDACTED]"},
		{"capture on with raw redactor", true, httpserver.RawBodyRedactor, "request-secret", "response-secret"},
		{"capture on with custom redactor", true, func(b []byte) []byte { return []byte("len=" + string(rune('0'+len(b)%10))) }, "len=4", "len=5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := testOptions()
			opts.Audit = httpserver.AuditOptions{CaptureBodies: tt.capture, BodyRedactor: tt.redactor}
			s := echoServer(t, opts)
			ctx, logs := logContext(t)

			perform(ctx, s, http.MethodPost, "/echo", []byte("request-secret"))

			lines := auditLines(t, logs.String())
			require.Len(t, lines, 1)
			require.Equal(t, tt.wantReq, lines[0]["request.body"])
			require.Equal(t, tt.wantResp, lines[0]["response.body"])
		})
	}
}

func TestInboundAuditDisabled(t *testing.T) {
	t.Parallel()

	opts := testOptions()
	opts.EnableAudit = false
	s := echoServer(t, opts)
	ctx, logs := logContext(t)

	perform(ctx, s, http.MethodPost, "/echo", []byte("x"))

	require.Empty(t, auditLines(t, logs.String()))
}

func TestInboundAuditSanitizesURLHeadersByDefault(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, raw, want string }{
		{"credentials query fragment", "https://user-secret:password-secret@example.test/callback?code=query-secret#fragment-secret", "https://example.test/callback?code=REDACTED"},
		{"relative", "/callback?code=query-secret#fragment-secret", "/callback?code=REDACTED"},
		{"malformed", "https://[bad-host?code=query-secret", "[invalid URL]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newTestServer(t, httpserver.DefaultServerOptions())
			s.GET("/login", func(ctx context.Context, c httpserver.Context) {
				c.SetHeader("lOcAtIoN", tt.raw)
				c.SetHeader("X-Trace-Note", "visible")
				c.String(http.StatusFound, "")
			})
			ctx, logs := logContext(t)
			res := perform(ctx, s, http.MethodGet, "/login", nil, header{"rEfErEr", tt.raw}, header{"X-Trace-Note", "visible"})
			require.Equal(t, http.StatusFound, res.StatusCode())
			require.Equal(t, tt.raw, string(res.Header.Peek("Location")), "redaction must not change the response")
			lines := auditLines(t, logs.String())
			require.Len(t, lines, 1)
			req := lines[0]["request.headers"].(map[string]any)
			resp := lines[0]["response.headers"].(map[string]any)
			require.Equal(t, tt.want, req["Referer"])
			require.Equal(t, tt.want, resp["Location"])
			require.Equal(t, "visible", req["X-Trace-Note"])
			require.Equal(t, "visible", resp["X-Trace-Note"])
			require.NotContains(t, logs.String(), "-secret")
		})
	}
}
