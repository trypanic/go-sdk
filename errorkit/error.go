package errorkit

import (
	"fmt"
	"io"
)

// ==============================
// Models
// ==============================

// TraceContext represents a single frame in the stack trace
type TraceContext struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Package  string `json:"package"`
	Function string `json:"function"`
}

// Metadata holds error classification and behavior information
type Metadata struct {
	Code        ErrorCode  `json:"code"`
	Type        ErrorType  `json:"type"`
	Group       ErrorGroup `json:"group"`
	Category    string     `json:"category"`
	Description string     `json:"description"`
	HTTPStatus  int        `json:"http_status"`
	Retriable   bool       `json:"retriable"`
}

// AppError represents a structured application error with stack trace
type AppError struct {
	ErrCode   ErrorCode      `json:"code"`
	Reason    string         `json:"reason"`
	Payload   any            `json:"payload"`
	Wrapped   error          `json:"-"`
	Metadata  Metadata       `json:"metadata"`
	Trace     []TraceContext `json:"stack_trace"`
	ID        string         `json:"id"`
	TraceID   string         `json:"trace_id"`
	Timestamp string         `json:"timestamp"`
}

// ErrorOption is a functional option for configuring AppError
type ErrorOption func(*AppError)

// ==============================
// Error Creation
// ==============================

// NewError creates a new AppError instance and captures stack trace automatically
func NewError(code ErrorCode) *AppError {
	// Fetch metadata
	meta, exists := GetMetadata(code)
	if !exists {
		// Use default metadata for unknown codes
		// No console output - just use defaults
		meta = unknownMetadata(code)
	}

	return newAppError(code, meta)
}

func newAppError(code ErrorCode, meta Metadata) *AppError {
	return newAppErrorWithStackDepth(code, meta, MaxStackDepth)
}

func newAppErrorWithStackDepth(code ErrorCode, meta Metadata, maxStackDepth int) *AppError {
	e := &AppError{
		ErrCode:   code,
		Metadata:  meta,
		ID:        GenerateID(),
		TraceID:   "", // Empty for now, will be populated in distributed systems
		Timestamp: nowISO8601(),
	}

	// Automatic stack capture
	e.Trace = captureStackWithDepth(skipNewErrorFrames, maxStackDepth)

	return e
}

// ==============================
// Error Interface
// ==============================

// Error returns a simple string representation of the error.
// This implements the error interface.
// Returns a concise one-line description suitable for logs and error messages.
func (e *AppError) Error() string {
	if e == nil {
		return "nil error"
	}

	// Simple format: [CODE] reason
	// This prevents duplicate output when used with structured loggers
	if e.Reason != "" {
		return fmt.Sprintf("[%s] %s", e.ErrCode, e.Reason)
	}
	return fmt.Sprintf("[%s] %s", e.ErrCode, e.Metadata.Description)
}

// Unwrap returns the wrapped error for errors.Unwrap compatibility
func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Wrapped
}

// Code returns the error code
func (e *AppError) Code() ErrorCode {
	if e == nil {
		return ERR_UNKNOWN
	}
	return e.ErrCode
}

// ==============================
// Functional Options
// ==============================

// With applies functional options to the error
// This method modifies the error in-place and returns it for chaining
func (e *AppError) With(opts ...ErrorOption) *AppError {
	if e == nil {
		return nil
	}

	for _, opt := range opts {
		opt(e)
	}
	return e
}

// WithPayload sets the payload data for the error
// Payload can be any type - it will be serialized to JSON when formatted
func WithPayload(payload any) ErrorOption {
	return func(e *AppError) {
		e.Payload = payload
	}
}

// WithReason sets a human-readable reason for the error
func WithReason(format string, args ...any) ErrorOption {
	return func(e *AppError) {
		e.Reason = fmt.Sprintf(format, args...)
	}
}

// WithWrapped wraps another error
// Note: Wrapping does NOT capture stack again - preserves the original stack trace
func WithWrapped(err error) ErrorOption {
	return func(e *AppError) {
		e.Wrapped = err
	}
}

// WithTraceID sets the distributed tracing ID
// This should be populated from context in distributed systems
func WithTraceID(traceID string) ErrorOption {
	return func(e *AppError) {
		e.TraceID = traceID
	}
}

// ==============================
// String Formatting Methods
// ==============================
// These methods ONLY return formatted strings.
// They never print to console.
// Use them for:
// - Manual debugging: fmt.Println(err.Pretty())
// - HTTP responses: w.Write([]byte(err.PrettyJSON()))
// - Logger integration: logger.WithErrorKit()

// Pretty returns a human-readable formatted string.
// Use for manual debugging or console tools.
// This method DOES NOT print - it only returns a string.
func (e *AppError) Pretty() string {
	if e == nil {
		return ""
	}

	formatter := NewStackFormatter()
	return formatter.Format(e)
}

// PrettyJSON returns the error formatted as JSON.
// Use for HTTP responses or structured logging.
// This method DOES NOT print - it only returns a string.
func (e *AppError) PrettyJSON() string {
	if e == nil {
		return "{}"
	}
	return NewJSONFormatter().Format(e)
}

// WriteTo writes the formatted error to the given writer
// Useful for direct logging to files or streams
func (e *AppError) WriteTo(w io.Writer) (int64, error) {
	if e == nil {
		return 0, nil
	}

	output := e.Pretty()

	n, err := w.Write([]byte(output))
	return int64(n), err
}
