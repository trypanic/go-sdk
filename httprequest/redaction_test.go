package httprequest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/logger"
)

var leakedSecrets = []string{"p4ssw0rd", "SECRET123", "sk-live-ABCDEF"}

func requireNoSecrets(t *testing.T, where, s string) {
	t.Helper()
	for _, secret := range leakedSecrets {
		require.NotContains(t, s, secret, "%s leaks %q", where, secret)
	}
}

// appErrorText renders everything an AppError can expose to logs.
func appErrorText(t *testing.T, err error) string {
	t.Helper()
	appErr, ok := err.(*errorkit.AppError)
	require.True(t, ok, "want *errorkit.AppError, got %T", err)
	payload, mErr := json.Marshal(appErr.Payload)
	require.NoError(t, mErr)
	text := appErr.Error() + " " + appErr.Reason + " " + string(payload)
	if appErr.Wrapped != nil {
		text += " " + appErr.Wrapped.Error()
	}
	return text
}

func secretURL(base string) string {
	return strings.Replace(base, "http://", "http://user:p4ssw0rd@", 1) + "/v1/chat?key=SECRET123&model=m"
}

func TestHTTPErrorDoesNotLeakURLSecretsOrProviderMessage(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"error":{"code":"invalid_api_key","type":"auth","message":"Incorrect API key provided: sk-live-ABCDEF"}}`)
	}))
	t.Cleanup(srv.Close)

	var logBuf bytes.Buffer
	zl := zerolog.New(&logBuf)
	ctx := logger.WithLogger(t.Context(), &zl)

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	ctx, span := tp.Tracer("test").Start(ctx, "parent")

	r := NewWithOptions(srv.Client(), WithAuditLog(), WithRetryConfig(RetryConfig{MaxRetries: 1}))
	err := r.Do(ctx, RequestConfig{Method: http.MethodGet, URL: secretURL(srv.URL)}, nil)
	span.End()

	require.Error(t, err)
	text := appErrorText(t, err)
	requireNoSecrets(t, "error payload", text)

	appErr := err.(*errorkit.AppError)
	payload := appErr.Payload.(map[string]any)
	require.Equal(t, "invalid_api_key", payload["provider_error_code"], "provider_error_code is safe and kept")
	require.Equal(t, "auth", payload["provider_error_type"], "provider_error_type is safe and kept")
	require.NotContains(t, payload, "provider_error_message", "message needs WithBodyCapture")
	require.Contains(t, payload["url"], "/v1/chat", "the sanitized URL keeps host and path")
	require.Contains(t, payload["url"], "key=", "query keys are kept, values masked")

	require.NotEmpty(t, logBuf.String(), "audit log line expected")
	requireNoSecrets(t, "audit log", logBuf.String())

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	var events strings.Builder
	for _, ev := range spans[0].Events() {
		for _, kv := range ev.Attributes {
			events.WriteString(kv.Value.String())
			events.WriteString(" ")
		}
	}
	require.NotEmpty(t, events.String(), "attempt span event expected")
	requireNoSecrets(t, "span events", events.String())
}

func TestProviderMessageFollowsBodyCaptureAndRedactor(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"code":"bad","message":"echo: sk-live-ABCDEF"}}`)
	}))
	t.Cleanup(srv.Close)

	tests := []struct {
		name string
		opts []Option
		want string
	}{
		{name: "capture with default redactor", opts: []Option{WithBodyCapture()}, want: "[REDACTED]"},
		{name: "capture with raw bodies", opts: []Option{WithBodyCapture(), WithRawBodies()}, want: "echo: sk-live-ABCDEF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := NewWithOptions(srv.Client(), tt.opts...)
			err := r.Do(t.Context(), RequestConfig{Method: http.MethodGet, URL: srv.URL}, nil)
			appErr, ok := err.(*errorkit.AppError)
			require.True(t, ok, "want *errorkit.AppError, got %T", err)
			require.Equal(t, tt.want, appErr.Payload.(map[string]any)["provider_error_message"])
		})
	}
}

func TestNetworkErrorDoesNotLeakURLSecrets(t *testing.T) {
	t.Parallel()

	r := NewWithOptions(http.DefaultClient, WithRetryConfig(RetryConfig{MaxRetries: 1, InitialDelay: time.Millisecond}))
	err := r.Do(t.Context(), RequestConfig{
		Method: http.MethodGet,
		URL:    "http://user:p4ssw0rd@127.0.0.1:1/v1?key=SECRET123",
	}, nil)

	require.Error(t, err)
	requireNoSecrets(t, "network error", appErrorText(t, err))
}

func TestRequestBuildErrorDoesNotLeakURLSecrets(t *testing.T) {
	t.Parallel()

	r := NewWithOptions(http.DefaultClient, WithRetryConfig(RetryConfig{MaxRetries: 1}))
	// A control character makes url.Parse fail; its error embeds the raw URL.
	err := r.Do(t.Context(), RequestConfig{
		Method: http.MethodGet,
		URL:    "http://user:p4ssw0rd@127.0.0.1/v1?key=SECRET123\x7f",
	}, nil)

	require.Error(t, err)
	requireNoSecrets(t, "request build error", appErrorText(t, err))
}
