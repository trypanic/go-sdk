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

func TestHeaderValueSanitizesURLHeaders(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Referer", "rEfErEr", "Location", "lOcAtIoN", "Content-Location"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, tt := range []struct{ name, raw, want string }{
				{"credentials query fragment", "https://user-secret:password-secret@example.test/callback?token=query-secret&code=other-secret#fragment-secret", "https://example.test/callback?token=REDACTED&code=REDACTED"},
				{"opaque URL", "https:user-secret:password-secret@example.test?code=query-secret", "[REDACTED]"},
				{"relative", "/callback?code=query-secret#fragment-secret", "/callback?code=REDACTED"},
				{"network path", "//user-secret:password-secret@example.test/callback?code=query-secret", "//example.test/callback?code=REDACTED"},
				{"malformed host", "https://[bad-host/callback?token=query-secret", "[invalid URL]"},
				{"malformed escape", "https://example.test/%zz?token=query-secret", "[invalid URL]"},
				{"malformed query escape", "/callback?code=%zz-secret", "/callback?code=REDACTED"},
				{"ordinary URL", "https://example.test/docs", "https://example.test/docs"},
				{"empty", "", ""},
			} {
				t.Run(tt.name, func(t *testing.T) { require.Equal(t, tt.want, HeaderValue(name, tt.raw)) })
			}
		})
	}
}

func TestHeadersSanitizesEachURLValueWithoutMutatingInput(t *testing.T) {
	t.Parallel()
	h := http.Header{
		"rEfErEr":       {"https://example.test/first", "https://user-secret:password-secret@example.test/second?token=query-secret#fragment-secret", "https://[bad-host?token=malformed-secret"},
		"Location":      {"/one?code=first-secret", "/two?code=second-secret"},
		"Authorization": {"first-secret", "second-secret"},
		"Accept":        {"text/plain", "application/json"},
	}
	original := h.Clone()
	require.Equal(t, map[string]string{
		"rEfErEr":       "https://example.test/first, https://example.test/second?token=REDACTED, [invalid URL]",
		"Location":      "/one?code=REDACTED, /two?code=REDACTED",
		"Authorization": "[REDACTED]",
		"Accept":        "text/plain, application/json",
	}, Headers(h))
	require.Equal(t, original, h)
}

func TestHeaderValueMasksCompoundURLHeaders(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Link", "lInK", "Refresh", "rEfReSh"} {
		t.Run(name, func(t *testing.T) {
			for _, value := range []string{
				`<https://example.test/next?token=query-secret>; rel="next", </previous?code=other-secret>; rel="prev"`,
				"0; url=/callback?code=query-secret#fragment-secret",
				"https://[bad-host?token=query-secret",
			} {
				require.Equal(t, "[REDACTED]", HeaderValue(name, value))
			}
		})
	}
}
