package messagingtest

import (
	"context"

	"github.com/trypanic/go-sdk/messaging"
)

// Broker delegates every operation to a caller-supplied function. An unset
// function panics so unexpected operations cannot silently pass a test.
// Set callbacks before use; concurrent callers require concurrency-safe callbacks.
// No connections or background goroutines are created.
type Broker struct {
	PublishFunc   func(context.Context, string, messaging.Event) error
	SubscribeFunc func(context.Context, string, string, messaging.MessageHandler) error
	CloseFunc     func() error
}

// Publish delegates to the configured callback.
func (m *Broker) Publish(ctx context.Context, queue string, event messaging.Event) error {
	return m.PublishFunc(ctx, queue, event)
}

// Subscribe delegates to the configured callback.
func (m *Broker) Subscribe(ctx context.Context, queue, service string, handler messaging.MessageHandler) error {
	return m.SubscribeFunc(ctx, queue, service, handler)
}

// Close delegates to the configured callback.
func (m *Broker) Close() error { return m.CloseFunc() }

var _ messaging.Broker = (*Broker)(nil)
