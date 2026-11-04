package httprequest

import (
	"context"
	"net"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/trace"

	"github.com/trypanic/go-sdk/telemetry"
)

// WrapWithTracing adds bounded external.http.<host>.<method> client spans.
// The URL path is an attribute; query strings and credentials are omitted.
func WrapWithTracing(inner Requester) Requester {
	return WrapWithInstrumenter(inner, telemetry.NewInstrumenter(telemetry.InstrumenterConfig{}))
}

// WrapWithInstrumenter wraps an Requester with an explicit instrumenter so
// SDK consumers can opt in to tracing without depending on global tracer state.
// Pass nil to disable tracing entirely (returns inner unchanged).
func WrapWithInstrumenter(inner Requester, instrumenter *telemetry.Instrumenter) Requester {
	if instrumenter == nil {
		return inner
	}
	return &tracingHTTPRequester{inner: inner, instrumenter: instrumenter}
}

type tracingHTTPRequester struct {
	inner        Requester
	instrumenter *telemetry.Instrumenter
}

func (t *tracingHTTPRequester) Do(ctx context.Context, config RequestConfig, out any) (err error) {
	ctx, span := httpSpan(t.instrumenter, ctx, config)
	defer func() { telemetry.End(span, err) }()
	return t.inner.Do(ctx, config, out)
}

func httpSpan(i *telemetry.Instrumenter, ctx context.Context, config RequestConfig) (context.Context, telemetry.Span) {
	host, method, path := httpSpanParts(config.Method, config.URL)
	return i.External(ctx, "http."+host, method, telemetry.WithSpanKind(trace.SpanKindClient), telemetry.WithString("server.address", host), telemetry.WithString("http.request.method", method), telemetry.WithString("url.path", path))
}

func httpSpanName(method, rawURL string) string {
	host, method, _ := httpSpanParts(method, rawURL)
	return "external.http." + host + "." + method
}

func httpSpanParts(method, rawURL string) (string, string, string) {
	method = strings.ToUpper(method)
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "CONNECT", "TRACE":
	default:
		method = "OTHER"
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "unknown", method, ""
	}
	return hostKeyword(u.Host), method, u.EscapedPath()
}

// hostKeyword strips an optional port without truncating IPv6 addresses.
func hostKeyword(host string) string {
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if host == "" {
		return "unknown"
	}
	return strings.ToLower(host)
}

// Unwrap returns the request implementation enclosed by this decorator.
func (t *tracingHTTPRequester) Unwrap() Requester { return t.inner }
