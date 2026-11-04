package errorkit

import (
	"fmt"
	"io"
	"os"
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

// NewWithoutStack creates an AppError without collecting caller frames. Use it
// for expected rejection paths such as input validation. Metadata, identity,
// timestamp and error-chain behavior are identical to NewError.
func NewWithoutStack(code ErrorCode) *AppError {
	meta, exists := GetMetadata(code)
	if !exists {
		meta = unknownMetadata(code)
	}
	return newAppErrorBase(code, meta)
}

func newAppError(code ErrorCode, meta Metadata) *AppError {
	return newAppErrorWithStackDepth(code, meta, currentMaxStackDepth())
}

func newAppErrorWithStackDepth(code ErrorCode, meta Metadata, maxStackDepth int) *AppError {
	e := newAppErrorBase(code, meta)
	e.Trace = captureStackWithDepth(maxStackDepth)
	return e
}

func newAppErrorBase(code ErrorCode, meta Metadata) *AppError {
	return &AppError{
		ErrCode:   code,
		Metadata:  meta,
		ID:        generateID(),
		Timestamp: nowISO8601(),
	}
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
// - Logs and monitoring: err.PrettyJSON()
// - HTTP responses: w.Write([]byte(err.PublicJSON())) (see public.go)
// - Logger integration: logger.WithErrorKit()

// Pretty returns a human-readable formatted string, with ANSI colors when
// stdout is a terminal. Use for manual debugging or console tools; use
// WriteTo to pick colors from the destination instead.
// This method DOES NOT print - it only returns a string.
func (e *AppError) Pretty() string {
	if e == nil {
		return ""
	}

	formatter := NewStackFormatter()
	return formatter.Format(e)
}

// PrettyJSON returns the error formatted as JSON, including the stack trace,
// payload and wrapped error text. Use it for logs and monitoring only: it
// exposes internal details and must not be sent to clients (use PublicJSON).
// This method DOES NOT print - it only returns a string.
func (e *AppError) PrettyJSON() string {
	if e == nil {
		return "{}"
	}
	return NewJSONFormatter().Format(e)
}

// WriteTo writes the formatted error to the given writer.
// Useful for direct logging to files or streams.
//
// ANSI colors are used only when w itself is a terminal (an *os.File for a
// character device) and colors are not disabled; files, sockets and buffers
// get plain text whatever the state of stdout.
func (e *AppError) WriteTo(w io.Writer) (int64, error) {
	if e == nil {
		return 0, nil
	}

	file, isFile := w.(*os.File)
	formatter := NewStackFormatterWithConfig(FormatterConfig{
		ColorsEnabled: isFile && isTerminal(file),
	})

	n, err := io.WriteString(w, formatter.Format(e))
	return int64(n), err
}
