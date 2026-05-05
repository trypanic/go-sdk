package httpserver

import (
	"context"
	"time"
)

// WithTimeout returns a derived context with the supplied timeout.
// The returned cancel function MUST be called.
func WithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

// WithCancel returns a derived cancellable context.
func WithCancel(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}

// WithValue returns a derived context carrying key→value.
func WithValue(parent context.Context, key, value any) context.Context {
	return context.WithValue(parent, key, value)
}

// GetValue returns the value associated with key, or nil.
func GetValue(ctx context.Context, key any) any { return ctx.Value(key) }

// Deadline reports whether the context has a deadline and when.
func Deadline(ctx context.Context) (time.Time, bool) { return ctx.Deadline() }

// Done returns the channel that closes when the context is cancelled.
func Done(ctx context.Context) <-chan struct{} { return ctx.Done() }
