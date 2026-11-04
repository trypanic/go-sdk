package httprequest

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/trypanic/go-sdk/v2/errorkit"
)

type oneByteReader struct{ io.Reader }

func (r oneByteReader) Read(p []byte) (int, error) { return r.Reader.Read(p[:min(len(p), 1)]) }

type chunkedRoundTrip func(*http.Request) (*http.Response, error)

func (r chunkedRoundTrip) RoundTrip(q *http.Request) (*http.Response, error) { return r(q) }
func TestChunkedJSONTrailingLimit(t *testing.T) {
	client := &http.Client{Transport: chunkedRoundTrip(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Request: req, Header: make(http.Header), Body: io.NopCloser(oneByteReader{strings.NewReader(`{"ok":true}` + strings.Repeat(" ", 4096))})}, nil
	})}
	request := NewWithOptions(client, WithMaxResponseBytes(15))
	var result map[string]any
	err := request.Do(t.Context(), RequestConfig{URL: "https://example.invalid"}, &result)
	app, ok := errorkit.As(err)
	if !ok || app.Code() != errorkit.ERR_EXTERNAL_INVALID_RESPONSE || app.Metadata.Retriable {
		t.Fatalf("expected non-retriable oversize error; got %#v", err)
	}
}
