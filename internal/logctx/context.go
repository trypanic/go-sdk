// Package logctx shares context lookup and trace fields without importing exporters.
package logctx

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

// Global stores the SDK logger. Published loggers must not be mutated.
var Global atomic.Pointer[zerolog.Logger]
var fallback = sync.OnceValue(func() *zerolog.Logger {
	l := zerolog.New(os.Stderr).Hook(timestampHook{})
	return &l
})

// GlobalLogger returns the configured logger or the startup fallback.
func GlobalLogger() *zerolog.Logger {
	if l := Global.Load(); l != nil {
		return l
	}
	return fallback()
}

// CtxOrGlobal returns the context logger unless it is disabled.
func CtxOrGlobal(ctx context.Context) *zerolog.Logger {
	l := zerolog.Ctx(ctx)
	if l.GetLevel() == zerolog.Disabled {
		return GlobalLogger()
	}
	return l
}

// WithTrace attaches the current span identifiers to an event.
func WithTrace(ctx context.Context, event *zerolog.Event) *zerolog.Event {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if sc.HasTraceID() {
		event = event.Str("trace_id", sc.TraceID().String())
	}
	if sc.HasSpanID() {
		event = event.Str("span_id", sc.SpanID().String())
	}
	return event
}

type timestampHook struct{}

func (timestampHook) Run(event *zerolog.Event, _ zerolog.Level, _ string) {
	var buf [64]byte
	event.Bytes(zerolog.TimestampFieldName, zerolog.TimestampFunc().UTC().AppendFormat(buf[:0], time.RFC3339Nano))
}
