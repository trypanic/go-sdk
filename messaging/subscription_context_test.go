package messaging

import (
	"context"
	"errors"
	"testing"
)

func TestSubscribeOutlivesConstructionContext(t *testing.T) {
	broker := newFakeBroker()
	setup, cancel := context.WithCancel(t.Context())
	defer cancel()
	pubsub, err := newPubSubContext(setup, "amqp://fake", broker.dial, WithTopology(testTopology()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pubsub.Close(); err != nil {
			t.Error(err)
		}
	})
	expected := errors.New("end subscription setup probe")
	broker.consumeErr = expected
	cancel()
	err = pubsub.Subscribe(t.Context(), "q", "svc", func(context.Context, Message) error { return nil })
	if !errors.Is(err, expected) {
		t.Fatalf("subscription inherited construction cancellation: %v", err)
	}
}
