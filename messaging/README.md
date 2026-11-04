# messaging

A Go library for publishing and consuming messages over RabbitMQ. The entire
broker setup — exchanges, queues, bindings, retry policies, and dead-letter
queues — is declared in a single JSON topology file.

---

## Architecture at a Glance

```
┌────────────┐   Publish(ctx, "queue", event)   ┌──────────┐
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

1. Reads, parses and validates the topology (see [Validation](#validation)).
2. Opens the publisher connection and declares **every exchange** listed in
   the topology on the broker (idempotent).

Queues and bindings are declared lazily: each call to `Subscribe` declares the
target queue and binds it before it starts consuming.  Pass
`WithDeclareTopology()` to have `NewPubSub` also declare **every queue and
binding** (dead-letter queues included), again on every publisher reconnect.
Without it, a message published to a queue that no subscriber has declared yet
has no route and `Publish` returns `ERR_MQ_PUBLISH_FAILED` (see
[Delivery Guarantee](#delivery-guarantee)).

The topology source is selected in priority order:

1. `WithTopology(*Topology)` — supply a fully-built topology in memory (best for tests).
2. `WithTopologyFile(path)` — load from a specific file.
3. `NewPubSubFromEnv` only: the file named by the `RABBITMQ_TOPOLOGY_FILE` env var.

`NewPubSub` never reads environment variables: without one of the first two
options it fails with `ERR_SYSTEM_CONFIG_INVALID` before dialing.  No
directory walking is performed; supply the path explicitly.

### Why `Publish` Takes a Queue Name — Not a Routing Key

```go
pubsub.Publish(ctx, "order.created.processor", event)
```

Internally the library resolves the exchange and routing keys from the topology:

```
Publish(ctx, "order.created.processor", event)
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

Each subscriber sets the channel QoS to the queue's `prefetch`.  When the
topology omits it (or sets `0`) the subscriber uses `DefaultPrefetch` (1):
handlers run one message at a time, so a larger window would only buffer the
backlog in process memory (`0` means *unlimited* to RabbitMQ).

Both sides open their connections with an AMQP heartbeat interval
(`DefaultHeartbeat`, 60 s, or `WithHeartbeat`).  amqp091-go declares a link
dead after **three** intervals without traffic, so with the default a silent
network partition is detected after up to ~180 s; only then does the
subscriber's reconnect delay start.  Lower the interval with `WithHeartbeat`
for faster detection.

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
import (
    "github.com/trypanic/go-sdk/logger"
    "github.com/trypanic/go-sdk/messaging"
)

// Topology file named by the RABBITMQ_TOPOLOGY_FILE env var.
pubsub, err := messaging.NewPubSubFromEnv("amqp://user:pass@rabbitmq:5672/")
if err != nil {
    logger.Panic(err, "messaging init failed")
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

// TLS with a private CA / mTLS, and connection tuning:
pubsub, err = messaging.NewPubSub(
    "amqps://user:pass@rabbitmq:5671/",
    messaging.WithTLSConfig(&tls.Config{RootCAs: caPool, Certificates: []tls.Certificate{clientCert}, MinVersion: tls.VersionTLS12}),
    messaging.WithHeartbeat(10*time.Second),
    messaging.WithDialTimeout(5*time.Second),
    messaging.WithConnectionName("order-service"),
)
```

### Connection Options

Every option applies to the publisher connection and to each subscriber
connection.

| Option | Default | Effect |
|--------|---------|--------|
| `WithTLSConfig(*tls.Config)` | amqp091-go default (system roots) | TLS settings for `amqps://` URLs: private CA, client certificates (mTLS), minimum version |
| `WithHeartbeat(d)` | `DefaultHeartbeat` (60 s) | AMQP heartbeat interval; a link is dead after three missed intervals |
| `WithDialTimeout(d)` | `DefaultDialTimeout` (30 s) | Bounds the TCP connect and the TLS/AMQP handshake of every dial; a `Publish` `ctx` deadline that ends sooner still wins |
| `WithConnectionName(name)` | none | Connection name shown in the RabbitMQ management UI |

A non-positive duration keeps the default.

### Logging

The package logs only diagnostics it does not return to the caller:
publisher and subscriber reconnects, the publisher's single in-call retry,
failed acks/nacks and teardown failures at **Warn** level, and recovered
handler panics at **Error** level with a `stack` field.  They go to the
`WithLogger` logger (else the logger stored in the `ctx`, else the global one),
with structured fields (`queue`, `exchange`, `routing_key`, `error`,
`retry_in` as a duration string) and the `trace_id`/`span_id` of the active
span.

### Tracing

Spans are started on the `WithTracer` tracer (default
`otel.Tracer("messaging")`) and named with the `telemetry.Messaging`
convention, `messaging.<operation>.<queue>`.  W3C trace context and baggage
travel in the AMQP headers (`WithPropagator`).

| Span | Kind | Attributes |
|------|------|------------|
| `messaging.publish.<queue>` | Producer | `messaging.system=rabbitmq`, `messaging.destination.name=<queue>`, `messaging.operation.type=send`, `messaging.operation.name=publish`, `messaging.rabbitmq.exchange`, `event.id`, `event.type`, `correlation.id` |
| `messaging.receive.<queue>` | Consumer | `messaging.system`, `messaging.destination.name`, `messaging.operation.type=receive`, `messaging.operation.name=receive`, `messaging.rabbitmq.exchange`, `messaging.rabbitmq.destination.routing_key`, `messaging.message.id` (when set) |
| `messaging.process.<queue>` | Internal (child of receive, one per attempt) | `messaging.system`, `messaging.destination.name`, `messaging.operation.type=process`, `messaging.operation.name=process`, `retry.attempt` |

Attribute keys follow OpenTelemetry semantic conventions v1.43.0.

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
    // err is an *errorkit.AppError: branch on Code() / Metadata.Retriable.
    logger.Error(err, "publish failed")
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
        return err  // retried per the queue's retry config, then dead-lettered
    }

    logger.LogInfoWithTrace(ctx, "processing event "+event.EventID)
    // … do work …
    return nil  // nil → message is Acked
}

// Blocks until ctx is cancelled or pubsub.Close is called.  Reconnection is automatic.
if err := pubsub.Subscribe(ctx, "order.created.processor", "order-worker", handler); err != nil {
    logger.Error(err, "subscriber exited")
}
```

Delivery to handlers is **at-least-once**: handlers must be **idempotent**
(deduplicate on `event_id`).  A message is acked only after its handler
returns `nil`; if the channel dies before the ack reaches the broker (heartbeat
timeout, broker restart), the broker redelivers it and the handler runs again.
A failed `Ack`/`Nack` is recorded on the `messaging.receive.<queue>` span and logged at
Warn level (through `WithLogger` when set) with `ERR_MQ_CHANNEL_ERROR`.

### Concurrency and Lifecycle

* A `PubSub` (the `Messager` returned by `NewPubSub`/`NewPubSubFromEnv`) is
  safe for concurrent use.  Create **one per process** and share it; do not
  build one per request.
* Its publisher serializes publishes on one AMQP channel; concurrent
  `Publish` calls queue for it, each bounded by its own `ctx`.
* Each `Subscribe` call blocks, owns its own connection, and runs its handler
  **one message at a time**; run several `Subscribe` calls (goroutines) for
  parallelism.
* A standalone `Subscriber` (from `NewSubscriber`) is **not** safe for
  concurrent use.
* The caller owns the lifecycle: `defer pubsub.Close()` at process scope.

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

Any JSON file with the shape of `Topology` (fields of `Exchange`,
`QueueTopology`, `Binding`, `DLXConfig` and `RetryConfig`) can be used.
Supply it via `WithTopologyFile` (or `WithTopology` for tests):

```go
pubsub, err := messaging.NewPubSub(amqpURL,
    messaging.WithTopologyFile("/config/my-topology.json"),
)
```

### Validation

Topologies are validated when they are loaded (`LoadTopologyFrom*`) and when
`NewPubSub` receives one through `WithTopology`; call `Topology.Validate()` to
check one yourself.  Every problem is reported in one
`ERR_SYSTEM_CONFIG_INVALID` error.  A topology is rejected when:

* the JSON has an **unknown field** (e.g. a typo such as `"prefech"`) or
  trailing data;
* an exchange or queue name is empty or declared twice, or an exchange has no
  `type`;
* a binding names an undeclared queue or exchange, has no routing keys (use
  `[""]` for a fanout exchange), or a queue has **more than one binding**.
  Bindings to the predeclared `amq.*` exchanges (`amq.direct`, `amq.topic`,
  …) are therefore rejected unless listed in `exchanges`, and RabbitMQ refuses
  to declare `amq.*` names, so use your own exchanges;
* a queue's `dlx.exchange` is not declared — this includes `""`, the default
  exchange, which cannot be used as a dead-letter exchange here;
* `prefetch`, `ttl` or `maxLength` is negative;
* a `retry` block has `maxAttempts < 1`, a negative delay,
  `maxDelay < initialDelay`, or `backoffMultiplier < 1` (an omitted multiplier
  is `0` and therefore rejected).

Accepted retry values are interpreted by the shared `algorithms` exponential
backoff: an `initialDelay` or `maxDelay` of `0` is replaced by its default
(1 s and 30 s respectively) instead of meaning "retry without delay", and
`maxAttempts` is capped at `algorithms.MaxRetriesLimit` (1000).

---

## Error Codes

Every error is an `*errorkit.AppError`. SDK errors are returned unchanged, so
callers can branch on `.Code()` and `Metadata.Retriable`.

`PubSub.Publish` returns the code of the step that failed, not always
`ERR_MQ_PUBLISH_FAILED`: a broker outage is `ERR_MQ_CONNECTION_FAILED`, a
missing exchange `ERR_MQ_EXCHANGE_ERROR` (`ERR_MQ_QUEUE_ERROR` for a queue
declared by `WithDeclareTopology`), refused credentials `ERR_MQ_AUTH_FAILED`,
an expired `ctx` `ERR_MQ_TIMEOUT`, a closed `PubSub` `ERR_MQ_CLOSED`, and only a
failed, nacked or returned publish `ERR_MQ_PUBLISH_FAILED`.  Do not match on
`ERR_MQ_PUBLISH_FAILED` alone: decide on `Metadata.Retriable`, or test for a
specific code with `errorkit.HasCode(err, code)` (or `errors.As` and
`.Code()`).

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
| `ERR_SYSTEM_CONFIG_INVALID` | The topology is invalid (see [Validation](#validation)), or a queue, binding or exchange is missing from it | no |

## Error Handling Summary

| Scenario | What happens |
|----------|--------------|
| `Publish` fails after all retries | Error returned to the caller |
| Broker nacks a message or cannot route it | `ERR_MQ_PUBLISH_FAILED` (retried per queue config) |
| Handler returns an error | Message retried per queue config; after exhaustion it is Nacked → DLQ |
| Handler panics | The panic is recovered, recorded (with its stack) on the process span and logged at Error level with its stack; the message is not retried and is Nacked without requeue → DLQ |
| Connection drops during `Subscribe` | Automatic reconnect with exponential backoff |
| Context cancelled | Subscriber exits cleanly, connection is closed via `defer`; a message being processed is Nacked with requeue (not dead-lettered) and no further retry or backoff wait happens |
| `Close` called | Active subscriptions stop as if their `ctx` were cancelled; later `Publish`/`Subscribe` calls fail with `ERR_MQ_CLOSED` |
| Marshal / unmarshal failure | Error returned immediately (no retry — it would produce the same result) |

Tracing injection accepts a shared `*telemetry.Instrumenter`. Use
`grpc.WithServerInstrumenter` / `grpc.WithClientInstrumenter` or
`messaging.WithInstrumenter`; each keeps its component scope and uses
`instr.TracerProvider()`. A nil instrumenter selects a no-op provider.
Hertz tracing remains an exception: its adapter uses the global provider.

### Cancellable setup

`NewPubSubContext(ctx, url, opts...)` and `NewPubSubFromEnvContext` bound initial
TCP/TLS/AMQP setup, channel confirmation and topology declaration with the caller
context and `WithDialTimeout` (30 seconds by default), whichever ends first.
Cancellation interrupts an in-progress handshake. The construction context does
not own the resulting instance: use `Close` for its lifetime and pass a context
to each operation. Existing constructors delegate with a background context.

### Concrete adapters and escape hatches (v2)

Constructors return concrete adapters; consumers should accept the narrow
interfaces they need. HTTP and LLM clients retain the constructor's instrumenter
inside the concrete adapter, producing one span per call. `WrapWithInstrumenter`
still preserves its input when the instrumenter is nil; an explicit non-nil wrapper
exposes `Unwrap()` to recover the enclosed interface. Wrapping an already traced
client intentionally creates another span, so wrap a tracing-disabled client.
