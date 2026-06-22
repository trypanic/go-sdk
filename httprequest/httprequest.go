package httprequest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/trypanic/go-sdk/errorkit"
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
