// Package httpservertest provides a framework-free handler recorder.
package httpservertest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/trypanic/go-sdk/httpserver"
)

// Context implements Context using an HTTP request and response recorder.
// Like an HTTP handler, it is confined to one goroutine. Configure Params and
// ReplyOptions before calling a handler. Inspect Response afterwards.
type Context struct {
	Request      *http.Request
	Response     *httptest.ResponseRecorder
	Params       map[string]string
	ReplyOptions httpserver.ReplyOptions
	// Err records body-read or response-encoding failures. Assert it after the handler.
	Err error
}

// NewContext creates an empty response recorder for req. A nil request uses GET /.
func NewContext(req *http.Request) *Context {
	if req == nil {
		req = httptest.NewRequest(http.MethodGet, "/", nil)
	}
	return &Context{Request: req, Response: httptest.NewRecorder(), Params: make(map[string]string)}
}
func (c *Context) Param(key string) string { return c.Params[key] }
func (c *Context) Query(key string) string { return c.Request.URL.Query().Get(key) }
func (c *Context) BindJSON(out any) error {
	body := c.GetBody()
	if c.Err != nil {
		return c.Err
	}
	return json.Unmarshal(body, out)
}
func (c *Context) GetBody() []byte {
	if c.Request.Body == nil {
		return nil
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Err = err
		return nil
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return body
}
func (c *Context) GetHeader(key string) string { return c.Request.Header.Get(key) }
func (c *Context) JSON(status int, data any) {
	body, err := json.Marshal(data)
	if err != nil {
		c.Err = err
		c.Response.WriteHeader(http.StatusInternalServerError)
		return
	}
	c.SetHeader("Content-Type", "application/json; charset=utf-8")
	c.Response.WriteHeader(status)
	_, _ = c.Response.Write(body)
}
func (c *Context) String(status int, message string) {
	c.SetHeader("Content-Type", "text/plain; charset=utf-8")
	c.Response.WriteHeader(status)
	_, _ = c.Response.WriteString(message)
}
func (c *Context) Status(status int)           { c.Response.WriteHeader(status) }
func (c *Context) SetHeader(key, value string) { c.Response.Header().Set(key, value) }
func (c *Context) Redirect(status int, location []byte) {
	c.SetHeader("Location", string(location))
	c.Status(status)
}
func (c *Context) Reply(status int, opts ...httpserver.OptionReply) {
	c.JSON(status, httpserver.BuildReply(c.ReplyOptions, opts...))
}
func (c *Context) WithMessage(message string) httpserver.OptionReply {
	return httpserver.WithMessageOpt(message)
}
func (c *Context) WithError(err error) httpserver.OptionReply { return httpserver.WithErrorOpt(err) }
func (c *Context) WithMetadata(value any) httpserver.OptionReply {
	return httpserver.WithMetadataOpt(value)
}
func (c *Context) WithData(value any) httpserver.OptionReply { return httpserver.WithDataOpt(value) }

var _ httpserver.Context = (*Context)(nil)
