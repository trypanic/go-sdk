package llmclient_test

import (
	"context"
	"fmt"

	"github.com/trypanic/go-sdk/httprequest"
	"github.com/trypanic/go-sdk/llmclient"
)

type exampleRequester struct{}

func (exampleRequester) Do(_ context.Context, _ httprequest.RequestConfig, out any) error {
	*out.(*llmclient.Response) = llmclient.Response{Choices: []llmclient.Choice{{Message: llmclient.Message{Content: "Hello"}}}}
	return nil
}
func ExampleNewWithoutTracing() {
	client := llmclient.NewWithoutTracing(exampleRequester{}, llmclient.Config{APIKey: "example-key", Endpoint: "https://example.invalid/chat"})
	response, err := client.Execute(context.Background(), llmclient.RequestConfig{Model: "example-model", UserMessage: "Say hello"})
	fmt.Println(response, err)
	// Output: Hello <nil>
}
