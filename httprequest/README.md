# httprequest — Retrying HTTP Client with Structured Error Mapping

Package `httprequest` wraps `net/http` with automatic exponential-backoff retries, OpenTelemetry span events per attempt, and structured `errorkit` errors mapped from every HTTP status code and network condition.

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
| `NewWithOptions`          | Function  | Builds an `HTTPRequester` from explicit options (`WithRetryConfig`, `WithAuditLog`, `WithBodyCapture`, `WithBodyRedactor`, `WithRawBodies`); no tracing wrapper |
| `WrapWithInstrumenter`    | Function  | Wraps an existing `HTTPRequester` with an explicit instrumenter |
| `BodyRedactor`            | Type      | `func([]byte) []byte` — policy applied to bodies in error payloads and audit logs |
| `DefaultBodyRedactor`     | Function  | Replaces any non-empty body with the literal `[REDACTED]` |
| `WithRetryConfig`         | Option    | Override the retry configuration |
| `WithAuditLog`            | Option    | Enable structured audit log lines per outbound HTTP call |
| `WithBodyCapture`         | Option    | Include redacted bodies in error payloads (off by default) |
| `WithBodyRedactor`        | Option    | Install a custom `BodyRedactor` |
| `WithRawBodies`           | Option    | Disable redaction (development only) |
| `DefaultMaxRetries`       | Constant  | `5`                                                    |
| `DefaultInitialDelay`     | Constant  | `100ms`                                                |
| `DefaultMaxDelay`         | Constant  | `5s`                                                   |

---

## Quick Start

```go
import (
    "context"
    "net/http"

    "github.com/trypanic/go-sdk/httprequest"
)

// 1. Build a requester with default retry settings (5 retries, 100 ms → 5 s backoff)
requester := httprequest.New(http.DefaultClient)

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
    MaxDelay     time.Duration // Ceiling for exponential backoff
}
```

| Field          | Default (via `New`) | Zero-value fallback (via `NewHTTPRequestWithRetry`) |
| -------------- | ------------------- | --------------------------------------------------- |
| `MaxRetries`   | `5`                 | `1` (single attempt)                                |
| `InitialDelay` | `100ms`             | `100ms`                                             |
| `MaxDelay`     | `5s`                | `5s`                                                |

When `MaxRetries` is zero in a `RetryConfig` passed to `NewHTTPRequestWithRetry`, the client performs exactly one attempt.

There are no environment variables read by this package directly. The surrounding `internal/platform` builder selects which constructor to call.

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

Returns an `*HTTPRequest` with `MaxRetries = 5`, `InitialDelay = 100ms`, `MaxDelay = 5s`. Used by `internal/platform` when wiring the default HTTP client for service-to-service calls.

---

### `NewHTTPRequestWithRetry`

```go
func NewHTTPRequestWithRetry(client *http.Client, retryConfig RetryConfig) *HTTPRequest
```

Returns `*HTTPRequest` (concrete type, not the interface) so callers that need to inspect or swap retry settings can do so. Used by `internal/platform` when building the LLM client with `MaxRetries: 1`.

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
5. On error, checks `isRetryable` via `errorkit.AppError.Metadata.Retriable`. Non-retryable errors are returned immediately.
6. Waits for the current delay using a `select` over `ctx.Done()` and `time.After`. Context cancellation during a wait returns a network error.
7. Doubles the delay for the next attempt, capped at `MaxDelay`.

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

**Error conditions and errorkit codes:**

| Condition                            | errorkit code                      |
| ------------------------------------ | ---------------------------------- |
| `http.NewRequestWithContext` failure | `ERR_SYSTEM_UNEXPECTED`            |
| `out` is not a pointer               | `ERR_SYSTEM_UNEXPECTED`            |
| Body read or parse failure           | `ERR_SYSTEM_UNEXPECTED`            |
| `context.DeadlineExceeded`           | `ERR_NETWORK_TIMEOUT`              |
| `context.Canceled`                   | `ERR_SYSTEM_UNEXPECTED`            |
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

Network error payloads always include `url` and `original_error`. HTTP error payloads always include `url`, `status_code`, and `response_body`.

**Retryability** is determined entirely by `errorkit.AppError.Metadata.Retriable`. The following codes are retriable by default: `ERR_NETWORK_TIMEOUT`, `ERR_NETWORK_ERROR`, `ERR_CLIENT_RATE_LIMIT`, `ERR_EXTERNAL_SERVICE_UNAVAILABLE`, `ERR_EXTERNAL_SERVICE_TIMEOUT`, `ERR_EXTERNAL_SERVICE_ERROR`.

---

## Real-World Usage

### Platform Builder (internal/platform)

The `internal/platform` builder constructs and exposes `HTTPRequester` in the shared `Container`. Two variants appear in practice:

```go
// Default client for service-to-service HTTP calls (5 retries)
httpClient := httpclient.NewDefaultClient()
httpRequest := httprequest.New(httpClient)
container.HTTPRequest = httpRequest

// LLM client — single attempt with a tuned http.Client
httpClient := httpclient.NewClient(httpclient.SetupForLLM())
httpRequest := httprequest.NewHTTPRequestWithRetry(httpClient, httprequest.RetryConfig{
    MaxRetries: 1,
})
llm := llmclient.New(httpRequest, llmclient.ClientConfig{ ... })
container.LLM = llm
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
| `github.com/trypanic/go-sdk/errorkit` | Structured error construction and retryability metadata |
