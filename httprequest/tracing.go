package httprequest

import (
	"context"
	"net/url"
	"strings"

	"github.com/trypanic/go-sdk/telemetry"
)

// WrapWithTracing wraps an HTTPRequester with automatic span creation.
//
// Each call to Do opens a span derived from the request URL and method:
//
//	"http.<domain>.<METHOD>.<parent>.<last>"
//
// Domain extraction: takes the second-to-last part of the hostname.
//
//	"openapi.taobao.com" → "taobao"
//	"api.example.com"   → "example"
//	"localhost"         → "localhost"
//
// Path extraction: takes the last two non-empty segments of the URL path,
// providing resource context alongside the action name.
//
// Examples:
//
//	GET  https://openapi.taobao.com/product/spus/get      → span: "http.taobao.GET.spus.get"
//	POST https://openapi.taobao.com/product/details/query → span: "http.taobao.POST.details.query"
//	POST https://openapi.taobao.com/purchase/order/render → span: "http.taobao.POST.order.render"
//
// The span becomes the parent of the underlying HTTP transport spans, making the
// trace tree immediately readable without any instrumentation in datasource methods.
//
// Usage (in bootstrap):
//
//	datasources.NewTaobao(httprequest.WrapWithTracing(httpClient), ...)
func WrapWithTracing(inner HTTPRequester) HTTPRequester {
	return WrapWithInstrumenter(inner, telemetry.NewInstrumenter(telemetry.InstrumenterConfig{}))
}

// WrapWithInstrumenter wraps an HTTPRequester with an explicit instrumenter so
// SDK consumers can opt in to tracing without depending on global tracer state.
// Pass nil to disable tracing entirely (returns inner unchanged).
func WrapWithInstrumenter(inner HTTPRequester, instrumenter *telemetry.Instrumenter) HTTPRequester {
	if instrumenter == nil {
		return inner
	}
	return &tracingHTTPRequester{inner: inner, instrumenter: instrumenter}
}

type tracingHTTPRequester struct {
	inner        HTTPRequester
	instrumenter *telemetry.Instrumenter
}

func (t *tracingHTTPRequester) Do(ctx context.Context, config RequestConfig, out any) error {
	ctx, span := t.instrumenter.Start(ctx, httpSpanName(config.Method, config.URL))
	defer span.End()
	return t.inner.Do(ctx, config, out)
}

func httpSpanName(method, rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "http.unknown." + strings.ToUpper(method) + ".unknown"
	}
	return "http." + hostKeyword(u.Host) + "." + strings.ToUpper(method) + "." + lastTwoPathSegments(u.Path)
}

// hostKeyword extracts the meaningful identifier from a hostname.
// Takes the second-to-last label (before the TLD) for multi-part domains,
// or the full label for single-part hostnames like "localhost".
func hostKeyword(host string) string {
	// Strip port if present
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	parts := strings.Split(host, ".")
	switch len(parts) {
	case 0:
		return "unknown"
	case 1:
		return parts[0]
	default:
		return parts[len(parts)-2]
	}
}

// lastTwoPathSegments returns the last two non-empty segments of a URL path joined by a dot.
// "/product/spus/get"      → "spus.get"
// "/purchase/order/render" → "order.render"
// "/items"                 → "items"
// "/"                      → "root"
func lastTwoPathSegments(p string) string {
	parts := strings.FieldsFunc(p, func(r rune) bool { return r == '/' })
	switch len(parts) {
	case 0:
		return "root"
	case 1:
		return parts[0]
	default:
		return parts[len(parts)-2] + "." + parts[len(parts)-1]
	}
}
