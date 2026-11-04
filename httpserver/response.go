package httpserver

import (
	"errors"

	"github.com/trypanic/go-sdk/errorkit"
)

type OptionReply func(*Reply)

// Reply is the JSON envelope written by HTTPContext.Reply. Code and Error
// are client-safe: they never carry err.Error() text (see WithErrorOpt).
type Reply struct {
	Message   *string `json:"message,omitempty"`
	Code      string  `json:"code,omitempty"`
	Error     *string `json:"error,omitempty"`
	TraceID   string  `json:"trace_id,omitempty"`
	Timestamp string  `json:"timestamp,omitempty"`
	Metadata  any     `json:"metadata,omitempty"`
	Data      any     `json:"data,omitempty"`
}

// WithMessageOpt returns an OptionReply that sets Reply.Message.
// Framework-agnostic equivalent of HTTPContext.WithMessage.
func WithMessageOpt(message string) OptionReply {
	return func(r *Reply) { r.Message = &message }
}

// genericErrorMessage is the public message for errors that carry no
// client-safe text.
const genericErrorMessage = "Internal server error"

// WithErrorOpt returns an OptionReply that sets a client-safe Reply.Code and
// Reply.Error, and copies the errorkit TraceID into Reply.TraceID when the
// error carries one.
//
// The text of err is never sent, because it can hold SQL, hostnames, file
// paths or panic values:
//   - *errorkit.AppError (anywhere in the chain): Code is its error code.
//     Error is its Reason for client errors (Metadata.HTTPStatus 4xx) and
//     its Metadata.Description otherwise.
//   - any other error: Code is ERR_INTERNAL and Error is a generic message.
//
// Log the full error server-side; the Hertz adapter's HTTPContext.WithError
// does that with trace correlation.
func WithErrorOpt(err error) OptionReply {
	return func(r *Reply) {
		if err == nil {
			return
		}
		code, message := publicError(err)
		r.Code = code
		r.Error = &message
		var appErr *errorkit.AppError
		if errors.As(err, &appErr) && appErr.TraceID != "" {
			r.TraceID = appErr.TraceID
		}
	}
}

// publicError returns the client-safe code and message for err.
func publicError(err error) (code, message string) {
	var appErr *errorkit.AppError
	if !errors.As(err, &appErr) || appErr == nil {
		return string(errorkit.ERR_INTERNAL), genericErrorMessage
	}
	status := appErr.Metadata.HTTPStatus
	switch {
	case status >= 400 && status < 500 && appErr.Reason != "":
		message = appErr.Reason
	case appErr.Metadata.Description != "":
		message = appErr.Metadata.Description
	default:
		message = genericErrorMessage
	}
	return string(appErr.Code()), message
}

// WithMetadataOpt returns an OptionReply that sets Reply.Metadata.
func WithMetadataOpt(metadata any) OptionReply {
	return func(r *Reply) { r.Metadata = &metadata }
}

// WithDataOpt returns an OptionReply that sets Reply.Data.
func WithDataOpt(data any) OptionReply {
	return func(r *Reply) { r.Data = &data }
}

// BuildReply produces a Reply by stamping the timestamp from ReplyOptions
// and applying each OptionReply in order. Pure: no side effects.
// Adapter packages call this to share a single reply construction path.
func BuildReply(o ReplyOptions, opts ...OptionReply) *Reply {
	r := &Reply{}
	if o.Clock != nil && o.Layout != "" {
		r.Timestamp = o.Clock().Format(o.Layout)
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// buildReply is the unexported alias used by package tests.
func buildReply(o ReplyOptions, opts ...OptionReply) *Reply { return BuildReply(o, opts...) }
