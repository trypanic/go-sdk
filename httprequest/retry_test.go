package httprequest

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/v2/errorkit"
	"github.com/trypanic/go-sdk/v2/httpclient"
)

// countingServer returns an httptest server that answers every request with
// status and counts how many requests reached it.
func countingServer(t *testing.T, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestDoMaxRetriesAttempts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		maxRetries int
		wantHits   int32
	}{
		{name: "zero uses the default", maxRetries: 0, wantHits: DefaultMaxRetries},
		{name: "negative still sends one request", maxRetries: -1, wantHits: 1},
		{name: "very negative still sends one request", maxRetries: -100, wantHits: 1},
		{name: "one sends one request", maxRetries: 1, wantHits: 1},
		{name: "three sends three requests", maxRetries: 3, wantHits: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv, hits := countingServer(t, http.StatusServiceUnavailable)
			r := NewWithOptions(srv.Client(), WithRetryConfig(RetryConfig{
				MaxRetries:   tt.maxRetries,
				InitialDelay: time.Millisecond,
				MaxDelay:     time.Millisecond,
			}))

			err := r.Do(t.Context(), RequestConfig{Method: http.MethodGet, URL: srv.URL}, nil)

			require.Error(t, err, "a failing upstream must never be reported as success")
			appErr, ok := err.(*errorkit.AppError)
			require.True(t, ok, "want *errorkit.AppError, got %T", err)
			require.Equal(t, httpclient.ERR_EXTERNAL_SERVICE_UNAVAILABLE, appErr.Code())
			require.Equal(t, tt.wantHits, hits.Load())
		})
	}
}

// TestDoGuardsUnnormalizedRetryConfig covers an Request whose retry
// configuration bypassed the constructors: Do must still send one request.
func TestDoGuardsUnnormalizedRetryConfig(t *testing.T) {
	t.Parallel()

	srv, hits := countingServer(t, http.StatusServiceUnavailable)
	h := &Request{client: srv.Client(), retryConfig: RetryConfig{MaxRetries: -1}}

	err := h.Do(t.Context(), RequestConfig{Method: http.MethodGet, URL: srv.URL}, nil)

	require.Error(t, err)
	require.Equal(t, int32(1), hits.Load())
}
