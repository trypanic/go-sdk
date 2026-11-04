package httprequest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"time"

	"github.com/trypanic/go-sdk/errorkit"
	sharedbackoff "github.com/trypanic/go-sdk/internal/backoff"
	"github.com/trypanic/go-sdk/internal/redact"
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

// HTTPRequester sends one logical request (with retries) and decodes the
// response into out. Accept it in constructors so tests can pass a fake.
type HTTPRequester interface {
	Do(ctx context.Context, config RequestConfig, out any) error
}

// RetryConfig holds retry behavior configuration.
//
// A retriable error is retried only when sending the request again is
// safe: the method is idempotent (GET, HEAD, OPTIONS, TRACE, PUT, DELETE),
// the request carries a non-empty Idempotency-Key or X-Idempotency-Key
// header, or the connection was never made (a dial error). Set
// RetryNonIdempotent to retry every retriable error regardless.
type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	// MaxElapsed bounds the whole Do call, every attempt and wait
	// included. Zero means no bound beyond the context's own deadline.
	MaxElapsed time.Duration
	// RetryNonIdempotent also retries non-idempotent requests (POST, PATCH,
	// ...) that may already have reached the server. Enable it only when
	// the upstream deduplicates or a duplicate is harmless.
	RetryNonIdempotent bool
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

// redactorNeedsContent reports whether the configured redactor looks at the
// body bytes. DefaultBodyRedactor only needs to know whether the body is
// empty, so the response body is not copied for it.
func (h *HTTPRequest) redactorNeedsContent() bool {
	if h.redactor == nil {
		return false
	}
	return reflect.ValueOf(h.redactor).Pointer() != reflect.ValueOf(DefaultBodyRedactor).Pointer()
}

// nonEmptyBody stands in for a body that was read but not kept; any
// redactor that does not need content treats it as "non-empty".
var nonEmptyBody = []byte{0}

// auditBody returns the bytes to hand to the redactor for the audit line:
// the copied body when one was kept, otherwise a non-empty placeholder when
// the body had content.
func auditBody(buf *bytes.Buffer, read int64) []byte {
	if buf != nil {
		return buf.Bytes()
	}
	if read > 0 {
		return nonEmptyBody
	}
	return nil
}

// HTTPRequest implements the HTTPRequester interface.
type HTTPRequest struct {
	instrumenter  *telemetry.Instrumenter
	client        *http.Client
	retryConfig   RetryConfig
	auditLog      bool
	captureBodies bool
	redactor      BodyRedactor
	// maxResponseBytes caps a successful response body; see
	// WithMaxResponseBytes.
	maxResponseBytes int64
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

// normalizeRetryConfig fills zero values with defaults and clamps a
// negative MaxRetries to a single attempt. Called once in constructors so
// Do() does not mutate receiver state per-call.
func normalizeRetryConfig(rc RetryConfig) RetryConfig {
	switch {
	case rc.MaxRetries == 0:
		rc.MaxRetries = DefaultMaxRetries
	case rc.MaxRetries < 0:
		rc.MaxRetries = 1
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
		client:           client,
		retryConfig:      normalizeRetryConfig(RetryConfig{}),
		redactor:         DefaultBodyRedactor,
		maxResponseBytes: DefaultMaxResponseBytes,
	}
}

// New creates a new instance of HTTPRequest with default retry settings.
// Bodies are redacted by default and the audit log is disabled.
func New(client *http.Client) *HTTPRequest {
	return NewWithInstrumenter(client, telemetry.NewInstrumenter(telemetry.InstrumenterConfig{}))
}

// NewHTTPRequestWithRetry creates a new instance with custom retry configuration.
func NewHTTPRequestWithRetry(client *http.Client, retryConfig RetryConfig) *HTTPRequest {
	h := newPlain(client)
	h.retryConfig = normalizeRetryConfig(retryConfig)
	h.instrumenter = telemetry.NewInstrumenter(telemetry.InstrumenterConfig{})
	return h
}

// NewWithoutTracing creates an HTTPRequester with no automatic tracing wrapper.
// SDK consumers that want explicit tracing should call WrapWithInstrumenter.
func NewWithoutTracing(client *http.Client) *HTTPRequest {
	return newPlain(client)
}

// NewWithInstrumenter creates an HTTPRequester wrapped with the supplied instrumenter.
// Passing nil disables tracing.
func NewWithInstrumenter(client *http.Client, instrumenter *telemetry.Instrumenter) *HTTPRequest {
	h := newPlain(client)
	h.instrumenter = instrumenter
	return h
}

// NewWithOptions builds an HTTPRequester from explicit options. Tracing is
// disabled by default; pass WrapWithInstrumenter outside this constructor if
// tracing is needed.
func NewWithOptions(client *http.Client, opts ...Option) *HTTPRequest {
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
func (h *HTTPRequest) Do(ctx context.Context, config RequestConfig, out any) (err error) {
	if h.instrumenter != nil {
		var span telemetry.Span
		ctx, span = httpSpan(h.instrumenter, ctx, config)
		defer func() { telemetry.End(span, err) }()
	}

	var lastErr error
	span := trace.SpanFromContext(ctx)
	rc := h.retryConfig // local copy keeps Do concurrent-safe.
	// Every Do sends at least one request, even when the retry configuration
	// bypassed normalizeRetryConfig.
	rc.MaxRetries = max(rc.MaxRetries, 1)
	currentDelay := rc.InitialDelay

	if err := validateURL(config.URL); err != nil {
		return err
	}

	if rc.MaxElapsed > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rc.MaxElapsed)
		defer cancel()
	}

	for attempt := 0; attempt < rc.MaxRetries; attempt++ {

		// Check context before attempt
		if err := ctx.Err(); err != nil {
			return h.buildNetworkError(err, config.URL, config.Body)
		}

		if span.SpanContext().IsValid() {
			span.AddEvent("http.attempt", trace.WithAttributes(
				attribute.Int("retry.attempt", attempt+1),
				attribute.Int("retry.max", rc.MaxRetries),
				attribute.String("http.url", redact.URL(config.URL)),
			))
		}

		// Execute request
		retryAfter, sent, err := h.executeRequest(ctx, config, out)
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if error is retryable and resending is safe
		if !h.isRetryable(err) || !safeToResend(config, err, sent, rc) || attempt >= rc.MaxRetries-1 {
			return err
		}

		// Wait before retry: Retry-After when the server sent one, else
		// exponential backoff with full jitter.
		delay, ok := retryDelay(ctx, currentDelay, retryAfter, rc.MaxDelay)
		if !ok {
			return err
		}
		if !h.waitForRetry(ctx, delay) {
			return h.buildNetworkError(ctx.Err(), config.URL, config.Body)
		}

		currentDelay = nextDelay(currentDelay, rc.MaxDelay)
	}
	// Unreachable: the last attempt always returns above. The guard keeps Do
	// from ever reporting success for a request that was not sent.
	if lastErr == nil {
		lastErr = errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).
			With(errorkit.WithReason("request was not attempted"))
	}
	return lastErr
}

// nextDelay doubles the backoff cap, bounded by maxDelay. The actual wait
// is a random fraction of it (see retryDelay).
func nextDelay(currentDelay, maxDelay time.Duration) time.Duration {
	return sharedbackoff.Grow(currentDelay, maxDelay, 2)
}

// executeRequest performs a single HTTP request attempt. For a non-2xx
// response it also returns the wait requested by its Retry-After header.
// The bool reports evidence that any request in the attempt was sent.
func (h *HTTPRequest) executeRequest(ctx context.Context, config RequestConfig, out any) (time.Duration, bool, error) {
	start := time.Now()

	// Build request
	req, err := h.buildRequest(ctx, config)
	if err != nil {
		return 0, false, err
	}

	// Execute request
	res, sent, err := doTracked(h.client, req)
	if err != nil {
		return 0, sent, h.buildNetworkError(err, config.URL, config.Body)
	}
	defer drainAndClose(res.Body)

	// Check status code
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// Only the provider's error code and message are needed here.
		bodyBytes, _ := io.ReadAll(io.LimitReader(res.Body, maxErrorBodyBytes))
		if h.auditLog {
			h.logHTTPAudit(ctx, start, config, req.Header, res.StatusCode, res.Header, bodyBytes)
		}
		return parseRetryAfter(res.Header.Get("Retry-After"), time.Now()), true,
			h.buildHTTPError(res, bodyBytes, config.Body)
	}

	// Process successful response
	if out != nil {
		body := newLimitedBody(res.Body, h.maxResponseBytes)
		var reader io.Reader = body
		// Keep a copy of the body only when the audit line needs its content.
		var respBuf *bytes.Buffer
		if h.auditLog && h.redactorNeedsContent() {
			respBuf = new(bytes.Buffer)
			reader = io.TeeReader(body, respBuf)
		}
		err := h.processResponse(reader, out)
		if body.overLimit() {
			err = buildBodyTooLargeError(redact.URL(config.URL), h.maxResponseBytes)
		}
		if h.auditLog {
			h.logHTTPAudit(ctx, start, config, req.Header, res.StatusCode, res.Header, auditBody(respBuf, body.read))
		}
		return 0, true, err
	}

	if h.auditLog {
		h.logHTTPAudit(ctx, start, config, req.Header, res.StatusCode, res.Header, nil)
	}
	return 0, true, nil
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
				errorkit.WithWrapped(redact.URLError(err)),
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
	// to the caller's trace. The SDK installs no propagator: the service must
	// call otel.SetTextMapPropagator (e.g. propagation.TraceContext{}) at
	// startup, otherwise nothing is injected.
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	return req, nil
}

// isRetryable determines if an error should be retried using errorkit metadata.
func (h *HTTPRequest) isRetryable(err error) bool {
	appErr, ok := errors.AsType[*errorkit.AppError](err)
	// Use the Retriable field from errorkit metadata
	return ok && appErr.Metadata.Retriable
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
