package urlkit_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/urlkit"
)

func TestEscapedSlashBoundaries(t *testing.T) {
	for _, base := range []string{"https://example.com/a%2Fb", "https://example.com/a%2F"} {
		joined, err := urlkit.JoinPath(base, "x/y")
		require.NoError(t, err)
		require.Equal(t, base+"/x/y", joined)
		built, err := urlkit.BuildURL(base, "x%2Fy", nil)
		require.NoError(t, err)
		require.Equal(t, base+"/x%2Fy", built.String())
	}
	joined, err := urlkit.JoinPath("https://example.com/a%2Fb", "x%2Fy")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/a%2Fb/x%252Fy", joined, "JoinPath segments are raw text")
}
func TestOpaqueBasesRejected(t *testing.T) {
	for _, base := range []string{"mailto:user@example.com", "custom:opaque"} {
		_, err := urlkit.JoinPath(base, "child")
		require.True(t, errorkit.HasCode(err, errorkit.ERR_VALIDATION_INVALID_FORMAT))
		_, err = urlkit.BuildURL(base, "child", nil)
		require.True(t, errorkit.HasCode(err, errorkit.ERR_VALIDATION_INVALID_FORMAT))
	}
}
