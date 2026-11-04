package errorkit_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/errorkit"
)

func TestExternalCodesAvailableWithoutTransport(t *testing.T) {
	for _, tc := range []struct {
		code   errorkit.ErrorCode
		status int
		retry  bool
	}{
		{"ERR_EXTERNAL_SERVICE_UNAVAILABLE", 503, true},
		{"ERR_EXTERNAL_SERVICE_TIMEOUT", 504, true},
		{"ERR_EXTERNAL_SERVICE_ERROR", 503, true},
		{"ERR_EXTERNAL_INVALID_RESPONSE", 502, false},
	} {
		meta, ok := errorkit.GetMetadata(tc.code)
		require.True(t, ok, "%s", tc.code)
		require.Equal(t, tc.status, meta.HTTPStatus)
		require.Equal(t, tc.retry, meta.Retriable)
	}
}
