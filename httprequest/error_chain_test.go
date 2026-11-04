package httprequest

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/errorkit"
)

func TestTransportClassificationSkipsTypedNil(t *testing.T) {
	var absent *errorkit.AppError
	expected := errorkit.NewError(errorkit.ERR_EXTERNAL_SERVICE_UNAVAILABLE)
	request := newPlain(nil)
	joined := errors.Join(absent, expected)
	require.Same(t, expected, request.buildNetworkError(joined, "https://example.invalid", nil))
	require.NotPanics(t, func() { require.True(t, request.isRetryable(joined)) })
	fallback := request.buildNetworkError(absent, "https://example.invalid", nil)
	app, ok := errorkit.As(fallback)
	require.True(t, ok)
	require.NotNil(t, app)
	require.NotPanics(t, func() { require.False(t, request.isRetryable(absent)) })
}
