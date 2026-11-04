# httprequest — Retrying HTTP Client with Structured Error Mapping

Package `httprequest` wraps `net/http` with automatic retries (exponential backoff with full jitter, `Retry-After` honored), OpenTelemetry span events per attempt, and structured `errorkit` errors mapped from every HTTP status code and network condition.

---

## Overview

| Symbol                    | Kind      | Purpose                                                |
| ------------------------- | --------- | ------------------------------------------------------ |
| `HTTPRequester`           | Interface | Single-method contract for executing HTTP requests     |
| `HTTPRequest`             | Struct    | Concrete implementation of `HTTPRequester`             |
| `RequestConfig`           | Struct    | All parameters needed to build one `http.Request`      |
| `RetryConfig`             | Struct    | Controls max retries, initial delay, and delay ceiling |
| `New`                     | Function  | Creates an `HTTPRequest` with default retry settings (auto-traced via global tracer) |
| `NewHTTPRequestWithRetry` | Function  | Creates an `HTTPRequest` with custom `RetryConfig` (auto-traced) |
| `NewWithoutTracing`       | Function  | Creates a plain `HTTPRequester` with no tracing wrapper |
| `NewWithInstrumenter`     | Function  | Creates an `HTTPRequester` wrapped with an explicit `*telemetry.Instrumenter` (nil disables tracing) |
| `NewWithOptions`          | Function  | Builds an `HTTPRequester` from explicit options (`WithRetryConfig`, `WithAuditLog`, `WithBodyCapture`, `WithBodyRedactor`, `WithRawBodies`, `WithMaxResponseBytes`); no tracing wrapper |
| `WrapWithInstrumenter`    | Function  | Wraps an existing `HTTPRequester` with an explicit instrumenter |
| `BodyRedactor`            | Type      | `func([]byte) []byte` — policy applied to bodies in error payloads and audit logs |
| `DefaultBodyRedactor`     | Function  | Replaces any non-empty body with the literal `[REDACTED]` |
| `WithRetryConfig`         | Option    | Override the retry configuration |
| `WithAuditLog`            | Option    | Enable structured audit log lines per outbound HTTP call |
| `WithBodyCapture`         | Option    | Include redacted bodies in error payloads (off by default) |
| `WithBodyRedactor`        | Option    | Install a custom `BodyRedactor` |
| `WithRawBodies`           | Option    | Disable redaction (development only) |
| `WithMaxResponseBytes`    | Option    | Cap a successful response body (after decompression); non-positive restores the default |
| `DefaultMaxResponseBytes` | Constant  | `32 MiB`                                               |
| `DefaultMaxRetries`       | Constant  | `5`                                                    |
| `DefaultInitialDelay`     | Constant  | `100ms`                                                |
| `DefaultMaxDelay`         | Constant  | `5s`                                                   |

---

## Quick Start

```go
import (
    "context"

    "github.com/trypanic/go-sdk/httpclient"
    "github.com/trypanic/go-sdk/httprequest"
)

// 1. Build one requester per process with default retry settings (5 attempts,
//    jittered backoff capped at 100 ms → 5 s). httpclient.NewDefaultClient has
//    a 60 s per-attempt timeout; http.DefaultClient has none.
requester := httprequest.New(httpclient.NewDefaultClient())

// 2. Describe the request
cfg := httprequest.RequestConfig{
    Method:      "POST",
    URL:         "https://api.example.com/items",
    Body:        bodyBytes,
    ContentType: "application/json",
    Headers: map[string]string{
        "Authorization": "Bearer " + token,
    },
}

// 3. Execute and decode into a struct
var result MyResponse
if err := requester.Do(ctx, cfg, &result); err != nil {
    // err is always an *errorkit.AppError
}
```

---

## Configuration / Settings

### RequestConfig

```go
type RequestConfig struct {
    Method      string            // HTTP method, e.g. "GET", "POST"
    URL         string            // Full URL including scheme
    Body        []byte            // Optional request body; nil or empty sends no body
    ContentType string            // Sets Content-Type header when non-empty
    Headers     map[string]string // Additional headers applied after Content-Type
}
```

`Content-Type` is set via the dedicated `ContentType` field before iterating `Headers`, so `Headers` can override it if needed.

### RetryConfig

```go
type RetryConfig struct {
    MaxRetries   int           // Total number of attempts (not retries after first)
    InitialDelay time.Duration // Delay before the second attempt
    MaxDelay     time.Duration // Ceiling for the jittered backoff and for Retry-After
    MaxElapsed   time.Duration // Bound on the whole Do call (all attempts and waits); 0 = only ctx
    // Also retry non-idempotent requests that may have reached the server.
    RetryNonIdempotent bool
}
```

| Field          | Default (via `New`) | Zero value in a `RetryConfig` you pass | Negative value |
| -------------- | ------------------- | -------------------------------------- | -------------- |
| `MaxRetries`   | `5`                 | `5` (`DefaultMaxRetries`)              | `1` (single attempt) |
| `InitialDelay` | `100ms`             | `100ms`                                | used as given  |
| `MaxDelay`     | `5s`                | `5s`                                   | used as given  |
| `MaxElapsed`   | none                | none (only `ctx` bounds `Do`)          | none           |

**Total time.** Every attempt and every wait spends the same budget: the `ctx` deadline and, when set, `MaxElapsed`. Without either, one `Do` can take up to `MaxRetries` × the client's per-attempt timeout (`http.Client.Timeout`, 60 s by default) plus the waits, so give `ctx` a deadline or set `MaxElapsed`. A wait that would outlast the budget ends the retries early (see `Do`).

`MaxRetries` is the total number of attempts. Zero means the default (5), not "no retries"; for a single attempt set `MaxRetries: 1`. `Do` always sends at least one request.

This package reads no environment variables.

---

## API Reference

### `HTTPRequester`

```go
type HTTPRequester interface {
    Do(ctx context.Context, config RequestConfig, out any) error
}
```

The single-method interface used throughout the codebase. Accept this interface in constructors to allow test doubles.

---

### `New`

```go
func New(client *http.Client) HTTPRequester
```

Returns an `HTTPRequester` (an `*HTTPRequest` wrapped with tracing through the global OTel tracer) with `MaxRetries = 5`, `InitialDelay = 100ms`, `MaxDelay = 5s`.

---

### `NewHTTPRequestWithRetry`

```go
func NewHTTPRequestWithRetry(client *http.Client, retryConfig RetryConfig) HTTPRequester
```

Returns an `HTTPRequester` wrapped with tracing, like `New`, with `retryConfig` normalized (zero fields take the defaults, a negative `MaxRetries` becomes 1). For explicit options without tracing, use `NewWithOptions(client, WithRetryConfig(rc), …)`.

---

### `Do`

```go
func (h *HTTPRequest) Do(ctx context.Context, config RequestConfig, out any) error
```

**Execution sequence per attempt:**

1. Checks `ctx.Err()` before each attempt; cancels immediately with an `errorkit` network error.
2. Records an `http.attempt` OpenTelemetry span event (attributes: `retry.attempt`, `retry.max`, `http.url`) when a valid span exists in `ctx`.
3. Calls `executeRequest`, which builds the `http.Request`, sends it, checks the status, and decodes the body.
4. On success (2xx), returns `nil`.
5. On error, checks `isRetryable` via `errorkit.AppError.Metadata.Retriable` and whether resending is safe (see **Retryability** below). Errors that are not retriable, or not safe to resend, are returned immediately.
6. Picks the wait before the next attempt. When the response carried a `Retry-After` header (delay-seconds or HTTP-date, typically on 429 and 503), the wait is exactly that value; if it exceeds `MaxDelay`, the error is returned without retrying, because an earlier retry would fail by design. Otherwise the wait is a random duration in `[0, backoff)` ("full jitter"), so clients that failed together do not retry in lockstep. If the wait would outlast the `ctx` deadline, the last error is returned at once instead of sleeping into a timeout.
7. Waits using a `select` over `ctx.Done()` and `time.After`. Context cancellation during a wait returns a network error.
8. Doubles the backoff ceiling for the next attempt (`InitialDelay`, `2×`, `4×`, …), capped at `MaxDelay`.

**`out` decoding rules** (driven by the reflected type of `out`):

| Pointed-to type                   | Decoding strategy                            |
| --------------------------------- | -------------------------------------------- |
| `*string`                         | `io.ReadAll` → raw string                    |
| `*int`, `*int8` … `*int64`        | `io.ReadAll` → `strconv.ParseInt` (base 10)  |
| `*uint`, `*uint8` … `*uint64`     | `io.ReadAll` → `strconv.ParseUint` (base 10) |
| `*float32`, `*float64`            | `io.ReadAll` → `strconv.ParseFloat`          |
| `*bool`                           | `io.ReadAll` → `strconv.ParseBool`           |
| `*[]byte`                         | `io.ReadAll` → raw bytes                     |
| `*[]T`, `*struct`, `*map`, `*any` | `json.NewDecoder(body).Decode(out)`          |

Passing a non-pointer as `out` returns `ERR_SYSTEM_UNEXPECTED`. Passing `nil` skips decoding entirely (useful for DELETE or other no-body responses).

An empty body when JSON decoding is expected (`io.EOF`) is silently treated as success.

**Response size limits.** A successful body is read through a cap of `DefaultMaxResponseBytes` (32 MiB), measured after transparent gzip decompression; change it with `WithMaxResponseBytes(n)`. A larger body fails with a non-retriable `ERR_EXTERNAL_INVALID_RESPONSE` whose payload carries `url` and `max_response_bytes`. Only the first 64 KiB of a non-2xx body are read (enough for the provider error and for `response_body`). Before the body is closed, at most 256 KiB of any unread remainder are discarded so the connection can be reused.

**Error conditions and errorkit codes:**

| Condition                            | errorkit code                      |
| ------------------------------------ | ---------------------------------- |
| Invalid URL (unparseable, scheme not `http`/`https`, no host) — checked once, before any attempt | `ERR_SYSTEM_CONFIG_INVALID` |
| `http.NewRequestWithContext` failure | `ERR_SYSTEM_UNEXPECTED`            |
| `out` is not a pointer               | `ERR_SYSTEM_UNEXPECTED`            |
| Body read or parse failure           | `ERR_SYSTEM_UNEXPECTED`            |
| Success body over the size limit     | `ERR_EXTERNAL_INVALID_RESPONSE`    |
| `context.DeadlineExceeded`           | `ERR_NETWORK_TIMEOUT`              |
| `context.Canceled`                   | `ERR_REQUEST_CANCELED` (HTTP 499, not retriable; `errors.Is(err, context.Canceled)` holds) |
| `*errorkit.AppError` raised inside the client (e.g. the `httpclient` redirect limit) | returned unchanged |
| TLS certificate verification failure | `ERR_EXTERNAL_INVALID_RESPONSE` (not retriable) |
| `net.Error` with `Timeout() == true` | `ERR_NETWORK_TIMEOUT`              |
| Any other transport error            | `ERR_NETWORK_ERROR`                |
| HTTP 400                             | `ERR_CLIENT_BAD_REQUEST`           |
| HTTP 401                             | `ERR_AUTH_UNAUTHENTICATED`         |
| HTTP 403                             | `ERR_AUTH_UNAUTHORIZED`            |
| HTTP 404                             | `ERR_CLIENT_NOT_FOUND`             |
| HTTP 429                             | `ERR_CLIENT_RATE_LIMIT`            |
| HTTP 4xx (other)                     | `ERR_EXTERNAL_INVALID_RESPONSE`    |
| HTTP 503                             | `ERR_EXTERNAL_SERVICE_UNAVAILABLE` |
| HTTP 504                             | `ERR_EXTERNAL_SERVICE_TIMEOUT`     |
| HTTP 5xx (other)                     | `ERR_EXTERNAL_SERVICE_ERROR`       |
| Any other status code                | `ERR_SYSTEM_UNEXPECTED`            |

Network error payloads always include `url` and `original_error`. HTTP error payloads always include `url` and `status_code`, plus `provider_error_code` and `provider_error_type` when the body carries an OpenAI-style `{"error":{...}}` object. `response_body`, `request_body` and `provider_error_message` (which providers often use to echo input or partial keys) are added only with `WithBodyCapture()`, after the configured `BodyRedactor`.

**Logging.** `Do` never logs the errors it returns: the caller handles each error once (log it with `logger.ErrorCtx(ctx, err, …)`, retry, or translate it). For one log line per attempt, opt in with `WithAuditLog()`, which writes through `logger.CtxOrGlobal(ctx)` with trace correlation.

**URL sanitization.** Every URL placed in an error payload, a wrapped `*url.Error`, the audit log or a span event is sanitized first: userinfo (`user:pass@`) and the fragment are dropped, and each query value is replaced by `REDACTED` while the keys are kept (`?key=SECRET` → `?key=REDACTED`). Wrapped URL-error diagnostic text is replaced by `[REDACTED]`, because even parse failures can echo credentials from a redirect Location. The original cause remains available through `errors.Is` and `errors.As`; timeout classification is preserved.

**Retryability** starts from `errorkit.AppError.Metadata.Retriable`. The following codes are retriable by default: `ERR_NETWORK_TIMEOUT`, `ERR_NETWORK_ERROR`, `ERR_CLIENT_RATE_LIMIT`, `ERR_EXTERNAL_SERVICE_UNAVAILABLE`, `ERR_EXTERNAL_SERVICE_TIMEOUT`, `ERR_EXTERNAL_SERVICE_ERROR`.

A retriable error is retried only when sending the request again cannot duplicate a side effect:

- the method is idempotent: `GET` (or empty), `HEAD`, `OPTIONS`, `TRACE`, `PUT`, `DELETE`; or
- the request carries a non-empty `Idempotency-Key` or `X-Idempotency-Key` header; or
- the initial connection was never made (a dial error such as "connection refused"), and no request in the attempt was sent. A dial failure after a redirect does not make the original request safe to repeat.

A `POST` or `PATCH` that gets a 5xx, a 429 or a timeout is therefore sent once and its error returned. Set `RetryConfig.RetryNonIdempotent` to retry every retriable error regardless, only when the upstream deduplicates requests or a duplicate is harmless.

---

## Real-World Usage

### Service bootstrap

Build the requesters once at startup and share them. Two variants appear in practice:

```go
// Default client for service-to-service HTTP calls (up to 5 attempts).
httpRequest := httprequest.New(httpclient.NewDefaultClient())

// LLM client: a single attempt over a client tuned for slow first bytes.
llmRequest := httprequest.NewHTTPRequestWithRetry(
    httpclient.NewClient(httpclient.SetupForLLM()),
    httprequest.RetryConfig{MaxRetries: 1},
)
cfg, err := llmclient.ConfigFromEnv() // LLM_API_KEY, LLM_ENDPOINT
if err != nil {
    return err
}
llm := llmclient.New(llmRequest, cfg)
```

### Constructor injection

Inject `HTTPRequester` via constructor so the dependency is explicit and replaceable in tests:

```go
type AuthClient struct {
    httpRequest httprequest.HTTPRequester
    endpoint    string
}

func NewAuthClient(r httprequest.HTTPRequester, endpoint string) *AuthClient {
    return &AuthClient{httpRequest: r, endpoint: endpoint}
}

func (a *AuthClient) Login(ctx context.Context, body []byte) (*Response, error) {
    var resp Response
    err := a.httpRequest.Do(ctx, httprequest.RequestConfig{
        Method: "POST",
        URL:    a.endpoint,
        Body:   body,
        Headers: map[string]string{
            "Content-Type": "application/json",
            "Accept":       "application/json",
        },
    }, &resp)
    return &resp, err
}
```

### Body redaction and audit log (opt-in)

By default `HTTPRequest` does not place request or response bodies into structured error payloads. When body context is required for debugging, opt in with `WithBodyCapture()`. Captured bodies pass through a `BodyRedactor` (default replaces non-empty bodies with `[REDACTED]`):

```go
requester := httprequest.NewWithOptions(client,
    httprequest.WithBodyCapture(),
    httprequest.WithBodyRedactor(func(b []byte) []byte {
        // custom policy: keep length + content-type only
        return []byte(fmt.Sprintf("len=%d", len(b)))
    }),
)
```

`WithRawBodies()` disables redaction entirely — use only in development environments where the bodies are guaranteed not to contain credentials or PII.

`Referer`, `Location` and `Content-Location` header values use the same URL sanitization; malformed values become `[invalid URL]` and opaque URLs are masked in full. `Link` and `Refresh` are masked in full because they contain compound URL syntax. Each header value is sanitized before multiple values are joined; unrelated diagnostic headers are preserved.

The audit log (one structured entry per outbound request) is opt-in via `WithAuditLog()`. Bodies in audit log entries always pass through the configured redactor, even when `WithRawBodies` is not set.

### Sending JSON

```go
cfg := httprequest.RequestConfig{
    URL:         u,
    Method:      "POST",
    Body:        bodyBytes,
    ContentType: "application/json",
}

var result MyResponse
if err := requester.Do(ctx, cfg, &result); err != nil {
    return err
}
```

---

## Lifecycle / Concurrency Notes

- `HTTPRequest` is fully immutable after construction. Retry defaults are normalized once in the constructor; `Do` works on a local copy of `retryConfig` and never mutates the receiver. The instance is safe for concurrent use across goroutines.
- There is no `Close` method. Lifecycle management of the underlying `*http.Client` and its transport is the caller's responsibility.
- One instance should be created per process (or per distinct `http.Client`) and reused across requests; do not create a new `HTTPRequest` per request.

---

## Dependencies

| Package                                                            | Role                                                    |
| ------------------------------------------------------------------ | ------------------------------------------------------- |
| `net/http`                                                         | HTTP transport and request construction                 |
| `encoding/json`                                                    | JSON decoding of structured responses                   |
| `go.opentelemetry.io/otel/trace`                                   | Span event recording per retry attempt                  |
| `go.opentelemetry.io/otel/attribute`                               | OTEL attribute types for span events                    |
| `go.opentelemetry.io/otel/propagation`                             | W3C trace-context injection into outbound headers (needs a global propagator set by the service) |
| `github.com/trypanic/go-sdk/errorkit`                              | Structured error construction and retryability metadata |
| `github.com/trypanic/go-sdk/httpclient`                            | External-service error codes (`ERR_EXTERNAL_*`)         |
| `github.com/trypanic/go-sdk/logger`                                | Opt-in audit log line (`WithAuditLog`) only             |
| `github.com/trypanic/go-sdk/telemetry`                             | Tracing wrapper (`New`, `NewWithInstrumenter`)          |

### Concrete adapters and escape hatches (v2)

Constructors return concrete adapters; consumers should accept the narrow
interfaces they need. HTTP and LLM clients retain the constructor's instrumenter
inside the concrete adapter, producing one span per call. `WrapWithInstrumenter`
still preserves its input when the instrumenter is nil; an explicit non-nil wrapper
exposes `Unwrap()` to recover the enclosed interface. Wrapping an already traced
client intentionally creates another span, so wrap a tracing-disabled client.

Transport backoff growth and full jitter are shared with messaging through
`internal/backoff`; growth saturates before duration overflow. Public retry
configuration units, idempotency rules and `Retry-After` precedence are unchanged.

### Failure spans

Tracing adapters emit client-kind spans and finalize failures through
`telemetry.End(span, err)`: status Error, one exception event and, for classified
errors, `error.code` and `error.retriable`. Exception text uses the registered
metadata description (or "operation failed"), excluding private reasons, wrapped
errors, payloads and stacks. Successful spans retain their existing status.

### Stable operation names

Client spans use `external.http.<host>.<METHOD>`, for example
`external.http.api.example.com.GET`. The hostname excludes credentials and
ports and preserves IPv6 addresses. Standard methods are normalized to uppercase;
custom methods use `OTHER`. Missing or invalid hosts use `unknown`.
`server.address`, `http.request.method`, and `url.path` carry request details;
query values and userinfo are excluded from these attributes. Paths and resource
IDs do not affect span names. Update dashboards that selected the previous
host/path-based names.
