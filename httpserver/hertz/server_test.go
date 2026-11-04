package hertz

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInboundSensitiveHeaderCoversCredentialHeaders(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"Authorization", "Proxy-Authorization", "X-Goog-Api-Key",
		"X-Amz-Security-Token", "X-Csrf-Token", "X-Vendor-Secret",
	} {
		require.True(t, isInboundSensitiveHeader(name), name)
	}
	for _, name := range []string{"Content-Type", "Accept", "User-Agent", "Keep-Alive"} {
		require.False(t, isInboundSensitiveHeader(name), name)
	}
}
