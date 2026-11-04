# llmclient

Minimal client for OpenAI-compatible chat-completion endpoints. The SDK only
implements the non-streaming path. Streaming, chunk callbacks, and post-hoc
JSON cleanup are intentionally not supported.

## Construction

```go
cfg := llmclient.Config{
    APIKey:   apiKey,
    Endpoint: "https://api.example.com/v1/chat/completions",
    Timeout:  30 * time.Second, // 0 = DefaultTimeout (5 min); < 0 = no client-side deadline
}
```

To read the conventional environment variables, use `ConfigFromEnv`:

```go
cfg, err := llmclient.ConfigFromEnv() // LLM_API_KEY (required), LLM_ENDPOINT (optional)
if err != nil {
    return err // ERR_SYSTEM_CONFIG_INVALID when LLM_API_KEY is unset
}
cfg.Timeout = 30 * time.Second
```

`ConfigFromEnv` is the only place `llmclient` reads the environment; the
constructors never do.

| Constructor | Returns | Tracing |
|---|---|---|
| `NewClient(httpRequest, cfg)` | `*Client` | None |
| `New(httpRequest, cfg)` | `LLMProvider` | Wraps with the global OTel tracer |
| `NewWithoutTracing(httpRequest, cfg)` | `LLMProvider` | None |
| `NewWithInstrumenter(httpRequest, cfg, instr)` | `LLMProvider` | Explicit instrumenter (`nil` skips tracing) |
| `WrapWithInstrumenter(provider, instr)` | `LLMProvider` | Wraps an existing provider |

`Config` and `Client` never print the API key: `String`, `GoString`
(`%v`, `%+v`, `%#v`), `MarshalJSON` and `LogValue` (slog) show
`[REDACTED]` for a set key and an empty value for an unset one.

`Config` is consumed at construction time. `Client` never mutates it, so the
returned value is safe for concurrent `Execute` calls.

## Execute

```go
// One requester for the process: a single attempt over a client tuned for
// slow first bytes (see httpclient.SetupForLLM).
requester := httprequest.NewHTTPRequestWithRetry(
    httpclient.NewClient(httpclient.SetupForLLM()),
    httprequest.RetryConfig{MaxRetries: 1},
)
provider := llmclient.New(requester, cfg)

content, err := provider.Execute(ctx, llmclient.LLMRequestConfig{
    Model:        "gpt-4o-mini",
    SystemPrompt: "You are a helpful assistant.",
    UserMessage:  "What is the capital of France?",
})
```

* `Model` is required; an empty value returns `ERR_VALIDATION_MISSING_FIELD`.
* Before anything is sent, `Execute` checks the config and returns
  `ERR_SYSTEM_CONFIG_INVALID` when `APIKey` is empty or `Endpoint` is not an
  absolute `https://` URL. A plain `http://` endpoint (for example a local
  model server) needs `Config.AllowInsecureEndpoint = true`, because the key
  then travels in cleartext.
* `Execute` derives a child context with a deadline of `cfg.Timeout` (or
  `DefaultTimeout`, 5 minutes, when it is zero) that bounds the whole call,
  retries included. `SetupForLLM` disables the per-attempt client timeout,
  so this deadline is what stops a stalled provider. A negative `Timeout`
  disables it; the caller's context must then carry a deadline.
* A response without `choices` returns `ERR_EXTERNAL_INVALID_RESPONSE` (not
  retriable).
* The response body is returned verbatim (no markdown / code-fence stripping).
  Callers that need to parse JSON wrapped in code fences must do so
  themselves.

## Retries

Every `Execute` is a `POST`, which `httprequest` does not resend after it may
have reached the provider: a 5xx, a 429 or a timeout returns the error after
one attempt, so one `Execute` never bills more than one completion. Only a
refused connection (nothing was sent) is retried. To retry rate limits and
provider errors anyway, build the requester with
`httprequest.RetryConfig{RetryNonIdempotent: true}`, accepting that a retried
call may be billed twice.

## Tracing

`New` and `NewWithInstrumenter` open a span named `llm.<model>` (or
`llm.execute` if the model is empty). The span is the parent of the
`httprequest` span produced by the underlying HTTP call, so end-to-end LLM
latency is visible in the trace tree without instrumenting call sites.
