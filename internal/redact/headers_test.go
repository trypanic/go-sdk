package redact

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsSensitiveHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{"Authorization", true},
		{"authorization", true},
		{"Proxy-Authorization", true},
		{"Cookie", true},
		{"Set-Cookie", true},
		{"X-Api-Key", true},
		{"Api-Key", true},
		{"X-Auth-Token", true},
		{"X-Access-Token", true},
		{"X-Goog-Api-Key", true},
		{"X-Amz-Security-Token", true},
		{"X-Csrf-Token", true},
		{"X-Xsrf-Token", true},
		{"X-Vendor-Secret", true},
		{"X-Client-Password", true},
		{"X-Session-Id", true},
		{"X-Hub-Signature-256", true},
		{"Content-Type", false},
		{"Content-Length", false},
		{"Accept", false},
		{"User-Agent", false},
		{"Keep-Alive", false},
		{"X-Request-Id", false},
		{"Traceparent", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, IsSensitiveHeader(tt.name))
		})
	}
}

func TestHeadersMasksValuesAndKeepsKeys(t *testing.T) {
	t.Parallel()

	h := http.Header{}
	h.Add("Authorization", "Bearer secret")
	h.Add("Accept", "text/plain")
	h.Add("Accept", "application/json")

	got := Headers(h)

	require.Equal(t, map[string]string{
		"Authorization": Placeholder,
		"Accept":        "text/plain, application/json",
	}, got)
	require.Equal(t, "Bearer secret", h.Get("Authorization"), "input must not be mutated")
}
