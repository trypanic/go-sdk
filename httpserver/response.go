package httpserver

import (
	"errors"

	"github.com/trypanic/go-sdk/errorkit"
)

type OptionReply func(*Reply)

type Reply struct {
	Message   *string `json:"message,omitempty"`
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

// WithErrorOpt returns an OptionReply that sets Reply.Error and copies the
// errorkit TraceID into Reply.TraceID when the underlying error carries one.
func WithErrorOpt(err error) OptionReply {
	return func(r *Reply) {
		if err == nil {
			return
		}
		s := err.Error()
		r.Error = &s
		var appErr *errorkit.AppError
		if errors.As(err, &appErr) && appErr.TraceID != "" {
			r.TraceID = appErr.TraceID
		}
	}
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
