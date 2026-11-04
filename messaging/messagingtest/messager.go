// Package messagingtest provides callback-based messaging test doubles.
package messagingtest

import (
	"context"

	"github.com/trypanic/go-sdk/messaging"
)

// Messager delegates every operation to a caller-supplied function. An unset
// function panics so unexpected operations cannot silently pass a test.
// Set callbacks before use; concurrent callers require concurrency-safe callbacks.
// No connections or background goroutines are created.
type Messager struct {
	PublishFunc   func(context.Context, string, messaging.Event) error
	SubscribeFunc func(context.Context, string, string, messaging.MessageHandler) error
	CloseFunc     func() error
}

func (m *Messager) Publish(ctx context.Context, queue string, event messaging.Event) error {
	return m.PublishFunc(ctx, queue, event)
}
func (m *Messager) Subscribe(ctx context.Context, queue, service string, handler messaging.MessageHandler) error {
	return m.SubscribeFunc(ctx, queue, service, handler)
}
func (m *Messager) Close() error { return m.CloseFunc() }

var _ messaging.Messager = (*Messager)(nil)
