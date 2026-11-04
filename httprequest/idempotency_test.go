package httprequest

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strconv"
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

func TestRedirectDialFailureDoesNotReplayNonIdempotentRequest(t *testing.T) {
	t.Parallel()
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			t.Parallel()
			for _, tc := range []struct {
				name    string
				method  string
				headers map[string]string
				retry   RetryConfig
				want    int32
			}{
				{name: "POST", method: http.MethodPost, want: 1},
				{name: "PATCH", method: http.MethodPatch, want: 1},
				{name: "GET", method: http.MethodGet, want: 3},
				{name: "keyed POST", method: http.MethodPost, headers: map[string]string{"Idempotency-Key": "operation"}, want: 3},
				{name: "opted in POST", method: http.MethodPost, retry: RetryConfig{RetryNonIdempotent: true}, want: 3},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					var hits atomic.Int32
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						hits.Add(1)
						w.Header().Set("Location", "http://redirect.invalid/target")
						w.WriteHeader(status)
					}))
					t.Cleanup(srv.Close)
					transport := &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
						if addr == "redirect.invalid:80" {
							return nil, &net.OpError{Op: "dial", Net: network, Err: errors.New("refused")}
						}
						return (&net.Dialer{}).DialContext(ctx, network, addr)
					}}
					t.Cleanup(transport.CloseIdleConnections)
					var writes atomic.Int32
					ctx := httptrace.WithClientTrace(t.Context(), &httptrace.ClientTrace{WroteRequest: func(httptrace.WroteRequestInfo) { writes.Add(1) }})
					client := &http.Client{Transport: transport}
					err := NewWithOptions(client, fastRetry(tc.retry)).Do(ctx, RequestConfig{
						Method: tc.method, URL: srv.URL, Headers: tc.headers, Body: []byte("charge"),
					}, nil)
					appErr, ok := errors.AsType[*errorkit.AppError](err)
					require.True(t, ok)
					require.Equal(t, ERR_NETWORK_ERROR, appErr.Code())
					require.Equal(t, tc.want, hits.Load())
					require.GreaterOrEqual(t, writes.Load(), tc.want, "caller trace must still run")
					require.Nil(t, client.CheckRedirect, "caller client must not be mutated")
				})
			}
		})
	}
}

type redirectDialTransport struct{ hits atomic.Int32 }

func (tr *redirectDialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	if req.URL.Path == "/target" {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("refused")}
	}
	tr.hits.Add(1)
	return &http.Response{StatusCode: http.StatusTemporaryRedirect,
		Header: http.Header{"Location": {"/target"}}, Body: io.NopCloser(http.NoBody), Request: req}, nil
}

func TestRedirectWithoutTraceCallbacksDoesNotReplayPOST(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		policy func(*http.Request, []*http.Request) error
	}{
		{name: "default policy"},
		{name: "custom policy", policy: func(*http.Request, []*http.Request) error { return nil }},
		{name: "stop policy", policy: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := new(redirectDialTransport)
			client := &http.Client{Transport: tr, CheckRedirect: tc.policy}
			err := NewWithOptions(client, fastRetry(RetryConfig{})).Do(t.Context(), RequestConfig{
				Method: http.MethodPost, URL: "http://upstream.invalid/start", Body: []byte("charge"),
			}, nil)
			require.Error(t, err)
			require.Equal(t, int32(1), tr.hits.Load())
		})
	}
}

func TestTrackedAttemptPreservesDefaultRedirectLimit(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Location", "/again")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	err := NewWithOptions(srv.Client(), fastRetry(RetryConfig{})).Do(t.Context(), RequestConfig{
		Method: http.MethodPost, URL: srv.URL, Body: []byte("charge"),
	}, nil)
	require.Error(t, err)
	require.Equal(t, int32(10), hits.Load())
}

func TestCustomTransportInitialDialFailureIsRetried(t *testing.T) {
	t.Parallel()
	tr := &countingRoundTripper{next: new(redirectDialTransport)}
	err := NewWithOptions(&http.Client{Transport: tr}, fastRetry(RetryConfig{})).Do(t.Context(), RequestConfig{
		Method: http.MethodPost, URL: "http://upstream.invalid/target", Body: []byte("charge"),
	}, nil)
	require.Error(t, err)
	require.Equal(t, int32(3), tr.trips.Load())
}
