package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/trypanic/go-sdk/httprequest"
)

type captureRequester struct {
	cfg          httprequest.RequestConfig
	ctx          context.Context
	responseText string
}

func (c *captureRequester) Do(ctx context.Context, cfg httprequest.RequestConfig, out any) error {
	c.cfg = cfg
	c.ctx = ctx
	resp, ok := out.(*LLMResponse)
	if !ok {
		return nil
	}
	text := c.responseText
	if text == "" {
		text = "ok"
	}
	resp.Choices = []Choice{{Message: Message{Role: "assistant", Content: text}}}
	return nil
}

func TestNewClientExecutePostsToConfiguredEndpointWithBearer(t *testing.T) {
	t.Parallel()

	stub := &captureRequester{}
	c := NewClient(stub, Config{APIKey: "key-123", Endpoint: "http://llm.example/v1/chat"})

	if _, err := c.Execute(context.Background(), LLMRequestConfig{
		Model: "m1", SystemPrompt: "s", UserMessage: "u",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stub.cfg.URL != "http://llm.example/v1/chat" {
		t.Fatalf("URL = %q, want endpoint from Config", stub.cfg.URL)
	}
	if stub.cfg.Method != "POST" {
		t.Fatalf("Method = %q, want POST", stub.cfg.Method)
	}
	if got := stub.cfg.Headers["Authorization"]; got != "Bearer key-123" {
		t.Fatalf("Authorization = %q, want Bearer key-123", got)
	}
}

func TestExecuteAppliesConfiguredTimeoutAsContextDeadline(t *testing.T) {
	t.Parallel()

	stub := &captureRequester{}
	c := NewClient(stub, Config{APIKey: "k", Endpoint: "u", Timeout: 50 * time.Millisecond})

	start := time.Now()
	if _, err := c.Execute(context.Background(), LLMRequestConfig{Model: "m"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	deadline, ok := stub.ctx.Deadline()
	if !ok {
		t.Fatalf("expected ctx deadline when Timeout > 0")
	}
	if d := deadline.Sub(start); d <= 0 || d > 500*time.Millisecond {
		t.Fatalf("deadline offset = %v, want close to 50ms", d)
	}
}

func TestExecuteNoDeadlineWhenTimeoutZero(t *testing.T) {
	t.Parallel()

	stub := &captureRequester{}
	c := NewClient(stub, Config{APIKey: "k", Endpoint: "u"})

	if _, err := c.Execute(context.Background(), LLMRequestConfig{Model: "m"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := stub.ctx.Deadline(); ok {
		t.Fatalf("did not expect ctx deadline when Timeout == 0")
	}
}

func TestExecuteRequiresModel(t *testing.T) {
	t.Parallel()

	stub := &captureRequester{}
	c := NewClient(stub, Config{APIKey: "k", Endpoint: "u"})

	if _, err := c.Execute(context.Background(), LLMRequestConfig{}); err == nil {
		t.Fatalf("expected error when Model is empty")
	}
}

func TestExecutePayloadOmitsStreamField(t *testing.T) {
	t.Parallel()

	stub := &captureRequester{}
	c := NewClient(stub, Config{APIKey: "k", Endpoint: "u"})

	if _, err := c.Execute(context.Background(), LLMRequestConfig{
		Model: "m", SystemPrompt: "s", UserMessage: "u",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bytes.Contains(stub.cfg.Body, []byte(`"stream"`)) {
		t.Fatalf("payload must not include stream field: %s", stub.cfg.Body)
	}

	var payload map[string]any
	if err := json.Unmarshal(stub.cfg.Body, &payload); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if _, present := payload["stream"]; present {
		t.Fatalf("payload must not include stream key: %v", payload)
	}
}

func TestExecuteReturnsResponseContentUnchanged(t *testing.T) {
	t.Parallel()

	stub := &captureRequester{responseText: "```json\n{\"a\":1}\n```"}
	c := NewClient(stub, Config{APIKey: "k", Endpoint: "u"})

	got, err := c.Execute(context.Background(), LLMRequestConfig{Model: "m"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != stub.responseText {
		t.Fatalf("Execute returned %q, want %q (no post-processing)", got, stub.responseText)
	}
}
