// Package hertz is the Hertz adapter for go-sdk/httpserver.
//
// Consumers that import this package get the Hertz framework as a transitive
// dependency. The core go-sdk/httpserver package is framework-agnostic and
// pulls no Hertz code; choose this adapter explicitly.
package hertz

import (
	"encoding/json"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpserver"
)

// hertzContext implements httpserver.HTTPContext over Hertz's RequestContext.
type hertzContext struct {
	requestCtx *app.RequestContext
	replyOpts  httpserver.ReplyOptions
}

func (h *hertzContext) Param(key string) string {
	return h.requestCtx.Param(key)
}

func (h *hertzContext) Query(key string) string {
	return string(h.requestCtx.Query(key))
}

func (h *hertzContext) BindJSON(obj any) error {
	if err := h.requestCtx.BindJSON(obj); err != nil {
		return errorkit.NewError(errorkit.ERR_CLIENT_BAD_REQUEST).
			With(errorkit.WithWrapped(err))
	}
	return nil
}

func (h *hertzContext) GetBody() []byte { return h.requestCtx.Request.Body() }

func (h *hertzContext) GetHeader(key string) string {
	return string(h.requestCtx.Request.Header.Peek(key))
}

func (h *hertzContext) JSON(statusCode int, data any) {
	h.requestCtx.JSON(statusCode, data)
}

func (h *hertzContext) String(statusCode int, message string) {
	h.requestCtx.String(statusCode, message)
}

func (h *hertzContext) Status(statusCode int) { h.requestCtx.Status(statusCode) }

func (h *hertzContext) SetHeader(key, value string) {
	h.requestCtx.Response.Header.Set(key, value)
}

func (h *hertzContext) Redirect(statusCode int, location []byte) {
	h.requestCtx.Redirect(statusCode, location)
}

func (h *hertzContext) WithMessage(message string) httpserver.OptionReply {
	return httpserver.WithMessageOpt(message)
}

func (h *hertzContext) WithError(err error) httpserver.OptionReply {
	return httpserver.WithErrorOpt(err)
}

func (h *hertzContext) WithMetadata(metadata any) httpserver.OptionReply {
	return httpserver.WithMetadataOpt(metadata)
}

func (h *hertzContext) WithData(data any) httpserver.OptionReply {
	return httpserver.WithDataOpt(data)
}

func (h *hertzContext) Reply(status int, options ...httpserver.OptionReply) {
	r := httpserver.BuildReply(h.replyOpts, options...)
	h.JSON(status, r)
}

// PrettyJSON sends nicely formatted JSON. Adapter-specific helper.
func (h *hertzContext) PrettyJSON(statusCode int, data any) {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		h.JSON(500, map[string]string{
			"error":   "internal_error",
			"message": err.Error(),
		})
		return
	}
	h.SetHeader("Content-Type", "application/json")
	h.requestCtx.Data(statusCode, "application/json", bytes)
}
