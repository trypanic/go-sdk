// llmclient/client.go

package llmclient

import (
	"context"
	"time"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpclient"
	"github.com/trypanic/go-sdk/httprequest"
	"github.com/trypanic/go-sdk/marshal"
	"github.com/trypanic/go-sdk/telemetry"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LLMRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

type LLMResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index   int     `json:"index"`
	Message Message `json:"message"`
}

type Usage struct {
	CompletionTokens int `json:"completion_tokens"`
	PromptTokens     int `json:"prompt_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type LLMProvider interface {
	Execute(ctx context.Context, reqConfig LLMRequestConfig) (string, error)
}

// LLMRequestConfig holds configuration for individual LLM requests.
// Streaming is intentionally not part of this contract; the SDK does not
// implement chunked responses.
type LLMRequestConfig struct {
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
	UserMessage  string `json:"user_message"`
}

// Config carries the connection settings for an LLM client. It is consumed
// at construction time and never mutated by Client.
type Config struct {
	APIKey   string
	Endpoint string
	Timeout  time.Duration
}

// Client provides LLM interaction capabilities. After construction it is safe
// for concurrent use; its fields are not mutated by Execute.
type Client struct {
	httpRequest httprequest.HTTPRequester
	cfg         Config
}

// NewClient builds a concrete *Client with explicit Config. No tracing wrapper.
func NewClient(httpRequest httprequest.HTTPRequester, cfg Config) *Client {
	return &Client{httpRequest: httpRequest, cfg: cfg}
}

// New builds an LLMProvider wrapped with the global tracer.
func New(httpRequest httprequest.HTTPRequester, cfg Config) LLMProvider {
	return WrapWithTracing(NewClient(httpRequest, cfg))
}

// NewWithoutTracing builds an LLMProvider with no tracing wrapper.
func NewWithoutTracing(httpRequest httprequest.HTTPRequester, cfg Config) LLMProvider {
	return NewClient(httpRequest, cfg)
}

// NewWithInstrumenter builds an LLMProvider wrapped with the supplied
// instrumenter; pass nil to disable tracing.
func NewWithInstrumenter(httpRequest httprequest.HTTPRequester, cfg Config, instrumenter *telemetry.Instrumenter) LLMProvider {
	return WrapWithInstrumenter(NewClient(httpRequest, cfg), instrumenter)
}

// Execute performs a non-streaming LLM request.
func (c *Client) Execute(ctx context.Context, reqConfig LLMRequestConfig) (string, error) {
	if reqConfig.Model == "" {
		return "", errorkit.
			NewError(errorkit.ERR_VALIDATION_MISSING_FIELD).
			With(errorkit.WithReason("model is required"))
	}

	if c.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.cfg.Timeout)
		defer cancel()
	}

	body, err := marshal.NoEscapeBytes(LLMRequest{
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

	var response LLMResponse
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
		return "", errorkit.NewError(httpclient.ERR_EXTERNAL_SERVICE_ERROR).With(
			errorkit.WithReason("no response from LLM"),
		)
	}

	return response.Choices[0].Message.Content, nil
}
