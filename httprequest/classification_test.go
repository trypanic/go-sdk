package httprequest

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpclient"
)

// countingRoundTripper counts round trips before delegating to next.
type countingRoundTripper struct {
	next  http.RoundTripper
	trips atomic.Int32
}

func (c *countingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.trips.Add(1)
	return c.next.RoundTrip(req)
}

func requireAppError(t *testing.T, err error, code errorkit.ErrorCode, retriable bool) *errorkit.AppError {
	t.Helper()
	appErr, ok := err.(*errorkit.AppError)
	require.True(t, ok, "want *errorkit.AppError, got %T (%v)", err, err)
	require.Equal(t, code, appErr.Code(), "error: %v", err)
	require.Equal(t, retriable, appErr.Metadata.Retriable)
	return appErr
}

func TestInvalidURLIsRejectedBeforeAnyAttempt(t *testing.T) {
	t.Parallel()

	for name, rawURL := range map[string]string{
		"empty":              "",
		"unparseable":        "://missing-scheme",
		"unsupported scheme": "ftp://example.com/file",
		"no scheme":          "example.com/path",
		"empty host":         "http:///path",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			rt := &countingRoundTripper{next: http.DefaultTransport}
			r := NewWithOptions(&http.Client{Transport: rt}, fastRetry(RetryConfig{}))

			err := r.Do(t.Context(), RequestConfig{Method: http.MethodGet, URL: rawURL}, nil)

			requireAppError(t, err, errorkit.ERR_SYSTEM_CONFIG_INVALID, false)
			require.Zero(t, rt.trips.Load(), "an invalid URL must not be sent or retried")
		})
	}
}

func TestRedirectLimitErrorIsPreservedAndNotRetried(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	client := httpclient.NewClient(&httpclient.ClientConfig{MaxRedirects: 2})
	r := NewWithOptions(client, fastRetry(RetryConfig{}))

	err := r.Do(t.Context(), RequestConfig{Method: http.MethodGet, URL: srv.URL}, nil)

	appErr := requireAppError(t, err, httpclient.ERR_EXTERNAL_SERVICE_ERROR, false)
	require.Contains(t, appErr.Reason, "redirects")
	// net/http counts the original request in via, as for its own limit of 10.
	require.Equal(t, int32(2), hits.Load(), "one attempt, not one per retry")
}

func TestTLSVerificationFailureIsNotRetried(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	var dials atomic.Int32
	dialer := &net.Dialer{}
	// The client does not trust the test server's certificate.
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dials.Add(1)
			return dialer.DialContext(ctx, network, address)
		},
	}}
	r := NewWithOptions(client, fastRetry(RetryConfig{}))

	err := r.Do(t.Context(), RequestConfig{Method: http.MethodGet, URL: srv.URL}, nil)

	requireAppError(t, err, httpclient.ERR_EXTERNAL_INVALID_RESPONSE, false)
	require.Equal(t, int32(1), dials.Load())
}

func TestCanceledContextIsNotAServerError(t *testing.T) {
	t.Parallel()

	t.Run("before the first attempt", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := NewWithOptions(http.DefaultClient).Do(ctx, RequestConfig{Method: http.MethodGet, URL: "http://127.0.0.1:1/"}, nil)

		appErr := requireAppError(t, err, ERR_REQUEST_CANCELED, false)
		require.Less(t, appErr.Metadata.HTTPStatus, 500)
		require.True(t, errors.Is(err, context.Canceled))
	})

	t.Run("during the request", func(t *testing.T) {
		t.Parallel()
		release := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}))
		t.Cleanup(func() { close(release); srv.Close() })

		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(20*time.Millisecond, cancel)

		err := NewWithOptions(srv.Client(), fastRetry(RetryConfig{})).
			Do(ctx, RequestConfig{Method: http.MethodGet, URL: srv.URL}, nil)

		requireAppError(t, err, ERR_REQUEST_CANCELED, false)
		require.True(t, errors.Is(err, context.Canceled))
	})
}
