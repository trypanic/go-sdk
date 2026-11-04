package httprequest

import (
	"bytes"
	"compress/gzip"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpclient"
)

func bodyServer(t *testing.T, status int, body []byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestSuccessBodyOverLimitIsRejected(t *testing.T) {
	t.Parallel()

	srv, hits := bodyServer(t, http.StatusOK, bytes.Repeat([]byte("a"), 4096))
	var (
		s   string
		raw []byte
		doc map[string]any
	)
	for name, out := range map[string]any{"string": &s, "bytes": &raw, "json": &doc} {
		t.Run(name, func(t *testing.T) {
			r := NewWithOptions(srv.Client(), WithMaxResponseBytes(1024))
			before := hits.Load()

			err := r.Do(t.Context(), RequestConfig{Method: http.MethodGet, URL: srv.URL}, out)

			appErr, ok := err.(*errorkit.AppError)
			require.True(t, ok, "want *errorkit.AppError, got %T (%v)", err, err)
			require.Equal(t, httpclient.ERR_EXTERNAL_INVALID_RESPONSE, appErr.Code())
			require.False(t, appErr.Metadata.Retriable)
			require.Equal(t, int32(1), hits.Load()-before, "an oversized body is not retried")
		})
	}
}

func TestSuccessBodyAtLimitIsAccepted(t *testing.T) {
	t.Parallel()

	srv, _ := bodyServer(t, http.StatusOK, bytes.Repeat([]byte("a"), 1024))
	r := NewWithOptions(srv.Client(), WithMaxResponseBytes(1024))

	var s string
	require.NoError(t, r.Do(t.Context(), RequestConfig{Method: http.MethodGet, URL: srv.URL}, &s))
	require.Len(t, s, 1024)
}

func TestJSONSuccessBodyLimitIncludesTrailingData(t *testing.T) {
	t.Parallel()

	const limit = int64(len(`{"ok":true}`) + 5)
	tests := []struct {
		name          string
		body          string
		wantOverLimit bool
	}{
		{name: "valid", body: `{"ok":true}`},
		{name: "at limit with trailing whitespace", body: `{"ok":true}` + strings.Repeat(" ", 5)},
		{name: "over limit trailing whitespace", body: `{"ok":true}` + strings.Repeat(" ", 4096), wantOverLimit: true},
		{name: "over limit second JSON value", body: `{"ok":true} {"ok":false}`, wantOverLimit: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, hits := bodyServer(t, http.StatusOK, []byte(tt.body))
			r := NewWithOptions(srv.Client(), WithMaxResponseBytes(limit))
			var out map[string]any

			err := r.Do(t.Context(), RequestConfig{Method: http.MethodGet, URL: srv.URL}, &out)

			if !tt.wantOverLimit {
				require.NoError(t, err)
				require.Equal(t, map[string]any{"ok": true}, out)
				return
			}
			appErr, ok := err.(*errorkit.AppError)
			require.True(t, ok, "want *errorkit.AppError, got %T (%v)", err, err)
			require.Equal(t, httpclient.ERR_EXTERNAL_INVALID_RESPONSE, appErr.Code())
			require.False(t, appErr.Metadata.Retriable)
			require.Equal(t, int32(1), hits.Load(), "an oversized body is not retried")
		})
	}
}

func TestDefaultMaxResponseBytes(t *testing.T) {
	t.Parallel()

	r := NewWithoutTracing(http.DefaultClient).(*HTTPRequest)
	require.Equal(t, DefaultMaxResponseBytes, r.maxResponseBytes)

	r = NewWithOptions(http.DefaultClient, WithMaxResponseBytes(0)).(*HTTPRequest)
	require.Equal(t, DefaultMaxResponseBytes, r.maxResponseBytes, "non-positive limit restores the default")
}

func TestGzipBombIsBoundedAfterDecompression(t *testing.T) {
	t.Parallel()

	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, _ = zw.Write(bytes.Repeat([]byte("a"), 1<<20))
	require.NoError(t, zw.Close())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(compressed.Bytes())
	}))
	t.Cleanup(srv.Close)

	r := NewWithOptions(srv.Client(), WithMaxResponseBytes(64<<10))
	var s string
	err := r.Do(t.Context(), RequestConfig{Method: http.MethodGet, URL: srv.URL}, &s)

	appErr, ok := err.(*errorkit.AppError)
	require.True(t, ok, "want *errorkit.AppError, got %T (%v)", err, err)
	require.Equal(t, httpclient.ERR_EXTERNAL_INVALID_RESPONSE, appErr.Code())
}

func TestErrorBodyReadIsCapped(t *testing.T) {
	t.Parallel()

	srv, _ := bodyServer(t, http.StatusBadRequest, bytes.Repeat([]byte("e"), 1<<20))
	r := NewWithOptions(srv.Client(), WithBodyCapture(), WithRawBodies())

	err := r.Do(t.Context(), RequestConfig{Method: http.MethodGet, URL: srv.URL}, nil)

	appErr, ok := err.(*errorkit.AppError)
	require.True(t, ok, "want *errorkit.AppError, got %T", err)
	body := appErr.Payload.(map[string]any)["response_body"].(string)
	require.Len(t, body, maxErrorBodyBytes)
	require.Equal(t, strings.Repeat("e", maxErrorBodyBytes), body)
}

func TestUnreadBodiesKeepConnectionsReusable(t *testing.T) {
	t.Parallel()

	var conns atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("b"), 32<<10))
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			conns.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)

	r := NewWithOptions(srv.Client())
	for range 5 {
		require.NoError(t, r.Do(t.Context(), RequestConfig{Method: http.MethodGet, URL: srv.URL}, nil))
	}

	require.Equal(t, int32(1), conns.Load(), "unread bodies must be drained so the connection is reused")
}
