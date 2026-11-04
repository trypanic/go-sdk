package httprequest

import (
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMaxElapsedBoundsAllAttempts(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// Each attempt fails after the server's Retry-After of 2 s; without
		// a budget, 5 attempts would take 8 s.
		tr, client := newScripted(scriptedResponse{status: http.StatusServiceUnavailable, retryAfter: "2"})
		r := NewWithOptions(client, WithRetryConfig(RetryConfig{
			MaxRetries: 5, InitialDelay: time.Second, MaxDelay: 5 * time.Second, MaxElapsed: 3 * time.Second,
		}))

		start := time.Now()
		err := r.Do(t.Context(), getReq, nil)

		require.Error(t, err)
		require.LessOrEqual(t, time.Since(start), 3*time.Second, "Do must not outlive MaxElapsed")
		require.Len(t, tr.gaps(), 1, "the third attempt would start after the budget, so it is not made")
	})
}
