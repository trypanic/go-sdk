// Package llmclient is a minimal client for OpenAI-compatible
// chat-completion endpoints (non-streaming only).
//
// Build a Config (or read it with ConfigFromEnv from LLM_API_KEY and
// LLM_ENDPOINT), pass an httprequest.HTTPRequester, and call Execute. The
// API key is required and is sent only over https unless
// Config.AllowInsecureEndpoint is set; it is never printed by String,
// GoString, MarshalJSON or LogValue. Each Execute is bounded by
// Config.Timeout (DefaultTimeout when zero). Execute is a POST, which
// httprequest does not resend once it may have reached the provider.
//
// New wraps the client with tracing through the global OTel tracer (span
// "llm.<model>"); NewWithoutTracing and NewWithInstrumenter follow the SDK
// constructor triplet. A Client is safe for concurrent use.
package llmclient
