package httprequest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPSpanNameDoesNotIncludeResourceIDs(t *testing.T) {
	require.Equal(t, httpSpanName("GET", "https://api.example.com/users/123"), httpSpanName("GET", "https://api.example.com/users/456?token=secret"))
	require.Equal(t, "external.http.2001:db8::1.GET", httpSpanName("GET", "https://[2001:db8::1]:8443/users/123"))
	require.Equal(t, "2001:db8::1", hostKeyword("[2001:db8::1]:8443"))
	require.Equal(t, "2001:db8::1", hostKeyword("[2001:db8::1]"))
}

func TestHTTPSpanDefaultMethodMatchesNetHTTP(t *testing.T) {
	require.Equal(t, "external.http.example.com.GET", httpSpanName("", "https://example.com/resource"))
}
