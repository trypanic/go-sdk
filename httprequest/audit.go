package httprequest

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/trypanic/go-sdk/logger"
)

// sensitiveHTTPHeaderKeys is the set of header names (lowercase) that must never be logged.
var sensitiveHTTPHeaderKeys = map[string]struct{}{
	"authorization":  {},
	"x-api-key":      {},
	"api-key":        {},
	"cookie":         {},
	"set-cookie":     {},
	"x-auth-token":   {},
	"x-access-token": {},
}

// sanitizeHTTPHeaders returns a copy of h with security-sensitive keys removed.
func sanitizeHTTPHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		if _, skip := sensitiveHTTPHeaderKeys[strings.ToLower(k)]; !skip {
			out[k] = strings.Join(v, ", ")
		}
	}
	return out
}

// logHTTPAudit emits one structured audit log entry per outbound HTTP call.
// It records method, URL, status code, sanitized headers, full request and response bodies,
// and elapsed duration. Called after every executeRequest attempt (success or error).
func (h *HTTPRequest) logHTTPAudit(
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

	logger.
		WithTrace(ctx, logger.CtxOrGlobal(ctx).Info()).
		Str("event", "http.audit.outbound").
		Str("method", config.Method).
		Str("url", config.URL).
		Int("status_code", statusCode).
		RawJSON("request.headers", reqHeadersJSON).
		Str("request.body", h.redactBody(config.Body)).
		RawJSON("response.headers", respHeadersJSON).
		Str("response.body", h.redactBody(respBody)).
		Int64("duration_ms", time.Since(start).Milliseconds()).
		Msg("http: outbound audit")
}
