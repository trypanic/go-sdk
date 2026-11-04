// Package hertz is the Hertz adapter for go-sdk/httpserver.
//
// Consumers that import this package get the Hertz framework as a transitive
// dependency. The core go-sdk/httpserver package is framework-agnostic and
// pulls no Hertz code; choose this adapter explicitly.
package hertz

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpserver"
	"github.com/trypanic/go-sdk/logger"
)

// hertzContext implements httpserver.HTTPContext over Hertz's RequestContext.
// ctx is the request context the handler received (never nil: every
// constructor copies the ctx Hertz passes in); it carries the logger and
// the active span used when an error reply is logged.
type hertzContext struct {
	ctx        context.Context
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

// WithError returns the client-safe error option of httpserver.WithErrorOpt
// and, when the reply is built, logs the full error server-side through
// logger.CtxOrGlobal(ctx) with trace correlation. When the error carries no
// TraceID, the reply gets the trace ID of the active span, so clients can
// quote it.
func (h *hertzContext) WithError(err error) httpserver.OptionReply {
	public := httpserver.WithErrorOpt(err)
	return func(r *httpserver.Reply) {
		public(r)
		if err == nil {
			return
		}
		ctx := h.ctx
		logReplyError(ctx, err)
		if r.TraceID == "" {
			r.TraceID = logger.TraceIDFromCtx(ctx)
		}
	}
}

// logReplyError logs an error that is being turned into an HTTP reply:
// at warn level for client errors (4xx) and at error level otherwise.
func logReplyError(ctx context.Context, err error) {
	l := logger.CtxOrGlobal(ctx)
	appErr, ok := errors.AsType[*errorkit.AppError](err)
	if !ok || appErr == nil {
		logger.WithTrace(ctx, l.Error()).Str("error", err.Error()).Msg("httpserver: error reply")
		return
	}
	event := l.Error()
	if status := appErr.Metadata.HTTPStatus; status >= 400 && status < 500 {
		event = l.Warn()
	}
	logger.WithErrorKit(logger.WithTrace(ctx, event), appErr).
		Str("error", err.Error()).
		Msg("httpserver: error reply")
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

// PrettyJSON sends nicely formatted JSON. Adapter-specific helper. A
// marshal failure is logged server-side and answered with a generic 500;
// the marshal error text is not sent to the client.
func (h *hertzContext) PrettyJSON(statusCode int, data any) {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		logReplyError(h.ctx, errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("failed to encode JSON response"),
			errorkit.WithWrapped(err),
		))
		h.JSON(500, map[string]string{
			"error":   "internal_error",
			"message": "failed to encode response",
		})
		return
	}
	h.SetHeader("Content-Type", "application/json")
	h.requestCtx.Data(statusCode, "application/json", bytes)
}
