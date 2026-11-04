package httprequest

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/trypanic/go-sdk/v2/internal/logctx"
	"github.com/trypanic/go-sdk/v2/internal/redact"
)

// sanitizeHTTPHeaders returns a log-safe copy of h: values of credential
// headers are masked by the shared redact rules, keys are kept.
func sanitizeHTTPHeaders(h http.Header) map[string]string {
	return redact.Headers(h)
}

// logHTTPAudit emits one structured audit log entry per outbound HTTP call.
// It records method, URL, status code, sanitized headers, full request and response bodies,
// and elapsed duration. Called after every executeRequest attempt (success or error).
func (h *Request) logHTTPAudit(
	ctx context.Context,
	start time.Time,
	config RequestConfig,
	reqHeaders http.Header,
	statusCode int,
	respHeaders http.Header,
	respBody []byte,
) {
	reqHeadersJSON, _ := json.Marshal(sanitizeHTTPHeaders(reqHeaders))
	respHeadersJSON, _ := json.Marshal(sanitizeHTTPHeaders(respHeaders))

	logctx.
		WithTrace(ctx, logctx.CtxOrGlobal(ctx).Info()).
		Str("event", "http.audit.outbound").
		Str("method", config.Method).
		Str("url", redact.URL(config.URL)).
		Int("status_code", statusCode).
		RawJSON("request.headers", reqHeadersJSON).
		Str("request.body", h.redactBody(config.Body)).
		RawJSON("response.headers", respHeadersJSON).
		Str("response.body", h.redactBody(respBody)).
		Int64("duration_ms", time.Since(start).Milliseconds()).
		Msg("http: outbound audit")
}
