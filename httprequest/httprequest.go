package httprequest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpclient"
	"github.com/trypanic/go-sdk/logger"
	"github.com/trypanic/go-sdk/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Default retry configuration
const (
	DefaultMaxRetries   = 5
	DefaultInitialDelay = 100 * time.Millisecond
	DefaultMaxDelay     = 5 * time.Second
)

// RequestConfig holds all parameters necessary to build an http.Request.
type RequestConfig struct {
	Method      string
	URL         string
	Body        []byte
	ContentType string
	Headers     map[string]string
}

type HTTPRequester interface {
	Do(ctx context.Context, config RequestConfig, out any) error
}

// RetryConfig holds retry behavior configuration.
type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

// BodyRedactor turns a raw HTTP body into a representation safe for logs and
// error payloads. The default redactor replaces any non-empty body with the
// constant `[REDACTED]` so credentials or PII never reach observability tools.
// SDK consumers may install a custom redactor to expose specific structure.
type BodyRedactor func([]byte) []byte

// DefaultBodyRedactor returns `[REDACTED]` for any non-empty input. It is the
// implicit redactor used by every constructor unless the caller installs a
// different one.
func DefaultBodyRedactor(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return []byte("[REDACTED]")
}

// rawBodyRedactor is the identity redactor used when WithRawBodies is set.
func rawBodyRedactor(b []byte) []byte { return b }

// HTTPRequest implements the HTTPRequester interface.
type HTTPRequest struct {
	client        *http.Client
	retryConfig   RetryConfig
	auditLog      bool
	captureBodies bool
	redactor      BodyRedactor
}

// Option configures an HTTPRequest at construction time.
type Option func(*HTTPRequest)

// WithRetryConfig overrides the default retry configuration.
func WithRetryConfig(rc RetryConfig) Option {
	return func(h *HTTPRequest) { h.retryConfig = normalizeRetryConfig(rc) }
}

// WithAuditLog enables structured audit log lines per outbound HTTP call.
// Bodies in audit log entries always pass through the configured redactor.
func WithAuditLog() Option {
	return func(h *HTTPRequest) { h.auditLog = true }
}

// WithBodyCapture includes request and response bodies (after redaction) in
// the payload of structured errors returned from Do. Off by default.
func WithBodyCapture() Option {
	return func(h *HTTPRequest) { h.captureBodies = true }
}

// WithBodyRedactor installs a custom redactor for body capture and audit log.
// Pass nil to fall back to DefaultBodyRedactor.
func WithBodyRedactor(r BodyRedactor) Option {
	return func(h *HTTPRequest) {
		if r == nil {
			h.redactor = DefaultBodyRedactor
			return
		}
		h.redactor = r
	}
}

// WithRawBodies disables redaction entirely. Use only in development or when
// the caller has independently confirmed the bodies carry no sensitive data.
func WithRawBodies() Option {
	return func(h *HTTPRequest) { h.redactor = rawBodyRedactor }
}

// Ensure HTTPRequest satisfies HTTPRequester
var _ HTTPRequester = (*HTTPRequest)(nil)

// normalizeRetryConfig fills zero values with defaults. Called once in
// constructors so Do() does not mutate receiver state per-call.
func normalizeRetryConfig(rc RetryConfig) RetryConfig {
	if rc.MaxRetries == 0 {
		rc.MaxRetries = DefaultMaxRetries
	}
	if rc.InitialDelay == 0 {
		rc.InitialDelay = DefaultInitialDelay
	}
	if rc.MaxDelay == 0 {
		rc.MaxDelay = DefaultMaxDelay
	}
	return rc
}

// newPlain creates a baseline HTTPRequest with default retry config and the
// default body redactor. Constructors compose this with options and tracing.
func newPlain(client *http.Client) *HTTPRequest {
	return &HTTPRequest{
		client:      client,
		retryConfig: normalizeRetryConfig(RetryConfig{}),
		redactor:    DefaultBodyRedactor,
	}
}

// New creates a new instance of HTTPRequest with default retry settings.
// Bodies are redacted by default and the audit log is disabled.
func New(client *http.Client) HTTPRequester {
	return WrapWithTracing(newPlain(client))
}

// NewHTTPRequestWithRetry creates a new instance with custom retry configuration.
func NewHTTPRequestWithRetry(client *http.Client, retryConfig RetryConfig) HTTPRequester {
	h := newPlain(client)
	h.retryConfig = normalizeRetryConfig(retryConfig)
	return WrapWithTracing(h)
}

// NewWithoutTracing creates an HTTPRequester with no automatic tracing wrapper.
// SDK consumers that want explicit tracing should call WrapWithInstrumenter.
func NewWithoutTracing(client *http.Client) HTTPRequester {
	return newPlain(client)
}

// NewWithInstrumenter creates an HTTPRequester wrapped with the supplied instrumenter.
// Passing nil disables tracing.
func NewWithInstrumenter(client *http.Client, instrumenter *telemetry.Instrumenter) HTTPRequester {
	return WrapWithInstrumenter(NewWithoutTracing(client), instrumenter)
}

// NewWithOptions builds an HTTPRequester from explicit options. Tracing is
// disabled by default; pass WrapWithInstrumenter outside this constructor if
// tracing is needed.
func NewWithOptions(client *http.Client, opts ...Option) HTTPRequester {
	h := newPlain(client)
	for _, opt := range opts {
		opt(h)
	}
	if h.redactor == nil {
		h.redactor = DefaultBodyRedactor
	}
	return h
}

// Do execute the request with retry logic and processes the response into 'out'.
// Do is safe for concurrent use after construction: it never mutates h.
func (h *HTTPRequest) Do(ctx context.Context, config RequestConfig, out any) error {
	var lastErr error
	span := trace.SpanFromContext(ctx)
	rc := h.retryConfig // local copy keeps Do concurrent-safe.
	currentDelay := rc.InitialDelay

	for attempt := 0; attempt < rc.MaxRetries; attempt++ {

		// Check context before attempt
		if err := ctx.Err(); err != nil {
			return h.buildNetworkError(err, config.URL, config.Body)
		}

		if span.SpanContext().IsValid() {
			span.AddEvent("http.attempt", trace.WithAttributes(
				attribute.Int("retry.attempt", attempt+1),
				attribute.Int("retry.max", rc.MaxRetries),
				attribute.String("http.url", config.URL),
			))
		}

		// Execute request
		err := h.executeRequest(ctx, config, out)
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if error is retryable
		if !h.isRetryable(err) || attempt >= rc.MaxRetries-1 {
			return err
		}

		// Wait before retry with exponential backoff
		if !h.waitForRetry(ctx, currentDelay) {
			return h.buildNetworkError(ctx.Err(), config.URL, config.Body)
		}

		currentDelay = nextDelay(currentDelay, rc.MaxDelay)
	}
	return lastErr
}

// nextDelay doubles delay capped at max.
func nextDelay(currentDelay, max time.Duration) time.Duration {
	d := currentDelay * 2
	if d > max {
		return max
	}
	return d
}

// executeRequest performs a single HTTP request attempt.
func (h *HTTPRequest) executeRequest(ctx context.Context, config RequestConfig, out any) error {
	start := time.Now()

	// Build request
	req, err := h.buildRequest(ctx, config)
	if err != nil {
		return err
	}

	// Execute request
	res, err := h.client.Do(req)
	if err != nil {
		return h.buildNetworkError(err, config.URL, config.Body)
	}
	defer res.Body.Close()

	// Check status code
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(res.Body)
		if h.auditLog {
			h.logHTTPAudit(ctx, start, config, req.Header, res.StatusCode, res.Header, bodyBytes)
		}
		return h.buildHTTPError(res, bodyBytes, config.Body)
	}

	// Process successful response
	if out != nil {
		var respBuf bytes.Buffer
		err := h.processResponse(io.TeeReader(res.Body, &respBuf), out)
		if h.auditLog {
			h.logHTTPAudit(ctx, start, config, req.Header, res.StatusCode, res.Header, respBuf.Bytes())
		}
		return err
	}

	if h.auditLog {
		h.logHTTPAudit(ctx, start, config, req.Header, res.StatusCode, res.Header, nil)
	}
	return nil
}

// buildRequest creates an http.Request from RequestConfig.
func (h *HTTPRequest) buildRequest(ctx context.Context, config RequestConfig) (*http.Request, error) {
	var body io.Reader
	if len(config.Body) > 0 {
		body = bytes.NewReader(config.Body)
	}

	req, err := http.NewRequestWithContext(ctx, config.Method, config.URL, body)
	if err != nil {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).
			With(
				errorkit.WithReason("Failed to create HTTP request"),
				errorkit.WithWrapped(err),
			)
	}

	// Set headers
	if config.ContentType != "" {
		req.Header.Set("Content-Type", config.ContentType)
	}
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	// Inject W3C trace context so downstream services can link incoming spans
	// to the caller's trace. Requires a global propagator to be configured
	// (installed by telemetry.InitProvider).
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	return req, nil
}

// processResponse decodes the response based on the type of 'out'.
func (h *HTTPRequest) processResponse(body io.Reader, out any) error {
	// Get the type of out
	outValue := reflect.ValueOf(out)
	if outValue.Kind() != reflect.Ptr {
		return errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).
			With(errorkit.WithReason("'out' parameter must be a pointer"))
	}

	outType := outValue.Elem().Type()

	// Handle different entities
	switch outType.Kind() {
	case reflect.String:
		return h.decodeString(body, out)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return h.decodeInt(body, out)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return h.decodeUint(body, out)
	case reflect.Float32, reflect.Float64:
		return h.decodeFloat(body, out)
	case reflect.Bool:
		return h.decodeBool(body, out)
	case reflect.Slice:
		// Check if it's []byte
		if outType == reflect.TypeOf([]byte{}) {
			return h.decodeBytes(body, out)
		}
		// Otherwise decode as JSON
		return h.decodeJSON(body, out)
	case reflect.Struct, reflect.Map, reflect.Interface:
		return h.decodeJSON(body, out)
	default:
		return h.decodeJSON(body, out)
	}
}

// decodeString reads the response body as a string.
func (h *HTTPRequest) decodeString(body io.Reader, out any) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return h.buildDecodeError(err)
	}
	*(out.(*string)) = string(data)
	return nil
}

// decodeInt reads the response body and parses it as an integer.
func (h *HTTPRequest) decodeInt(body io.Reader, out any) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return h.buildDecodeError(err)
	}

	val, err := strconv.ParseInt(string(bytes.TrimSpace(data)), 10, 64)
	if err != nil {
		return h.buildDecodeError(err)
	}

	outValue := reflect.ValueOf(out).Elem()
	outValue.SetInt(val)
	return nil
}

// decodeUint reads the response body and parses it as an unsigned integer.
func (h *HTTPRequest) decodeUint(body io.Reader, out any) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return h.buildDecodeError(err)
	}

	val, err := strconv.ParseUint(string(bytes.TrimSpace(data)), 10, 64)
	if err != nil {
		return h.buildDecodeError(err)
	}

	outValue := reflect.ValueOf(out).Elem()
	outValue.SetUint(val)
	return nil
}

// decodeFloat reads the response body and parses it as a float.
func (h *HTTPRequest) decodeFloat(body io.Reader, out any) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return h.buildDecodeError(err)
	}

	val, err := strconv.ParseFloat(string(bytes.TrimSpace(data)), 64)
	if err != nil {
		return h.buildDecodeError(err)
	}

	outValue := reflect.ValueOf(out).Elem()
	outValue.SetFloat(val)
	return nil
}

// decodeBool reads the response body and parses it as a boolean.
func (h *HTTPRequest) decodeBool(body io.Reader, out any) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return h.buildDecodeError(err)
	}

	val, err := strconv.ParseBool(string(bytes.TrimSpace(data)))
	if err != nil {
		return h.buildDecodeError(err)
	}

	*(out.(*bool)) = val
	return nil
}

// decodeBytes reads the response body as raw bytes.
func (h *HTTPRequest) decodeBytes(body io.Reader, out any) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return h.buildDecodeError(err)
	}
	*(out.(*[]byte)) = data
	return nil
}

// decodeJSON decodes the response body as JSON.
func (h *HTTPRequest) decodeJSON(body io.Reader, out any) error {
	if err := json.NewDecoder(body).Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			// Empty body is acceptable for some responses (e.g., 204 No Content)
			return nil
		}
		return h.buildDecodeError(err)
	}
	return nil
}

// buildDecodeError creates a standardized decode error.
func (h *HTTPRequest) buildDecodeError(err error) error {
	return errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).
		With(
			errorkit.WithReason("Failed to decode response body"),
			errorkit.WithWrapped(err),
		)
}

// redactBody returns the redactor output as a string. It centralizes the
// nil-safety logic so callers can always call it.
func (h *HTTPRequest) redactBody(b []byte) string {
	r := h.redactor
	if r == nil {
		r = DefaultBodyRedactor
	}
	return string(r(b))
}

// buildNetworkError creates a structured network error. Request body is only
// included when WithBodyCapture is enabled, and even then it is redacted.
func (h *HTTPRequest) buildNetworkError(err error, requestURL string, requestBody []byte) error {
	payload := map[string]any{
		"url":            requestURL,
		"original_error": err.Error(),
	}
	if h.captureBodies {
		payload["request_body"] = h.redactBody(requestBody)
	}

	// Check for context errors
	if errors.Is(err, context.DeadlineExceeded) {
		return errorkit.NewError(ERR_NETWORK_TIMEOUT).
			With(
				errorkit.WithReason("Request deadline exceeded"),
				errorkit.WithWrapped(err),
				errorkit.WithPayload(payload),
			)
	}

	if errors.Is(err, context.Canceled) {
		return errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).
			With(
				errorkit.WithReason("Request was canceled"),
				errorkit.WithWrapped(err),
				errorkit.WithPayload(payload),
			)
	}

	// Check for net.Error timeout
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return errorkit.NewError(ERR_NETWORK_TIMEOUT).
			With(
				errorkit.WithReason("Network operation timed out"),
				errorkit.WithWrapped(err),
				errorkit.WithPayload(payload),
			)
	}

	// Generic network error
	return errorkit.NewError(ERR_NETWORK_ERROR).
		With(
			errorkit.WithReason("Network communication error"),
			errorkit.WithWrapped(err),
			errorkit.WithPayload(payload),
		)
}

// buildHTTPError maps HTTP status codes to structured errors.
func (h *HTTPRequest) buildHTTPError(res *http.Response, bodyBytes []byte, requestBody []byte) error {
	statusCode := res.StatusCode
	requestURL := res.Request.URL.String()
	providerErr := parseProviderError(bodyBytes)

	payload := map[string]any{
		"url":         requestURL,
		"status_code": statusCode,
	}
	if h.captureBodies {
		payload["response_body"] = h.redactBody(bodyBytes)
		payload["request_body"] = h.redactBody(requestBody)
	}
	if providerErr.Code != "" {
		payload["provider_error_code"] = providerErr.Code
	}
	if providerErr.Type != "" {
		payload["provider_error_type"] = providerErr.Type
	}
	if providerErr.Message != "" {
		payload["provider_error_message"] = providerErr.Message
	}

	var errCode errorkit.ErrorCode
	var reason string
	forceNonRetriable := false

	// Map status codes to error codes
	switch {
	// 5xx Server Errors
	case statusCode == http.StatusServiceUnavailable: // 503
		errCode = httpclient.ERR_EXTERNAL_SERVICE_UNAVAILABLE
		reason = fmt.Sprintf("External service unavailable (%d)", statusCode)
	case statusCode == http.StatusGatewayTimeout: // 504
		errCode = httpclient.ERR_EXTERNAL_SERVICE_TIMEOUT
		reason = fmt.Sprintf("External service timeout (%d)", statusCode)
	case statusCode >= 500 && statusCode <= 599:
		errCode = httpclient.ERR_EXTERNAL_SERVICE_ERROR
		reason = fmt.Sprintf("External service error (%d)", statusCode)

	// 4xx Client Errors
	case statusCode == http.StatusTooManyRequests: // 429
		errCode = errorkit.ERR_CLIENT_RATE_LIMIT
		reason = fmt.Sprintf("Rate limit exceeded (%d)", statusCode)
		if strings.EqualFold(providerErr.Code, "insufficient_quota") {
			reason = fmt.Sprintf("Insufficient quota at provider (%s) (%d)", providerErr.Code, statusCode)
			forceNonRetriable = true
		}
	case statusCode == http.StatusNotFound: // 404
		errCode = errorkit.ERR_CLIENT_NOT_FOUND
		reason = fmt.Sprintf("Resource not found (%d)", statusCode)
	case statusCode == http.StatusBadRequest: // 400
		errCode = errorkit.ERR_CLIENT_BAD_REQUEST
		reason = fmt.Sprintf("Bad request (%d)", statusCode)
	case statusCode == http.StatusUnauthorized: // 401
		errCode = ERR_AUTH_UNAUTHENTICATED
		reason = fmt.Sprintf("Authentication required (%d)", statusCode)
	case statusCode == http.StatusForbidden: // 403
		errCode = ERR_AUTH_UNAUTHORIZED
		reason = fmt.Sprintf("Access forbidden (%d)", statusCode)
	case statusCode >= 400 && statusCode <= 499:
		errCode = httpclient.ERR_EXTERNAL_INVALID_RESPONSE
		reason = fmt.Sprintf("Invalid request or response (%d)", statusCode)

	// Unexpected status codes
	default:
		errCode = errorkit.ERR_SYSTEM_UNEXPECTED
		reason = fmt.Sprintf("Unexpected status code (%d)", statusCode)
	}

	appErr := errorkit.NewError(errCode).
		With(
			errorkit.WithReason("%s", reason),
			errorkit.WithPayload(payload),
		)
	if forceNonRetriable {
		appErr.Metadata.Retriable = false
	}
	logger.Error(appErr)
	return appErr
}

type providerError struct {
	Code    string
	Type    string
	Message string
}

func parseProviderError(bodyBytes []byte) providerError {
	var raw struct {
		Error struct {
			Code    string `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return providerError{}
	}

	return providerError{
		Code:    raw.Error.Code,
		Type:    raw.Error.Type,
		Message: raw.Error.Message,
	}
}

// isRetryable determines if an error should be retried using errorkit metadata.
func (h *HTTPRequest) isRetryable(err error) bool {
	var appErr *errorkit.AppError
	if !errors.As(err, &appErr) {
		return false
	}

	// Use the Retriable field from errorkit metadata
	return appErr.Metadata.Retriable
}

// waitForRetry waits for the specified delay with context cancellation support.
// Returns false if context was cancelled, true if wait completed successfully.
func (h *HTTPRequest) waitForRetry(ctx context.Context, delay time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(delay):
		return true
	}
}

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
