package httprequest

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
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
