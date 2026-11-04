package httprequest_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/trypanic/go-sdk/v2/httprequest"
)

type exampleTransport struct{}

func (exampleTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"name":"Ada"}`))}, nil
}
func ExampleNewWithoutTracing() {
	// An injected transport makes the example executable without a service.
	client := httprequest.NewWithoutTracing(&http.Client{Transport: exampleTransport{}})
	var user struct{ Name string }
	err := client.Do(context.Background(), httprequest.RequestConfig{Method: http.MethodGet, URL: "https://example.invalid/users/1"}, &user)
	fmt.Println(user.Name, err)
	// Output: Ada <nil>
}
