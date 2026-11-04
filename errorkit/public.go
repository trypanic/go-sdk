package errorkit

import (
	"encoding/json"
	"net/http"
)

// PublicError is the client-safe view of an AppError. It carries only what a
// caller outside the service may see: the error code, a stable message, the
// error ID (to quote in support requests) and the distributed trace ID.
//
// It never contains the stack trace, the payload, the wrapped error or, for
// server-side failures, the reason. Use it (or PublicJSON) for HTTP and RPC
// responses; keep PrettyJSON for logs.
type PublicError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	ID      string    `json:"id"`
	TraceID string    `json:"trace_id,omitempty"`
}

// Public returns the client-safe view of the error.
//
// The message is the Reason when the error belongs to a client-error class
// (Metadata.HTTPStatus 4xx) and a reason is set, because such reasons describe
// the caller's own input. For every other class the Reason may hold internal
// details, so the message is Metadata.Description, or the standard status text
// when the description is empty.
//
// A nil receiver returns the zero PublicError.
func (e *AppError) Public() PublicError {
	if e == nil {
		return PublicError{}
	}
	return PublicError{
		Code:    e.ErrCode,
		Message: e.publicMessage(),
		ID:      e.ID,
		TraceID: e.TraceID,
	}
}

// PublicJSON returns Public() encoded as compact JSON. It is the serializer to
// use for HTTP response bodies:
//
//	w.WriteHeader(appErr.Metadata.HTTPStatus)
//	w.Write([]byte(appErr.PublicJSON()))
//
// A nil receiver returns "{}".
func (e *AppError) PublicJSON() string {
	if e == nil {
		return "{}"
	}
	// PublicError holds only strings, so marshalling cannot fail.
	b, _ := json.Marshal(e.Public())
	return string(b)
}

func (e *AppError) publicMessage() string {
	status := e.Metadata.HTTPStatus
	if status >= http.StatusBadRequest && status < http.StatusInternalServerError && e.Reason != "" {
		return e.Reason
	}
	if e.Metadata.Description != "" {
		return e.Metadata.Description
	}
	if text := http.StatusText(status); text != "" {
		return text
	}
	return http.StatusText(http.StatusInternalServerError)
}
