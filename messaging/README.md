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
target queue and binds it before it starts consuming.  Pass
`WithDeclareTopology()` to have `NewPubSub` also declare **every queue and
binding** (dead-letter queues included), again on every publisher reconnect.
Without it, a message published to a queue that no subscriber has declared yet
has no route and `Publish` returns `ERR_MQ_PUBLISH_FAILED` (see
[Delivery Guarantee](#delivery-guarantee)).

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
| **Publisher** | Checks the connection before every `Publish`.  If closed (or the previous publish on it failed), reconnects first.  Exactly one retry layer applies: for a queue **with** a `retry` policy, the policy alone governs retries (`maxAttempts` = attempts per routing key, each on a fresh channel after a failure); for a queue **without** one, a failed publish reconnects once more and retries once.  The caller's `ctx` bounds the whole call: waiting for the shared channel, the dial (TCP connect and handshake), the publish and the backoff.  An ended `ctx` never dials and the error wraps `ctx.Err()` (`ERR_MQ_TIMEOUT` for a deadline).  Reconnects are single-flight and never block `Close`. |
| **Subscriber** | When the delivery channel closes the subscriber enters a reconnect loop.  The delay starts at `DefaultReconnectDelay` (5 s) and doubles on each failed attempt, capped at `MaxReconnectDelay` (60 s).  Once reconnected it resumes consuming on the same queue without losing its position.  The loop stops and `Subscribe` returns the error when it is not retriable (`Metadata.Retriable == false`, e.g. `ERR_MQ_AUTH_FAILED`), since retrying cannot succeed. |

Both sides open their connections with an AMQP heartbeat interval
(`DefaultHeartbeat`, 60 s) so dead TCP links are detected quickly.

### Delivery Guarantee

The publisher channel runs in **publisher-confirm mode** and every message is
published as **mandatory**.  `Publish` returns `nil` only after the broker has
confirmed every routing key of the event:

* A broker **nack** → `ERR_MQ_PUBLISH_FAILED`.
* An **unroutable** message (no queue bound for the routing key; the broker
  sends `basic.return`) → `ERR_MQ_PUBLISH_FAILED` with the broker's reply text
  (e.g. `NO_ROUTE`).
* No confirm before `ctx` ends → the error wraps `ctx.Err()`
  (`ERR_MQ_TIMEOUT` for a deadline).  The broker may still have stored the
  message.

Delivery is therefore **at-least-once**: a retry after a lost confirm, or a
redelivery after a consumer crash, can deliver the same event twice.
Consumers must deduplicate on `event_id`.  Confirms add one broker round trip
to every publish.

### Retry with Exponential Backoff

Retry is configured **per queue** in the topology (`retry` block).  The same
policy is applied to both publish and consume via the shared `executeWithRetry`
helper.  Under the hood it delegates to the internal `algorithms.ExponentialBackoff`
utility.

For **publish**, each routing key of the binding is retried on its own: a
retry never re-sends a key the broker has already confirmed, and
`maxAttempts` is the number of publish attempts per routing key (the
publisher adds no retry of its own when a policy is configured).

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

// Blocks until ctx is cancelled or pubsub.Close is called.  Reconnection is automatic.
if err := pubsub.Subscribe(ctx, "order.created.processor", "order-worker", handler); err != nil {
    slog.Error("subscriber exited", "error", err)
}
```

### Closing

`Close` is terminal.  It stops every `Subscribe` call of the `PubSub` (as a
cancelled `ctx` would: the in-flight message is requeued and `Subscribe`
returns `nil`) without waiting for them to return, then closes the shared
publisher connection.  After `Close`, `Publish` and `Subscribe` fail with
`ERR_MQ_CLOSED` and never dial the broker; calling `Close` again returns
`nil`.

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

## Error Codes

Every error is an `*errorkit.AppError`. SDK errors are returned unchanged, so
callers can branch on `.Code()` and `Metadata.Retriable`:

| Code | Raised when | Retriable |
|------|-------------|-----------|
| `ERR_MQ_CONNECTION_FAILED` | Dialing the broker fails | yes |
| `ERR_MQ_AUTH_FAILED` | The broker refuses access (AMQP 403: credentials, vhost or resource permissions) on any operation | **no** |
| `ERR_MQ_CHANNEL_ERROR` | Opening a channel or setting QoS fails | yes |
| `ERR_MQ_EXCHANGE_ERROR` | Declaring an exchange fails | yes |
| `ERR_MQ_QUEUE_ERROR` | Declaring or binding a queue fails | yes |
| `ERR_MQ_CONSUME_FAILED` | Starting a consumer fails | yes |
| `ERR_MQ_PUBLISH_FAILED` | Publishing a message fails, the broker nacks it, or returns it as unroutable | yes |
| `ERR_MQ_TIMEOUT` | The caller's `ctx` deadline ends a `Publish` (waiting, dialing or waiting for the confirm) | yes |
| `ERR_MQ_CLOSED` | `Publish` or `Subscribe` is called after `Close` | **no** |
| `ERR_SYSTEM_CONFIG_INVALID` | A queue, binding or exchange is missing from the topology | no |

## Error Handling Summary

| Scenario | What happens |
|----------|--------------|
| `Publish` fails after all retries | Error returned to the caller |
| Broker nacks a message or cannot route it | `ERR_MQ_PUBLISH_FAILED` (retried per queue config) |
| Handler returns an error | Message retried per queue config; after exhaustion it is Nacked → DLQ |
| Handler panics | The panic is recovered and recorded (with its stack) on the process span; the message is not retried and is Nacked without requeue → DLQ |
| Connection drops during `Subscribe` | Automatic reconnect with exponential backoff |
| Context cancelled | Subscriber exits cleanly, connection is closed via `defer`; a message being processed is Nacked with requeue (not dead-lettered) and no further retry or backoff wait happens |
| `Close` called | Active subscriptions stop as if their `ctx` were cancelled; later `Publish`/`Subscribe` calls fail with `ERR_MQ_CLOSED` |
| Marshal / unmarshal failure | Error returned immediately (no retry — it would produce the same result) |
