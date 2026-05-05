package httprequest

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestDefaultRetryConfigSetInConstructor(t *testing.T) {
	t.Parallel()

	r := NewWithoutTracing(http.DefaultClient).(*HTTPRequest)
	if r.retryConfig.MaxRetries != DefaultMaxRetries ||
		r.retryConfig.InitialDelay != DefaultInitialDelay ||
		r.retryConfig.MaxDelay != DefaultMaxDelay {
		t.Fatalf("retry defaults not applied at construction: %+v", r.retryConfig)
	}
}

func TestDoDoesNotMutateRetryConfig(t *testing.T) {
	t.Parallel()

	r := NewWithoutTracing(http.DefaultClient).(*HTTPRequest)
	r.retryConfig = RetryConfig{} // simulate caller passing zero values
	before := r.retryConfig

	// Drive Do once with a guaranteed unreachable URL so it returns quickly.
	_ = r.Do(context.Background(), RequestConfig{
		Method: http.MethodGet,
		URL:    "http://127.0.0.1:1/bogus",
	}, nil)

	if r.retryConfig != before {
		t.Fatalf("Do mutated h.retryConfig: before=%+v after=%+v", before, r.retryConfig)
	}
}

func TestDoIsConcurrentSafe(t *testing.T) {
	t.Parallel()

	r := NewWithOptions(http.DefaultClient, WithRetryConfig(RetryConfig{
		MaxRetries:   1,
		InitialDelay: time.Millisecond,
		MaxDelay:     time.Millisecond,
	}))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = r.Do(ctx, RequestConfig{
				Method: http.MethodGet,
				URL:    "http://127.0.0.1:1/bogus",
			}, nil)
		}()
	}
	wg.Wait()
}

func TestDefaultRedactorOmitsRequestBody(t *testing.T) {
	t.Parallel()

	h := NewWithoutTracing(http.DefaultClient).(*HTTPRequest)

	// captureBodies is false by default — payload should not include request_body.
	netErr := h.buildNetworkError(errors.New("boom"), "http://x", []byte("secret=42")).(*errorkit.AppError)
	pl, _ := netErr.Payload.(map[string]any)
	if _, ok := pl["request_body"]; ok {
		t.Fatalf("request_body must not appear in default error payload, got %v", netErr.Payload)
	}
}

func TestWithBodyCaptureRedactsBody(t *testing.T) {
	t.Parallel()

	h := NewWithOptions(http.DefaultClient, WithBodyCapture()).(*HTTPRequest)

	netErr := h.buildNetworkError(errors.New("boom"), "http://x", []byte("secret=42")).(*errorkit.AppError)
	body, ok := netErr.Payload.(map[string]any)["request_body"].(string)
	if !ok {
		t.Fatalf("request_body should be present with WithBodyCapture, got %#v", netErr.Payload)
	}
	if body != "[REDACTED]" {
		t.Fatalf("expected default redactor placeholder, got %q", body)
	}
}

func TestWithBodyRedactorCustom(t *testing.T) {
	t.Parallel()

	h := NewWithOptions(
		http.DefaultClient,
		WithBodyCapture(),
		WithBodyRedactor(func(b []byte) []byte { return []byte("size=" + itoa(len(b))) }),
	).(*HTTPRequest)

	netErr := h.buildNetworkError(errors.New("boom"), "http://x", []byte("payload-bytes")).(*errorkit.AppError)
	if got := netErr.Payload.(map[string]any)["request_body"].(string); !strings.HasPrefix(got, "size=") {
		t.Fatalf("custom redactor not applied, got %q", got)
	}
}

func TestWithRawBodiesPreservesContent(t *testing.T) {
	t.Parallel()

	h := NewWithOptions(http.DefaultClient, WithBodyCapture(), WithRawBodies()).(*HTTPRequest)

	netErr := h.buildNetworkError(errors.New("boom"), "http://x", []byte("raw=keep-me")).(*errorkit.AppError)
	if got := netErr.Payload.(map[string]any)["request_body"].(string); got != "raw=keep-me" {
		t.Fatalf("WithRawBodies should preserve body, got %q", got)
	}
}

// itoa keeps the test free of strconv import noise.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	const digits = "0123456789"
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = digits[i%10]
		i /= 10
	}
	return string(b[pos:])
}

// silence unused import when no httptest case lives here yet.
var _ = url.Parse
