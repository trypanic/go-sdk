package httprequest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/trypanic/go-sdk/telemetry"
)

func TestNewWithoutTracingSkipsSpanWrapper(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := NewWithoutTracing(srv.Client())
	if _, ok := r.(*tracingHTTPRequester); ok {
		t.Fatalf("NewWithoutTracing should return unwrapped HTTPRequest, got tracingHTTPRequester")
	}

	var out string
	if err := r.Do(context.Background(), RequestConfig{Method: http.MethodGet, URL: srv.URL}, &out); err != nil {
		t.Fatalf("plain Do failed: %v", err)
	}
}

func TestWrapWithInstrumenterNilReturnsInner(t *testing.T) {
	t.Parallel()

	plain := NewWithoutTracing(http.DefaultClient)
	if got := WrapWithInstrumenter(plain, nil); got != plain {
		t.Fatalf("WrapWithInstrumenter(nil) must return inner unchanged")
	}
}

func TestWrapWithInstrumenterAddsTracing(t *testing.T) {
	t.Parallel()

	plain := NewWithoutTracing(http.DefaultClient)
	wrapped := WrapWithInstrumenter(plain, telemetry.NewInstrumenter(telemetry.InstrumenterConfig{}))
	if _, ok := wrapped.(*tracingHTTPRequester); !ok {
		t.Fatalf("expected tracingHTTPRequester, got %T", wrapped)
	}
}
