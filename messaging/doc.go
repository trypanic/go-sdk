// Package messaging publishes and consumes events over RabbitMQ (AMQP 0-9-1)
// with a declarative, topology-driven configuration: exchanges, queues,
// bindings, dead-letter exchanges and per-queue retry policies are described
// once in a JSON Topology, and callers address queues by name only.
//
// # Construction
//
// NewPubSub builds a Messager from WithTopology or WithTopologyFile;
// NewPubSubFromEnv loads the topology from the file named by
// RABBITMQ_TOPOLOGY_FILE. No other constructor reads the environment.
// Topologies are validated (Topology.Validate) before anything is dialed.
// Connection options (WithTLSConfig, WithHeartbeat, WithDialTimeout,
// WithConnectionName) apply to every connection; WithTracer, WithPropagator
// and WithLogger set the observability dependencies.
//
// # Delivery semantics
//
// Publishing uses publisher confirms and the mandatory flag: Publish returns
// nil only after the broker has confirmed every routing key, and reports a
// nack or an unroutable message as ERR_MQ_PUBLISH_FAILED. Consumption acks a
// message only after its handler returns nil. Both directions are
// at-least-once, so handlers must be idempotent (deduplicate on the
// event_id of the EventMessage envelope).
//
// # Concurrency and lifecycle
//
// A PubSub is safe for concurrent use: create one per process and share it.
// Its Publisher serializes publishes on one channel; each Subscribe call
// owns its own Subscriber and connection and handles one message at a time.
// A standalone Subscriber is not safe for concurrent use. Close is terminal:
// it stops every Subscribe call and closes the publisher, after which
// Publish and Subscribe fail with ERR_MQ_CLOSED. The caller owns the
// lifecycle and defers Close at process scope.
//
// # Errors
//
// Every error is an *errorkit.AppError with an ERR_MQ_* code (registered by
// RegisterErrors) or ERR_SYSTEM_CONFIG_INVALID; callers branch on Code() and
// Metadata.Retriable. See the package README for the full contract.
package messaging
