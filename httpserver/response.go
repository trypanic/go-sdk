package httpserver

import (
	"github.com/trypanic/go-sdk/errorkit"
)

// OptionReply sets one field of a Reply; see BuildReply.
type OptionReply func(*Reply)

// Reply is the JSON envelope written by Context.Reply. Code and Error
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
// Framework-agnostic equivalent of Context.WithMessage.
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
// Log the full error server-side; the Hertz adapter's Context.WithError
// does that with trace correlation.
func WithErrorOpt(err error) OptionReply {
	return func(r *Reply) {
		if err == nil {
			return
		}
		code, message := publicError(err)
		r.Code = code
		r.Error = &message
		if appErr, ok := errorkit.As(err); ok && appErr.TraceID != "" {
			r.TraceID = appErr.TraceID
		}
	}
}

// publicError returns the client-safe code and message for err.
func publicError(err error) (code, message string) {
	appErr, ok := errorkit.As(err)
	if !ok || appErr == nil {
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

// WithMetadataOpt returns an OptionReply that sets Reply.Metadata to
// metadata as given; nil is omitted from the JSON.
func WithMetadataOpt(metadata any) OptionReply {
	return func(r *Reply) { r.Metadata = metadata }
}

// WithDataOpt returns an OptionReply that sets Reply.Data to data as given;
// nil is omitted from the JSON.
func WithDataOpt(data any) OptionReply {
	return func(r *Reply) { r.Data = data }
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

// StatusFor returns an AppError's registered HTTP failure status, including
// through wrappers. Nil means 200; unknown errors or invalid statuses mean 500.
func StatusFor(err error) int {
	if err == nil {
		return 200
	}
	if appErr, ok := errorkit.As(err); ok {
		if status := appErr.Metadata.HTTPStatus; status >= 400 && status <= 599 {
			return status
		}
	}
	return 500
}

// ReplyError writes a metadata-driven, client-safe error response. A nil error
// does not write a response. The adapter's WithError hook retains its logging.
func ReplyError(c Context, err error) {
	if err == nil {
		return
	}
	c.Reply(StatusFor(err), c.WithError(err))
}
