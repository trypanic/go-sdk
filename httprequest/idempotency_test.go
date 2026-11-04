package httprequest

import (
	"context"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/errorkit"
)

func fastRetry(rc RetryConfig) Option {
	rc.MaxRetries = 3
	rc.InitialDelay = time.Millisecond
	rc.MaxDelay = time.Millisecond
	return WithRetryConfig(rc)
}

func TestRetryHonorsMethodIdempotency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		method   string
		headers  map[string]string
		rc       RetryConfig
		wantHits int32
	}{
		{name: "POST on 500 is sent once", method: http.MethodPost, wantHits: 1},
		{name: "PATCH on 500 is sent once", method: http.MethodPatch, wantHits: 1},
		{name: "GET on 500 is retried", method: http.MethodGet, wantHits: 3},
		{name: "empty method means GET and is retried", method: "", wantHits: 3},
		{name: "HEAD on 500 is retried", method: http.MethodHead, wantHits: 3},
		{name: "OPTIONS on 500 is retried", method: http.MethodOptions, wantHits: 3},
		{name: "PUT on 500 is retried", method: http.MethodPut, wantHits: 3},
		{name: "DELETE on 500 is retried", method: http.MethodDelete, wantHits: 3},
		{
			name: "POST with Idempotency-Key is retried", method: http.MethodPost,
			headers: map[string]string{"idempotency-key": "k-1"}, wantHits: 3,
		},
		{
			name: "POST with X-Idempotency-Key is retried", method: http.MethodPost,
			headers: map[string]string{"X-Idempotency-Key": "k-1"}, wantHits: 3,
		},
		{
			name: "POST with an empty Idempotency-Key is sent once", method: http.MethodPost,
			headers: map[string]string{"Idempotency-Key": ""}, wantHits: 1,
		},
		{
			name: "POST is retried when RetryNonIdempotent is set", method: http.MethodPost,
			rc: RetryConfig{RetryNonIdempotent: true}, wantHits: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv, hits := countingServer(t, http.StatusInternalServerError)
			r := NewWithOptions(srv.Client(), fastRetry(tt.rc))

			err := r.Do(t.Context(), RequestConfig{
				Method: tt.method, URL: srv.URL, Body: []byte(`{}`), Headers: tt.headers,
			}, nil)

			require.Error(t, err)
			require.Equal(t, tt.wantHits, hits.Load())
		})
	}
}

func TestPOSTIsRetriedWhenTheConnectionWasNeverMade(t *testing.T) {
	t.Parallel()

	// A closed loopback port: every dial is refused, so nothing is sent.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	var dials atomic.Int32
	dialer := &net.Dialer{}
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dials.Add(1)
			return dialer.DialContext(ctx, network, address)
		},
	}}
	r := NewWithOptions(client, fastRetry(RetryConfig{}))

	err = r.Do(t.Context(), RequestConfig{Method: http.MethodPost, URL: "http://" + addr, Body: []byte(`{}`)}, nil)

	appErr, ok := err.(*errorkit.AppError)
	require.True(t, ok, "want *errorkit.AppError, got %T", err)
	require.Equal(t, ERR_NETWORK_ERROR, appErr.Code())
	require.Equal(t, int32(3), dials.Load(), "a refused dial proves the POST was not sent, so it is retried")
}
