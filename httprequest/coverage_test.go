package httprequest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpclient"
	"github.com/trypanic/go-sdk/logger"
	"github.com/trypanic/go-sdk/telemetry"
)

func TestStatusMapsToCodeAndRetriability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status    int
		code      errorkit.ErrorCode
		retriable bool
	}{
		{http.StatusBadRequest, errorkit.ERR_CLIENT_BAD_REQUEST, false},
		{http.StatusUnauthorized, ERR_AUTH_UNAUTHENTICATED, false},
		{http.StatusForbidden, ERR_AUTH_UNAUTHORIZED, false},
		{http.StatusNotFound, errorkit.ERR_CLIENT_NOT_FOUND, false},
		{http.StatusConflict, httpclient.ERR_EXTERNAL_INVALID_RESPONSE, false},
		{http.StatusTooManyRequests, errorkit.ERR_CLIENT_RATE_LIMIT, true},
		{http.StatusInternalServerError, httpclient.ERR_EXTERNAL_SERVICE_ERROR, true},
		{http.StatusBadGateway, httpclient.ERR_EXTERNAL_SERVICE_ERROR, true},
		{http.StatusServiceUnavailable, httpclient.ERR_EXTERNAL_SERVICE_UNAVAILABLE, true},
		{http.StatusGatewayTimeout, httpclient.ERR_EXTERNAL_SERVICE_TIMEOUT, true},
		{http.StatusNotModified, errorkit.ERR_SYSTEM_UNEXPECTED, false},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			t.Parallel()

			srv, hits := countingServer(t, tt.status)
			r := NewWithOptions(srv.Client(), WithRetryConfig(RetryConfig{MaxRetries: 1}))

			err := r.Do(t.Context(), RequestConfig{Method: http.MethodGet, URL: srv.URL}, nil)

			appErr := requireAppError(t, err, tt.code, tt.retriable)
			require.Equal(t, tt.status, appErr.Payload.(map[string]any)["status_code"])
			require.Equal(t, int32(1), hits.Load())
		})
	}
}

func TestRetriesStopOnFirstSuccess(t *testing.T) {
	t.Parallel()

	tr, client := newScripted(
		scriptedResponse{status: http.StatusServiceUnavailable},
		scriptedResponse{status: http.StatusBadGateway},
		scriptedResponse{status: http.StatusOK},
	)
	r := NewWithOptions(client, fastRetry(RetryConfig{MaxRetries: 5}))

	require.NoError(t, r.Do(t.Context(), getReq, nil))
	require.Len(t, tr.gaps(), 2, "three attempts: two failures, then success")
}

func TestCancellationDuringBackoffWait(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		tr, client := newScripted(scriptedResponse{status: http.StatusServiceUnavailable, retryAfter: "3"})
		r := NewWithOptions(client, WithRetryConfig(RetryConfig{
			MaxRetries: 5, InitialDelay: time.Second, MaxDelay: 5 * time.Second,
		}))
		ctx, cancel := context.WithCancel(t.Context())
		time.AfterFunc(time.Second, cancel)

		start := time.Now()
		err := r.Do(ctx, getReq, nil)

		requireAppError(t, err, ERR_REQUEST_CANCELED, false)
		require.Equal(t, time.Second, time.Since(start), "the wait ends when ctx is canceled")
		require.Empty(t, tr.gaps(), "no attempt after cancellation")
	})
}

func TestConstructorsApplyDefaultsAndTracing(t *testing.T) {
	t.Parallel()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	instr := telemetry.NewInstrumenter(telemetry.InstrumenterConfig{TracerProvider: tp})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	for name, r := range map[string]HTTPRequester{
		"New":                     New(srv.Client()),
		"NewHTTPRequestWithRetry": NewHTTPRequestWithRetry(srv.Client(), RetryConfig{MaxRetries: 2}),
		"WrapWithTracing":         WrapWithTracing(NewWithoutTracing(srv.Client())),
	} {
		switch client := r.(type) {
		case *HTTPRequest:
			require.NotNil(t, client.instrumenter, name)
		case *tracingHTTPRequester:
			require.NotNil(t, client.instrumenter, name)
		default:
			t.Fatalf("%s: unexpected implementation %T", name, r)
		}
		require.NoError(t, r.Do(t.Context(), RequestConfig{Method: http.MethodGet, URL: srv.URL}, nil), name)
	}

	inner := NewHTTPRequestWithRetry(srv.Client(), RetryConfig{MaxRetries: 2})
	require.Equal(t, 2, inner.retryConfig.MaxRetries)
	require.Equal(t, DefaultInitialDelay, inner.retryConfig.InitialDelay)
	require.Equal(t, DefaultMaxDelay, inner.retryConfig.MaxDelay)

	r := NewWithInstrumenter(srv.Client(), instr)
	require.NoError(t, r.Do(t.Context(), RequestConfig{Method: http.MethodPost, URL: srv.URL + "/v1/chat/completions"}, nil))
	spans := recorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, "external.http.127.0.0.1.POST", spans[0].Name(), "path segments do not enter the span name")

	plain := NewWithInstrumenter(srv.Client(), nil)
	require.Nil(t, plain.instrumenter, "a nil instrumenter disables tracing")
	require.Equal(t, normalizeRetryConfig(RetryConfig{}), plain.retryConfig)
	require.Equal(t, DefaultMaxResponseBytes, plain.maxResponseBytes)
}

func TestHTTPSpanName(t *testing.T) {
	t.Parallel()

	for rawURL, want := range map[string]string{
		"https://openapi.taobao.com/purchase/order/render": "external.http.openapi.taobao.com.POST",
		"http://localhost:8080/items":                      "external.http.localhost.POST",
		"https://api.example.com/":                         "external.http.api.example.com.POST",
		"://bad":                                           "external.http.unknown.POST",
	} {
		require.Equal(t, want, httpSpanName("post", rawURL), rawURL)
	}
}

func TestAuditLogLineIsSanitized(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Set-Cookie", "session=server-secret")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"token":"response-secret"}`))
	}))
	t.Cleanup(srv.Close)

	var logBuf bytes.Buffer
	zl := zerolog.New(&logBuf)
	ctx := logger.WithLogger(t.Context(), &zl)

	var out map[string]any
	err := NewWithOptions(srv.Client(), WithAuditLog()).Do(ctx, RequestConfig{
		Method:  http.MethodPost,
		URL:     srv.URL + "/v1?key=query-secret",
		Body:    []byte(`{"password":"request-secret"}`),
		Headers: map[string]string{"Authorization": "Bearer header-secret", "X-Trace-Note": "visible"},
	}, &out)
	require.NoError(t, err)

	var line map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(logBuf.Bytes()), &line))
	require.Equal(t, "http.audit.outbound", line["event"])
	require.Equal(t, "POST", line["method"])
	require.EqualValues(t, http.StatusCreated, line["status_code"])
	require.Equal(t, "[REDACTED]", line["request.body"])
	require.Equal(t, "[REDACTED]", line["response.body"])
	reqHeaders := line["request.headers"].(map[string]any)
	require.Equal(t, "[REDACTED]", reqHeaders["Authorization"])
	require.Equal(t, "visible", reqHeaders["X-Trace-Note"])
	require.Equal(t, "[REDACTED]", line["response.headers"].(map[string]any)["Set-Cookie"])
	for _, secret := range []string{"query-secret", "request-secret", "response-secret", "header-secret", "server-secret"} {
		require.NotContains(t, logBuf.String(), secret)
	}
	require.Contains(t, line["url"], "key=REDACTED")
}

func FuzzParseProviderError(f *testing.F) {
	f.Add([]byte(`{"error":{"code":"c","type":"t","message":"m"}}`))
	f.Add([]byte(`{"error":"plain"}`))
	f.Add([]byte(`not json`))
	f.Fuzz(func(t *testing.T, body []byte) {
		_ = parseProviderError(body)
	})
}

func FuzzSpanNameHelpers(f *testing.F) {
	f.Add("api.example.com:443", "/a/b/c")
	f.Add("", "")
	f.Add("[::1]:80", "///")
	f.Fuzz(func(t *testing.T, host, path string) {
		require.NotEmpty(t, hostKeyword(host))
		// Arbitrary resource identifiers must not affect the operation name.
		base := "https://api.example.com/"
		require.Equal(t, httpSpanName("GET", base), httpSpanName("GET", base+url.PathEscape(path)))
	})
}

func FuzzParseRetryAfter(f *testing.F) {
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	f.Add("120")
	f.Add("Wed, 04 Nov 2026 00:01:00 GMT")
	f.Add("-1")
	f.Fuzz(func(t *testing.T, header string) {
		if d := parseRetryAfter(header, now); d < 0 {
			t.Fatalf("parseRetryAfter(%q) = %v, want >= 0", header, d)
		}
	})
}
