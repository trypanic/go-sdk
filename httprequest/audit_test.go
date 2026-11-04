package httprequest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/v2/logger"
)

func TestSanitizeHTTPHeadersMasksCredentialHeaders(t *testing.T) {
	t.Parallel()

	h := http.Header{}
	sensitive := []string{
		"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie",
		"X-Api-Key", "X-Goog-Api-Key", "X-Amz-Security-Token", "X-Csrf-Token",
		"X-Vendor-Secret", "X-Session-Token",
	}
	for _, name := range sensitive {
		h.Set(name, "credential-value")
	}
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "*/*")

	got := sanitizeHTTPHeaders(h)

	for _, name := range sensitive {
		require.Contains(t, got, name, "sensitive header key is kept (masked), not dropped")
		require.Equal(t, "[REDACTED]", got[name], "header %s value must be masked", name)
	}
	require.Equal(t, "application/json", got["Content-Type"])
	require.Equal(t, "*/*", got["Accept"])
}

func TestOutboundAuditSanitizesURLHeaders(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, raw, want string }{
		{"credentials query fragment", "https://user-secret:password-secret@example.test/callback?code=query-secret#fragment-secret", "https://example.test/callback?code=REDACTED"},
		{"relative", "/callback?code=query-secret#fragment-secret", "/callback?code=REDACTED"},
		{"malformed", "https://[bad-host?code=query-secret", "[invalid URL]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Add("Location", "https://example.test/help")
				w.Header().Add("Location", tt.raw)
				w.Header().Set("X-Trace-Note", "visible")
				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()
			var logs bytes.Buffer
			zl := zerolog.New(&logs)
			ctx := logger.WithLogger(t.Context(), &zl)
			requester := NewWithOptions(srv.Client(), WithAuditLog(), WithRetryConfig(RetryConfig{MaxRetries: 1}))
			require.NoError(t, requester.Do(ctx, RequestConfig{Method: http.MethodGet, URL: srv.URL, Headers: map[string]string{"rEfErEr": tt.raw, "X-Trace-Note": "visible"}}, nil))
			var line map[string]any
			require.NoError(t, json.Unmarshal(logs.Bytes(), &line))
			require.Equal(t, "http.audit.outbound", line["event"])
			req := line["request.headers"].(map[string]any)
			resp := line["response.headers"].(map[string]any)
			require.Equal(t, tt.want, req["Referer"])
			require.Equal(t, "https://example.test/help, "+tt.want, resp["Location"])
			require.Equal(t, "visible", req["X-Trace-Note"])
			require.Equal(t, "visible", resp["X-Trace-Note"])
			require.NotContains(t, logs.String(), "-secret")
		})
	}
}
