package httprequest

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/logger"
)

// captureStderr runs fn with os.Stderr redirected and returns what was
// written. The SDK's fallback global logger writes to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stderr
	os.Stderr = w
	done := make(chan []byte)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()
	defer func() {
		os.Stderr = orig
	}()
	fn()
	os.Stderr = orig
	require.NoError(t, w.Close())
	return string(<-done)
}

// TestNon2xxIsNotLoggedByDefault: Do returns the error to the caller, who
// decides whether to log it; the library itself writes nothing.
// Not parallel: it redirects os.Stderr.
func TestNon2xxIsNotLoggedByDefault(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusUnauthorized, http.StatusInternalServerError} {
		srv, hits := countingServer(t, status)

		var ctxLog bytes.Buffer
		zl := zerolog.New(&ctxLog)
		ctx := logger.WithLogger(t.Context(), &zl)

		r := NewWithOptions(srv.Client(), WithRetryConfig(RetryConfig{
			MaxRetries: 3, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond,
		}))
		var err error
		stderr := captureStderr(t, func() {
			err = r.Do(ctx, RequestConfig{Method: http.MethodGet, URL: srv.URL}, nil)
		})

		require.Error(t, err)
		require.NotZero(t, hits.Load())
		require.Empty(t, stderr, "status %d: the global logger must not log a returned error", status)
		require.Empty(t, ctxLog.String(), "status %d: the context logger must not log a returned error", status)
	}
}
