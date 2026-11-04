package messaging

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetryHasExplicitElapsedBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		err := executeWithRetry(t.Context(), func() error {
			calls++
			time.Sleep(16 * time.Minute)
			return errors.New("operation failed")
		}, &RetryConfig{MaxAttempts: 3, InitialDelay: 1, MaxDelay: 1, BackoffMult: 1})
		require.Error(t, err)
		require.Equal(t, 1, calls, "do not retry after the 15 minute budget")
	})
}
