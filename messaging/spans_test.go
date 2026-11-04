package messaging

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func spanByName(t *testing.T, rec *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	var names []string
	for _, s := range rec.Ended() {
		if s.Name() == name {
			return s
		}
		names = append(names, s.Name())
	}
	t.Fatalf("no span %q; recorded %v", name, names)
	return nil
}

func spanAttrs(s sdktrace.ReadOnlySpan) map[attribute.Key]attribute.Value {
	m := map[attribute.Key]attribute.Value{}
	for _, kv := range s.Attributes() {
		m[kv.Key] = kv.Value
	}
	return m
}

// deprecatedKeys are the pre-1.26 semconv keys the package used to emit.
var deprecatedKeys = []attribute.Key{"messaging.destination", "messaging.operation", "messaging.rabbitmq.routing_key"}

// C5-017: the producer span follows the telemetry.Messaging naming
// convention and the current semconv messaging keys.
func TestPublishSpanConventions(t *testing.T) {
	t.Parallel()

	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	b := newFakeBroker()
	m, err := newPubSub("amqp://fake", b.dial, WithTopology(testTopology()), WithTracer(tp.Tracer("test")))
	require.NoError(t, err)
	t.Cleanup(func() { _ = m.Close() })

	require.NoError(t, m.Publish(context.Background(), "q", Event{}))

	span := spanByName(t, rec, "messaging.publish.q")
	assert.Equal(t, trace.SpanKindProducer, span.SpanKind())
	attrs := spanAttrs(span)
	assert.Equal(t, "rabbitmq", attrs["messaging.system"].AsString())
	assert.Equal(t, "q", attrs["messaging.destination.name"].AsString())
	assert.Equal(t, "send", attrs["messaging.operation.type"].AsString())
	assert.Equal(t, "publish", attrs["messaging.operation.name"].AsString())
	assert.Equal(t, "events", attrs["messaging.rabbitmq.exchange"].AsString())
	for _, k := range deprecatedKeys {
		assert.NotContains(t, attrs, k)
	}
}

// C5-017: the consumer spans follow the same conventions.
func TestDeliverySpanConventions(t *testing.T) {
	t.Parallel()

	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	p := &PubSub{tracer: tp.Tracer("test")}
	d := newTestDelivery(&fakeAcknowledger{})
	d.MessageId = "m-1"

	p.handleDelivery(context.Background(), "q", d, QueueTopology{Name: "q"},
		func(context.Context, Message) error { return nil })

	recv := spanByName(t, rec, "messaging.receive.q")
	assert.Equal(t, trace.SpanKindConsumer, recv.SpanKind())
	attrs := spanAttrs(recv)
	assert.Equal(t, "rabbitmq", attrs["messaging.system"].AsString())
	assert.Equal(t, "q", attrs["messaging.destination.name"].AsString())
	assert.Equal(t, "receive", attrs["messaging.operation.type"].AsString())
	assert.Equal(t, "order.created", attrs["messaging.rabbitmq.destination.routing_key"].AsString())
	assert.Equal(t, "m-1", attrs["messaging.message.id"].AsString())
	for _, k := range deprecatedKeys {
		assert.NotContains(t, attrs, k)
	}

	proc := spanByName(t, rec, "messaging.process.q")
	pattrs := spanAttrs(proc)
	assert.Equal(t, "process", pattrs["messaging.operation.type"].AsString())
	assert.Equal(t, "q", pattrs["messaging.destination.name"].AsString())
	assert.Equal(t, int64(1), pattrs["retry.attempt"].AsInt64())
}
