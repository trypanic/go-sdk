package algorithms_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/v2/algorithms"
)

var _ func(algorithms.ExponentialBackoffConfig) algorithms.Backoff = algorithms.NewExponentialBackoff

func TestOwnedBackoffContract(t *testing.T) {
	b := algorithms.NewExponentialBackoff(algorithms.ExponentialBackoffConfig{MaxRetries: 2})

	require.NotEqual(t, algorithms.Stop, b.NextBackOff())
	require.Equal(t, algorithms.Stop, b.NextBackOff())
	b.Reset()
	require.NotEqual(t, algorithms.Stop, b.NextBackOff())
}
