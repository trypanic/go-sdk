# llmclient

Minimal client for OpenAI-compatible chat-completion endpoints. The SDK only
implements the non-streaming path. Streaming, chunk callbacks, and post-hoc
JSON cleanup are intentionally not supported.

## Construction

```go
cfg := llmclient.Config{
    APIKey:   os.Getenv("LLM_API_KEY"),
    Endpoint: "https://api.example.com/v1/chat/completions",
    Timeout:  30 * time.Second, // 0 = no client-side deadline
}
```

| Constructor | Returns | Tracing |
|---|---|---|
| `NewClient(httpRequest, cfg)` | `*Client` | None |
| `New(httpRequest, cfg)` | `LLMProvider` | Wraps with the global OTel tracer |
| `NewWithoutTracing(httpRequest, cfg)` | `LLMProvider` | None |
| `NewWithInstrumenter(httpRequest, cfg, instr)` | `LLMProvider` | Explicit instrumenter (`nil` skips tracing) |
| `WrapWithInstrumenter(provider, instr)` | `LLMProvider` | Wraps an existing provider |

`Config` is consumed at construction time. `Client` never mutates it, so the
returned value is safe for concurrent `Execute` calls.

## Execute

```go
provider := llmclient.New(httpClient, cfg)

content, err := provider.Execute(ctx, llmclient.LLMRequestConfig{
    Model:        "gpt-4o-mini",
    SystemPrompt: "You are a helpful assistant.",
    UserMessage:  "What is the capital of France?",
})
```

* `Model` is required; an empty value returns `ERR_VALIDATION_MISSING_FIELD`.
* When `cfg.Timeout > 0`, `Execute` derives a child context with that deadline
  for the duration of the HTTP call.
* The response body is returned verbatim (no markdown / code-fence stripping).
  Callers that need to parse JSON wrapped in code fences must do so
  themselves.

## Tracing

`New` and `NewWithInstrumenter` open a span named `llm.<model>` (or
`llm.execute` if the model is empty). The span is the parent of the
`httprequest` span produced by the underlying HTTP call, so end-to-end LLM
latency is visible in the trace tree without instrumenting call sites.
