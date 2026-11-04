package llmclient

import (
	"context"
	"net/url"
	"time"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httprequest"
	"github.com/trypanic/go-sdk/marshal"
	"github.com/trypanic/go-sdk/telemetry"
)

// Message is one chat message (role "system", "user" or "assistant").
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Request is the chat-completions request body.
type Request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitzero"`
	Temperature float64   `json:"temperature,omitzero"`
}

// Response is the chat-completions response body.
type Response struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice is one completion in an Response.
type Choice struct {
	Index   int     `json:"index"`
	Message Message `json:"message"`
}

// Usage reports the token counts of a completion.
type Usage struct {
	CompletionTokens int `json:"completion_tokens"`
	PromptTokens     int `json:"prompt_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Provider runs one chat completion and returns the first choice's
// content. *Client and the tracing wrapper implement it.
type Provider interface {
	Execute(ctx context.Context, reqConfig RequestConfig) (string, error)
}

// RequestConfig holds configuration for individual LLM requests.
// Streaming is intentionally not part of this contract; the SDK does not
// implement chunked responses.
type RequestConfig struct {
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
	UserMessage  string `json:"user_message"`
}

// Config carries the connection settings for an LLM client. It is consumed
// at construction time and never mutated by Client.
type Config struct {
	APIKey   string
	Endpoint string
	// Timeout bounds each Execute call end to end, every retry included.
	// Zero applies DefaultTimeout; a negative value disables the
	// client-side deadline (the caller's context must then bound it).
	Timeout time.Duration
	// AllowInsecureEndpoint permits a plain http:// Endpoint, which sends
	// the API key in cleartext. Use it only for local model servers.
	AllowInsecureEndpoint bool
}

// DefaultTimeout bounds an Execute call when Config.Timeout is zero. It is
// long enough for slow generations and stops a stalled upstream from
// holding the goroutine and connection forever.
const DefaultTimeout = 5 * time.Minute

// Client provides LLM interaction capabilities. After construction it is safe
// for concurrent use; its fields are not mutated by Execute.
type Client struct {
	instrumenter *telemetry.Instrumenter
	httpRequest  httprequest.Requester
	cfg          Config
}

// NewClient builds a concrete *Client with explicit Config. No tracing wrapper.
func NewClient(httpRequest httprequest.Requester, cfg Config) *Client {
	return &Client{httpRequest: httpRequest, cfg: cfg}
}

// New builds an Provider wrapped with the global tracer.
func New(httpRequest httprequest.Requester, cfg Config) *Client {
	return NewWithInstrumenter(httpRequest, cfg, telemetry.NewInstrumenter(telemetry.InstrumenterConfig{}))
}

// NewWithoutTracing builds an Provider with no tracing wrapper.
func NewWithoutTracing(httpRequest httprequest.Requester, cfg Config) *Client {
	return NewClient(httpRequest, cfg)
}

// NewWithInstrumenter builds an Provider wrapped with the supplied
// instrumenter; pass nil to disable tracing.
func NewWithInstrumenter(httpRequest httprequest.Requester, cfg Config, instrumenter *telemetry.Instrumenter) *Client {
	client := NewClient(httpRequest, cfg)
	client.instrumenter = instrumenter
	return client
}

// validate checks the settings Execute needs before the API key is sent:
// a key, and an absolute https endpoint (http only with
// AllowInsecureEndpoint). The endpoint is never echoed, as it may embed
// credentials.
func (c Config) validate() error {
	reason := ""
	u, err := url.Parse(c.Endpoint)
	switch {
	case c.APIKey == "":
		reason = "llm api key is empty"
	case err != nil || u.Host == "":
		reason = "llm endpoint must be an absolute URL"
	case u.Scheme == "https":
		return nil
	case u.Scheme == "http" && c.AllowInsecureEndpoint:
		return nil
	case u.Scheme == "http":
		reason = "llm endpoint must use https (set AllowInsecureEndpoint for a local http server)"
	default:
		reason = "llm endpoint must use https"
	}
	return errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(errorkit.WithReason("%s", reason))
}

// timeout returns the per-call deadline: Config.Timeout, DefaultTimeout
// when it is zero, or 0 (none) when it is negative.
func (c *Client) timeout() time.Duration {
	switch {
	case c.cfg.Timeout == 0:
		return DefaultTimeout
	case c.cfg.Timeout < 0:
		return 0
	}
	return c.cfg.Timeout
}

// Execute performs a non-streaming LLM request.
func (c *Client) Execute(ctx context.Context, reqConfig RequestConfig) (result string, err error) {
	if c.instrumenter != nil {
		var span telemetry.Span
		ctx, span = llmSpan(ctx, c.instrumenter, reqConfig.Model)
		defer func() { telemetry.End(span, err) }()
	}

	if reqConfig.Model == "" {
		return "", errorkit.
			NewError(errorkit.ERR_VALIDATION_MISSING_FIELD).
			With(errorkit.WithReason("model is required"))
	}

	if err := c.cfg.validate(); err != nil {
		return "", err
	}

	if timeout := c.timeout(); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	body, err := marshal.NoEscapeBytes(Request{
		Model: reqConfig.Model,
		Messages: []Message{
			{Role: "system", Content: reqConfig.SystemPrompt},
			{Role: "user", Content: reqConfig.UserMessage},
		},
	})
	if err != nil {
		return "", errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("failed to marshal request"),
			errorkit.WithWrapped(err),
		)
	}

	var response Response
	httpCfg := httprequest.RequestConfig{
		URL:         c.cfg.Endpoint,
		Method:      "POST",
		Body:        body,
		ContentType: "application/json",
		Headers: map[string]string{
			"Authorization": "Bearer " + c.cfg.APIKey,
		},
	}
	if err := c.httpRequest.Do(ctx, httpCfg, &response); err != nil {
		return "", err
	}

	if len(response.Choices) == 0 {
		return "", errorkit.NewError(errorkit.ERR_EXTERNAL_INVALID_RESPONSE).With(
			errorkit.WithReason("LLM response has no choices"),
		)
	}

	return response.Choices[0].Message.Content, nil
}
