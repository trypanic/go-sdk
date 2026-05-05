# `httpclient` — Production-Tuned HTTP Client Factory

Package `httpclient` constructs production-tuned `*http.Client` values with a shared connection pool, configurable redirect policy, and an opt-in seam for transport middleware (tracing, metrics, etc.). The default build has **no telemetry imports** — tracing is only added when the caller passes `WithTransportWrapper(otelhttp.NewTransport)` (or any other RoundTripper wrapper).

> **Note on `Timeout`.** The default client sets `http.Client.Timeout` to `60s`. Use `WithTimeout(0)` or `ClientConfig.Timeout = 0` only when the caller reliably bounds requests with `context.WithTimeout` or `context.WithDeadline`, such as long-lived streaming requests.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Configuration / Settings](#configuration--settings)
  - [ClientConfig Fields](#clientconfig-fields)
  - [Default Values](#default-values)
  - [LLM Preset Values](#llm-preset-values)
- [API Reference](#api-reference)
  - [DefaultConfig](#defaultconfig)
  - [SetupForLLM](#setupforllm)
  - [NewClient](#newclient)
  - [NewClientWithOptions](#newclientwithoptions)
  - [NewDefaultClient](#newdefaultclient)
  - [ClientOption Functions](#clientoption-functions)
- [Real-World Usage](#real-world-usage)
  - [General-purpose outbound HTTP](#general-purpose-outbound-http)
  - [LLM provider requests](#llm-provider-requests)
  - [Opt-in OTel transport tracing](#opt-in-otel-transport-tracing)
- [Lifecycle / Concurrency Notes](#lifecycle--concurrency-notes)
- [Dependencies](#dependencies)

---

## Overview

| Symbol | Kind | Purpose |
|---|---|---|
| `ClientConfig` | struct | All tunable parameters for the transport and redirect policy |
| `ClientOption` | type | Functional option that mutates a `*ClientConfig` |
| `DefaultConfig()` | func | Returns a `*ClientConfig` filled with production defaults |
| `SetupForLLM()` | func | Returns a `*ClientConfig` preset for LLM API providers |
| `NewClient(config)` | func | Builds an `*http.Client` from an explicit `*ClientConfig` |
| `NewClientWithOptions(opts...)` | func | Builds an `*http.Client` starting from defaults, then applying options |
| `NewDefaultClient()` | func | Convenience wrapper: `NewClient(DefaultConfig())` |
| `WithTimeout` | option | Overall `http.Client` timeout |
| `WithDialTimeout` | option | TCP dial timeout |
| `WithKeepAlive` | option | Keep-alive probe interval |
| `WithMaxIdleConns` | option | Global idle connection ceiling |
| `WithMaxIdleConnsPerHost` | option | Per-host idle connection ceiling |
| `WithMaxConnsPerHost` | option | Per-host total connection ceiling |
| `WithIdleConnTimeout` | option | Idle connection eviction time |
| `WithTLSHandshakeTimeout` | option | TLS handshake timeout |
| `WithResponseHeaderTimeout` | option | Time to wait for the first response header byte |
| `WithMaxRedirects` | option | Redirect follow limit |
| `WithDisableCompression` | option | Toggle gzip decompression |
| `WithDisableKeepAlives` | option | Toggle HTTP keep-alives |
| `WithInsecureSkipVerify` | option | Skip TLS certificate verification |
| `WithTLSConfig` | option | Provide a fully custom `*tls.Config` |
| `WithTransportWrapper` | option | Opt-in middleware around the base transport (e.g. `otelhttp.NewTransport`) |

---

## Quick Start

```go
import (
    "github.com/trypanic/go-sdk/httpclient"
)

// Production-ready client with all defaults applied.
client := httpclient.NewDefaultClient()

resp, err := client.Get("https://api.example.com/v1/resource")
```

---

## Configuration / Settings

### ClientConfig Fields

```go
type ClientConfig struct {
    // Timeouts
    Timeout               time.Duration // Overall request timeout; 0 disables client-level deadline
    DialTimeout           time.Duration // TCP dial timeout
    KeepAlive             time.Duration // Keep-alive probe interval
    TLSHandshakeTimeout   time.Duration // TLS handshake timeout
    ResponseHeaderTimeout time.Duration // Time to wait for the first response header byte
    ExpectContinueTimeout time.Duration // Time to wait for a 100-continue response

    // Connection pooling
    MaxIdleConns        int           // Maximum idle connections across all hosts
    MaxIdleConnsPerHost int           // Maximum idle connections per host
    MaxConnsPerHost     int           // Maximum total connections per host
    IdleConnTimeout     time.Duration // Idle connection eviction time

    // Features
    DisableCompression bool // Disable automatic gzip decompression
    DisableKeepAlives  bool // Disable HTTP keep-alives (one request per connection)
    MaxRedirects       int  // Redirect follow limit; 0 = no redirects; negative = Go default

    // TLS
    InsecureSkipVerify bool        // Skip TLS certificate verification (not for production)
    TLSConfig          *tls.Config // Fully custom TLS config; takes precedence over InsecureSkipVerify
}
```

There are no environment variables. All configuration is provided through `ClientConfig` fields or `ClientOption` functions.

### Default Values

These are returned by `DefaultConfig()` and used by `NewDefaultClient()`.

| Field | Default | Constant |
|---|---|---|
| `Timeout` | `60s` | `DefaultTimeout` |
| `DialTimeout` | `60s` | `DefaultTimeout` |
| `KeepAlive` | `60s` | `DefaultKeepAlive` |
| `TLSHandshakeTimeout` | `10s` | `DefaultTLSHandshakeTimeout` |
| `ResponseHeaderTimeout` | `10s` | `DefaultResponseHeaderTimeout` |
| `ExpectContinueTimeout` | `1s` | `DefaultExpectContinueTimeout` |
| `MaxIdleConns` | `500` | `DefaultMaxIdleConns` |
| `MaxIdleConnsPerHost` | `50` | `DefaultMaxIdleConnsPerHost` |
| `MaxConnsPerHost` | `500` | `DefaultMaxConnsPerHost` |
| `IdleConnTimeout` | `120s` | `DefaultIdleConnTimeout` |
| `DisableCompression` | `false` | — |
| `DisableKeepAlives` | `false` | — |
| `MaxRedirects` | `10` | `DefaultMaxRedirects` |
| `InsecureSkipVerify` | `false` | — |

### LLM Preset Values

These are returned by `SetupForLLM()`, tuned for OpenAI, Anthropic, Google AI, and similar providers.

| Field | Value | Rationale |
|---|---|---|
| `Timeout` | `0` | Caller context controls long-lived streaming and generation deadlines |
| `DialTimeout` | `10s` | Fast connection expected; provider infra is stable |
| `KeepAlive` | `90s` | Keep connections alive through streaming responses |
| `TLSHandshakeTimeout` | `10s` | Same as default |
| `ResponseHeaderTimeout` | `4m` | Allows model warm-up and queue wait before first token |
| `ExpectContinueTimeout` | `2s` | Slightly more generous than default |
| `MaxIdleConns` | `50` | Moderate pool; respects LLM provider rate limits |
| `MaxIdleConnsPerHost` | `10` | Per-host cap |
| `MaxConnsPerHost` | `5` | Conservative; prevents overwhelming the provider |
| `IdleConnTimeout` | `120s` | Same as default |
| `DisableCompression` | `false` | Compression enabled for large prompts/responses |
| `DisableKeepAlives` | `false` | Keep-alive required for streaming |
| `MaxRedirects` | `5` | Reduced from default |
| `InsecureSkipVerify` | `false` | TLS verification always on |

---

## API Reference

### DefaultConfig

```go
func DefaultConfig() *ClientConfig
```

Returns a new `*ClientConfig` populated with the production default values listed above. The returned value is safe to mutate before passing to `NewClient`.

---

### SetupForLLM

```go
func SetupForLLM() *ClientConfig
```

Returns a new `*ClientConfig` preset for LLM API providers. The most significant deviations from `DefaultConfig` are `Timeout: 0` for long-lived streaming responses and `ResponseHeaderTimeout: 4 * time.Minute` to accommodate model warm-up and queueing before the first response byte arrives. Connection pool values are intentionally conservative to respect provider rate limits.

---

### NewClient

```go
func NewClient(config *ClientConfig) *http.Client
```

Constructs an `*http.Client` from the given config.

**Behavior:**

1. If `config` is `nil`, `newDefaultConfig()` is used.
2. A `net.Dialer` is created with `DialTimeout` and `KeepAlive`.
3. If `config.TLSConfig` is `nil`, a TLS config is created with `MinVersion: tls.VersionTLS13` and `InsecureSkipVerify` copied from config.
4. An `*http.Transport` is built with all pool, timeout, compression, keep-alive, and TLS fields.
   - `Proxy` is set to `http.ProxyFromEnvironment` (respects `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`).
   - `ForceAttemptHTTP2` is `true`.
5. If `config.TransportWrapper` is non-nil, the base `*http.Transport` is passed through it before being installed on the client. This is the seam for tracing/metrics middleware. The default is no wrapper.
6. A `CheckRedirect` policy is installed:
   - `MaxRedirects > 0`: follows up to `MaxRedirects` redirects; exceeding the limit returns `errorkit.ERR_EXTERNAL_SERVICE_ERROR` with a reason string `"stopped after N redirects"`.
   - `MaxRedirects == 0`: returns `http.ErrUseLastResponse` on the first redirect attempt, causing the caller to receive the redirect response directly.
   - `MaxRedirects < 0`: no `CheckRedirect` is set; Go's default behavior (up to 10 redirects) applies.

**Error conditions:**

| Condition | Error Code |
|---|---|
| Redirect count exceeds `MaxRedirects` | `ERR_EXTERNAL_SERVICE_ERROR` |

---

### NewClientWithOptions

```go
func NewClientWithOptions(opts ...ClientOption) *http.Client
```

Starts with `newDefaultConfig()`, applies each `ClientOption` in order, then delegates to `NewClient`. Use this when you need to override only a small number of fields.

```go
client := httpclient.NewClientWithOptions(
    httpclient.WithResponseHeaderTimeout(30 * time.Second),
    httpclient.WithMaxConnsPerHost(20),
)
```

---

### NewDefaultClient

```go
func NewDefaultClient() *http.Client
```

Convenience wrapper equivalent to `NewClient(DefaultConfig())`. Use this for standard outbound HTTP calls where the production defaults are appropriate.

---

### ClientOption Functions

All options follow the signature `func(c *ClientConfig)` and are applied by `NewClientWithOptions`.

| Function | Field Modified | Notes |
|---|---|---|
| `WithTimeout(d time.Duration)` | `Timeout` | Overall client request timeout; 0 disables it |
| `WithDialTimeout(d time.Duration)` | `DialTimeout` | TCP connection establishment |
| `WithKeepAlive(d time.Duration)` | `KeepAlive` | Keep-alive probe interval |
| `WithMaxIdleConns(n int)` | `MaxIdleConns` | Global idle pool ceiling |
| `WithMaxIdleConnsPerHost(n int)` | `MaxIdleConnsPerHost` | Per-host idle pool ceiling |
| `WithMaxConnsPerHost(n int)` | `MaxConnsPerHost` | Per-host total connection ceiling |
| `WithIdleConnTimeout(d time.Duration)` | `IdleConnTimeout` | Idle eviction timeout |
| `WithTLSHandshakeTimeout(d time.Duration)` | `TLSHandshakeTimeout` | TLS negotiation timeout |
| `WithResponseHeaderTimeout(d time.Duration)` | `ResponseHeaderTimeout` | Time until first header byte |
| `WithMaxRedirects(n int)` | `MaxRedirects` | 0 = no follow; negative = Go default |
| `WithDisableCompression(b bool)` | `DisableCompression` | `true` disables gzip |
| `WithDisableKeepAlives(b bool)` | `DisableKeepAlives` | `true` closes after each request |
| `WithInsecureSkipVerify(b bool)` | `InsecureSkipVerify` | Not for production use |
| `WithTLSConfig(cfg *tls.Config)` | `TLSConfig` | Overrides `InsecureSkipVerify` when non-nil |

---

## Real-World Usage

### General-purpose outbound HTTP

```go
import (
    "github.com/trypanic/go-sdk/httpclient"
    "github.com/trypanic/go-sdk/httprequest"
)

httpClient := httpclient.NewDefaultClient()
requester := httprequest.New(httpClient)
```

### LLM provider requests

```go
httpClient := httpclient.NewClient(httpclient.SetupForLLM())
requester := httprequest.NewHTTPRequestWithRetry(httpClient, httprequest.RetryConfig{
    MaxRetries: 1,
})
```

### Opt-in OTel transport tracing

```go
import "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

httpClient := httpclient.NewClientWithOptions(
    httpclient.WithTransportWrapper(func(rt http.RoundTripper) http.RoundTripper {
        return otelhttp.NewTransport(rt)
    }),
)
```

The package itself does not import `otelhttp` — callers add the dependency only when they need it.

---

## Lifecycle / Concurrency Notes

- `*http.Client` is safe for concurrent use. Create one instance per logical role (e.g., one for general HTTP, one for LLM) and share it across goroutines.
- The underlying `http.Transport` maintains its own connection pool. Do not close or replace the client between requests; doing so discards pooled connections.
- There is no `Close` method on `*http.Client`. To drain idle connections at shutdown, call `client.CloseIdleConnections()`.
- Each call to `NewClient`, `NewClientWithOptions`, or `NewDefaultClient` produces an independent transport with its own connection pool. Avoid creating a new client per request.

---

## Dependencies

| Package | Role |
|---|---|
| `github.com/trypanic/go-sdk/errorkit` | Structured error (`ERR_EXTERNAL_SERVICE_ERROR`) returned when the redirect limit is exceeded |

The package has no telemetry dependency. Callers wishing to add tracing supply a `TransportWrapper` (e.g. `otelhttp.NewTransport`) themselves.
