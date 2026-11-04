package algorithms_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/algorithms"
)

func TestOwnedBackoffContract(t *testing.T) {
	var constructor func(algorithms.ExponentialBackoffConfig) algorithms.Backoff = algorithms.NewExponentialBackoff
	b := constructor(algorithms.ExponentialBackoffConfig{MaxRetries: 2})
	require.NotEqual(t, algorithms.Stop, b.NextBackOff())
	require.Equal(t, algorithms.Stop, b.NextBackOff())
	b.Reset()
	require.NotEqual(t, algorithms.Stop, b.NextBackOff())
}
