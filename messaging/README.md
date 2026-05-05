# messaging

A Go library for publishing and consuming messages over RabbitMQ. The entire
broker setup — exchanges, queues, bindings, retry policies, and dead-letter
queues — is declared in a single JSON topology file.

---

## Architecture at a Glance

```
┌────────────┐   Publish("queue.name", event)   ┌──────────┐
│ Your Code  │ ────────────────────────────────> │  PubSub  │
│            │                                   │          │
│            │ Subscribe(ctx, "queue.name", fn)  │          │
│            │ <──────────────────────────────── │          │
└────────────┘                                   └────┬─────┘
                                                      │  reads
                                                 topology.json
                                                      │  declares
                                              ┌───────▼────────┐
                                              │    RabbitMQ     │
                                              │ exchanges · DLX │
                                              │ queues   · DLQs │
                                              └─────────────────┘
```

`PubSub` owns **one shared, thread-safe Publisher** and spins up a dedicated
**Subscriber per `Subscribe` call**.  Everything each side needs (exchange
names, routing keys, QoS, DLX, retry) is resolved from the topology — no
broker-specific knowledge leaks into application code.

---

## How It Works Internally

### Topology-Driven Configuration

All RabbitMQ resources are declared in a JSON topology file.  When `NewPubSub`
is called it:

1. Reads and parses the topology file.
2. Declares **every exchange** listed in the file on the broker (idempotent).

Queues and bindings are declared lazily: each call to `Subscribe` declares the
target queue and binds it before it starts consuming.

The topology source is selected via constructor options, in priority order:

1. `WithTopology(*Topology)` — supply a fully-built topology in memory (best for tests).
2. `WithTopologyFile(path)` — load from a specific file.
3. Otherwise, `RABBITMQ_TOPOLOGY_FILE` env var. The package no longer walks parent directories looking for `migrations/rabbitmq/topology.json`; supply the path explicitly.

### Why `Publish` Takes a Queue Name — Not a Routing Key

```go
pubsub.Publish("order.created.processor", event)
```

Internally the library resolves the exchange and routing keys from the topology:

```
Publish("order.created.processor")
  └── topology lookup
        ├── queue      → "order.created.processor"
        ├── exchange   → "order.events"          (from the binding)
        └── routingKeys→ ["order.created"]       (from the binding)
  └── publishes to exchange "order.events" with routing key "order.created"
```

Three reasons for this indirection:

| # | Reason | What it means in practice |
|---|--------|---------------------------|
| 1 | **Intent over mechanism** | The queue name says *what* and *who*. Routing keys are an internal routing detail of the exchange. |
| 2 | **Zero-code topology changes** | To reroute, fan-out, or split traffic you edit the topology file — no application code changes. |
| 3 | **Traceability** | `grep order.created.processor` across the codebase immediately reveals the data-flow path. |

### Dead Letter Queues (DLQ)

When all retry attempts for a message are exhausted the subscriber **Nacks**
the message without requeueing.  RabbitMQ then forwards it automatically to
the dead-letter exchange (`x-dead-letter-exchange`) configured on the queue,
which routes it to the matching DLQ via the routing key defined in `dlx.routingKey`.

No manual re-publishing to the DLX is needed — the broker handles it natively.

### Automatic Reconnection

Connection drops are handled transparently at both ends:

| Component | Strategy |
|-----------|----------|
| **Publisher** | Checks the connection before every `Publish`.  If closed, reconnects first.  If the publish itself fails (channel died between the check and the call) it reconnects once more and retries.  Further retries are governed by the per-queue backoff config. |
| **Subscriber** | When the delivery channel closes the subscriber enters a reconnect loop.  The delay starts at `DefaultReconnectDelay` (5 s) and doubles on each failed attempt, capped at `MaxReconnectDelay` (60 s).  Once reconnected it resumes consuming on the same queue without losing its position. |

Both sides open their connections with an AMQP heartbeat interval
(`DefaultHeartbeat`, 60 s) so dead TCP links are detected quickly.

### Retry with Exponential Backoff

Retry is configured **per queue** in the topology (`retry` block).  The same
policy is applied to both publish and consume via the shared `executeWithRetry`
helper.  Under the hood it delegates to the internal `algorithms.ExponentialBackoff`
utility.

When retries are exhausted:

* **Publish** — the error is returned to the caller.
* **Subscribe** — the message is Nacked → RabbitMQ routes it to the DLQ.

---

## Usage

### Initialisation

```go
import "github.com/trypanic/go-sdk/messaging"

// Minimal: relies on RABBITMQ_TOPOLOGY_FILE env var.
pubsub, err := messaging.NewPubSub("amqp://user:pass@rabbitmq:5672/")
if err != nil {
    log.Fatal(err)
}
defer pubsub.Close()

// Explicit topology file path:
pubsub, err = messaging.NewPubSub(
    "amqp://user:pass@rabbitmq:5672/",
    messaging.WithTopologyFile("/etc/myapp/topology.json"),
)

// In-memory topology (preferred in tests):
topo, _ := messaging.LoadTopologyFromBytes(rawJSON)
pubsub, err = messaging.NewPubSub(
    "amqp://user:pass@rabbitmq:5672/",
    messaging.WithTopology(topo),
)

// Explicit telemetry + logger:
pubsub, err = messaging.NewPubSub(
    "amqp://user:pass@rabbitmq:5672/",
    messaging.WithTracer(myTracer),
    messaging.WithPropagator(myPropagator),
    messaging.WithLogger(myLogger),
)
```

### Publishing

```go
event := messaging.Event{
    CorrelationID: "req-123",
    ServiceName:   "order-service",
    Payload: map[string]any{
        "order_id": "ord-456",
        "amount":   99.99,
    },
    Metadata: map[string]any{
        "source":  "api",
        "version": "2",
    },
    // OccurredAt is optional — set to time.Now() automatically if zero.
}

if err := pubsub.Publish(ctx, "order.created.processor", event); err != nil {
    slog.Error("publish failed", "error", err)
}
```

The published JSON envelope looks like:

```json
{
  "event_id":      "uuid …",
  "event_version": "v1",
  "event_type":    "order.created.processor",
  "routing_keys":  ["order.created"],
  "correlation_id":"req-123",
  "service_name":  "order-service",
  "payload":       { … },
  "metadata":      { … },
  "occurred_at":   "2025-…"
}
```

### Subscribing

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

handler := func(ctx context.Context, msg messaging.Message) error {
    var event messaging.EventMessage
    if err := json.Unmarshal(msg.Raw, &event); err != nil {
        return err  // triggers retry per the queue's retry config
    }

    slog.InfoContext(ctx, "processing", "event_id", event.EventID, "type", event.EventType)
    // … do work …
    return nil  // nil → message is Acked
}

// Blocks until ctx is cancelled.  Reconnection is automatic.
if err := pubsub.Subscribe(ctx, "order.created.processor", "order-worker", handler); err != nil {
    slog.Error("subscriber exited", "error", err)
}
```

### Mocking in Tests

`Messager` is the public interface — use it in your function signatures so
the real broker is never needed in unit tests:

```go
type Messager interface {
    Publish(ctx context.Context, queueName string, payload messaging.Event) error
    Subscribe(ctx context.Context, queueName string, serviceName string, handler messaging.MessageHandler) error
    Close() error
}

func NewOrderService(bus messaging.Messager) *OrderService { … }
```

---

## Custom Topology

Any JSON file that matches the schema in **`schemas/rabbit-topology.json`** can
be used. Supply it via `WithTopologyFile` (or `WithTopology` for tests):

```go
pubsub, err := messaging.NewPubSub(amqpURL,
    messaging.WithTopologyFile("/config/my-topology.json"),
)
```

---

## Error Handling Summary

| Scenario | What happens |
|----------|--------------|
| `Publish` fails after all retries | Error returned to the caller |
| Handler returns an error | Message retried per queue config; after exhaustion it is Nacked → DLQ |
| Connection drops during `Subscribe` | Automatic reconnect with exponential backoff |
| Context cancelled | Subscriber exits cleanly, connection is closed via `defer` |
| Marshal / unmarshal failure | Error returned immediately (no retry — it would produce the same result) |
