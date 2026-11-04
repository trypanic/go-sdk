package messaging_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/logger"
	"github.com/trypanic/go-sdk/messaging"
)

// ExampleNewPubSub wires a PubSub from an in-memory topology and publishes
// one event. It needs a broker, so it is compiled but not run.
func ExampleNewPubSub() {
	topo, err := messaging.LoadTopologyFromBytes([]byte(`{
		"version": "1.0.0",
		"exchanges": [{"name": "order.events", "type": "topic", "durable": true}],
		"queues": [{"name": "order.created.processor", "durable": true, "prefetch": 10,
			"retry": {"maxAttempts": 3, "initialDelay": 100, "maxDelay": 1000, "backoffMultiplier": 2}}],
		"bindings": [{"queue": "order.created.processor", "exchange": "order.events", "routingKeys": ["order.created"]}]
	}`))
	if err != nil {
		logger.Panic(err, "invalid topology")
	}

	pubsub, err := messaging.NewPubSub("amqp://user:pass@rabbitmq:5672/",
		messaging.WithTopology(topo),
		messaging.WithDeclareTopology(),
	)
	if err != nil {
		logger.Panic(err, "messaging init failed")
	}
	defer func() { _ = pubsub.Close() }()

	ctx := context.Background()
	event := messaging.Event{CorrelationID: "req-123", ServiceName: "order-service"}
	if err := pubsub.Publish(ctx, "order.created.processor", event); err != nil {
		logger.Error(err, "publish failed")
	}
}

// ExampleTopology_Validate shows that configuration mistakes are reported
// before anything is dialed.
func ExampleTopology_Validate() {
	topo := &messaging.Topology{
		Exchanges: []messaging.Exchange{{Name: "order.events", Type: "topic"}},
		Queues: []messaging.QueueTopology{{
			Name:  "order.created.processor",
			Retry: &messaging.RetryConfig{MaxAttempts: 0, BackoffMult: 2},
		}},
		Bindings: []messaging.Binding{{Queue: "order.created.processor", Exchange: "order.events", RoutingKeys: []string{"order.created"}}},
	}

	var appErr *errorkit.AppError
	if errors.As(topo.Validate(), &appErr) {
		fmt.Println(appErr.Code())
	}
	// Output: ERR_SYSTEM_CONFIG_INVALID
}
