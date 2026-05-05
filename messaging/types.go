package messaging

import (
	"context"
	"time"
)

// QueueTopology defines the full configuration for a queue as declared in the topology file.
type QueueTopology struct {
	Name       string         `json:"name"`
	Durable    bool           `json:"durable"`
	AutoDelete bool           `json:"autoDelete"`
	TTL        int64          `json:"ttl,omitempty"`
	MaxLength  int            `json:"maxLength,omitempty"`
	Overflow   string         `json:"overflow,omitempty"`
	Prefetch   int            `json:"prefetch,omitempty"`
	QueueMode  string         `json:"queueMode,omitempty"`
	Arguments  map[string]any `json:"arguments,omitempty"`
	DLX        *DLXConfig     `json:"dlx,omitempty"`
	Retry      *RetryConfig   `json:"retry,omitempty"`
}

// DLXConfig configures the dead-letter exchange for a queue.
type DLXConfig struct {
	Exchange   string `json:"exchange"`
	RoutingKey string `json:"routingKey,omitempty"`
}

// RetryConfig defines the exponential-backoff retry policy for a queue.
type RetryConfig struct {
	MaxAttempts  int     `json:"maxAttempts"`
	InitialDelay int64   `json:"initialDelay"` // milliseconds
	MaxDelay     int64   `json:"maxDelay"`     // milliseconds
	BackoffMult  float64 `json:"backoffMultiplier"`
}

// Event is the application-level payload published to a queue.
type Event struct {
	CorrelationID string         `json:"correlation_id"`
	ServiceName   string         `json:"service_name"`
	Payload       map[string]any `json:"payload"`
	Metadata      map[string]any `json:"metadata"`
	OccurredAt    time.Time      `json:"occurred_at"`
}

// EventMessage wraps an Event with envelope metadata added by the publisher.
// The embedded Event fields (including OccurredAt) are promoted directly into
// the JSON output — no duplicate or shadowed timestamp.
type EventMessage struct {
	EventID      string   `json:"event_id"`
	EventVersion string   `json:"event_version"`
	EventType    string   `json:"event_type"`
	RoutingKeys  []string `json:"routing_keys"`
	Event
}

// Message is the raw payload delivered to a subscriber handler.
type Message struct {
	Raw        []byte
	Headers    map[string]any
	Exchange   string
	RoutingKey string
	MessageID  string
}

// MessageHandler processes an incoming message. Return nil to acknowledge;
// return an error to trigger the queue's retry policy.
type MessageHandler func(ctx context.Context, msg Message) error

// Messager is the public contract for publish/subscribe operations.
// Use this interface in function signatures to allow mocking in tests.
type Messager interface {
	// keep old methods for backward compatibility
	Publish(ctx context.Context, queueName string, payload Event) error
	Subscribe(ctx context.Context, queueName string, serviceName string, handler MessageHandler) error
	Close() error
}
