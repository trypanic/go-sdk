package errorkit

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ==============================
// Stack Formatter
// ==============================

// StackFormatter formats errors as stack traces for terminal output
// This formatter ONLY returns formatted strings - it never prints to console.
type StackFormatter struct {
	colorsEnabled *bool
}

// FormatterConfig controls formatter behavior without mutating package globals.
type FormatterConfig struct {
	ColorsEnabled bool
}

// NewStackFormatter creates a new stack trace formatter
func NewStackFormatter() *StackFormatter {
	return &StackFormatter{}
}

// NewStackFormatterWithConfig creates a formatter using explicit color config.
func NewStackFormatterWithConfig(config FormatterConfig) *StackFormatter {
	return &StackFormatter{
		colorsEnabled: &config.ColorsEnabled,
	}
}

// Format formats the error with automatic TTY detection for colors.
// Returns a formatted string - does NOT print to console.
func (f *StackFormatter) Format(e *AppError) string {
	if f != nil && f.colorsEnabled != nil {
		if *f.colorsEnabled {
			return f.FormatColored(e)
		}
		return f.FormatPlain(e)
	}
	if isTerminal(os.Stdout) {
		return f.FormatColored(e)
	}
	return f.FormatPlain(e)
}

// FormatColored formats the error with ANSI colors.
// Returns a formatted string - does NOT print to console.
func (f *StackFormatter) FormatColored(e *AppError) string {
	if e == nil {
		return ""
	}

	builder := &strings.Builder{}

	// Header
	builder.WriteString("\n")
	fmt.Fprintf(builder, "%s %s %s\n",
		f.colorize(bold+red, "✘ ERROR"),
		f.colorize(cyan, string(e.ErrCode)),
		f.colorize(yellow, e.Metadata.Description),
	)

	// Reason
	if e.Reason != "" {
		fmt.Fprintf(builder, "  %s %s\n",
			f.colorize(purple, "Reason:"),
			e.Reason,
		)
	}

	// Wrapped
	if e.Wrapped != nil {
		fmt.Fprintf(builder, "  %s %s\n",
			f.colorize(purple, "Wrapped:"),
			e.Unwrap(),
		)
	}

	if e.Payload != nil {
		fmt.Fprintf(builder, "  %s %v\n",
			f.colorize(cyan, "Payload:"),
			e.Payload,
		)
	}

	// Error ID
	fmt.Fprintf(builder, "  %s %s\n",
		f.colorize(purple, "ErrorID:"),
		e.ID,
	)

	// Trace ID (only if present)
	if e.TraceID != "" {
		fmt.Fprintf(builder, "  %s %s\n",
			f.colorize(purple, "TraceID:"),
			e.TraceID,
		)
	}

	// Stack Trace
	builder.WriteString("\n")
	builder.WriteString(f.colorize(bold+blue, "Stack Trace:"))
	builder.WriteString("\n")

	for i, frame := range e.Trace {
		fmt.Fprintf(builder,
			"%d:%s:%s:%s:%d\n",
			i,
			f.colorize(red, frame.Package),
			f.colorize(yellow, frame.Function),
			f.colorize(cyan, frame.File),
			frame.Line,
		)
	}

	builder.WriteString("\n")

	return builder.String()
}

func (f *StackFormatter) colorize(code, text string) string {
	if f != nil && f.colorsEnabled != nil && !*f.colorsEnabled {
		return text
	}
	return colorize(code, text)
}

// FormatPlain formats the error without colors.
// Returns a formatted string - does NOT print to console.
func (f *StackFormatter) FormatPlain(e *AppError) string {
	if e == nil {
		return ""
	}

	builder := &strings.Builder{}

	// Header
	builder.WriteString("\n")
	fmt.Fprintf(builder, "✘ ERROR [%s] %s\n", e.ErrCode, e.Metadata.Description)

	// Reason
	if e.Reason != "" {
		fmt.Fprintf(builder, "  Reason: %s\n", e.Reason)
	}

	// Wrapped
	if e.Wrapped != nil {
		fmt.Fprintf(builder, "  Wrapped: %s\n", e.Wrapped.Error())
	}

	// Error ID
	fmt.Fprintf(builder, "  ErrorID: %s\n", e.ID)

	// Trace ID (only if present)
	if e.TraceID != "" {
		fmt.Fprintf(builder, "  TraceID: %s\n", e.TraceID)
	}

	// Stack Trace
	builder.WriteString("\nStack Trace:\n")

	for i, frame := range e.Trace {
		fmt.Fprintf(builder,
			"%d:%s:%s:%s:%d\n",
			i,
			frame.Package,
			frame.Function,
			frame.File,
			frame.Line,
		)
	}

	builder.WriteString("\n")

	return builder.String()
}

// ==============================
// JSON Formatter
// ==============================

// JSONFormatter formats errors as JSON for logging and monitoring systems
// This formatter ONLY returns JSON strings - it never prints to console.
type JSONFormatter struct{}

// NewJSONFormatter creates a new JSON formatter
func NewJSONFormatter() *JSONFormatter {
	return &JSONFormatter{}
}

// Format formats the error as pretty-printed JSON.
// Returns a JSON string - does NOT print to console.
func (f *JSONFormatter) Format(e *AppError) string {
	if e == nil {
		return "{}"
	}

	// Create JSON-serializable model
	type wrappedError struct {
		Message string `json:"message"`
	}

	model := struct {
		Code     ErrorCode      `json:"code"`
		Reason   string         `json:"reason,omitempty"`
		Payload  any            `json:"payload,omitempty"`
		Metadata Metadata       `json:"metadata"`
		Trace    []TraceContext `json:"stack_trace"`
		ID       string         `json:"id"`
		TraceID  string         `json:"trace_id,omitempty"`
		Wrapped  *wrappedError  `json:"wrapped,omitempty"`
	}{
		Code:     e.ErrCode,
		Reason:   e.Reason,
		Payload:  e.Payload,
		Metadata: e.Metadata,
		Trace:    e.Trace,
		ID:       e.ID,
		TraceID:  e.TraceID,
	}

	// Add wrapped error if present
	if e.Wrapped != nil {
		model.Wrapped = &wrappedError{
			Message: e.Wrapped.Error(),
		}
	}

	b, err := json.MarshalIndent(model, "", "  ")
	if err != nil {
		return marshalFailedJSON(e, err)
	}

	return string(b)
}

// marshalFailedJSON is the fallback document when the full model cannot be
// encoded (for example a payload holding a channel or a func). It is built
// with encoding/json so free-form codes cannot break the JSON.
func marshalFailedJSON(e *AppError, cause error) string {
	fallback := struct {
		Error  string    `json:"error"`
		ID     string    `json:"id"`
		Code   ErrorCode `json:"code"`
		Detail string    `json:"detail"`
	}{
		Error:  "marshal_failed",
		ID:     e.ID,
		Code:   e.ErrCode,
		Detail: cause.Error(),
	}
	// Only strings: marshalling cannot fail.
	b, _ := json.Marshal(fallback)
	return string(b)
}

// ==============================
// Helper: TTY Detection
// ==============================

// isTerminal checks if the given file is a terminal
func isTerminal(f *os.File) bool {
	fileInfo, err := f.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}
