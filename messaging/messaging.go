package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/trypanic/go-sdk/algorithms"
	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// PubSub implements Messager backed by RabbitMQ with a topology-driven
// configuration.  It owns a single Publisher (shared, thread-safe) and
// creates one Subscriber per Subscribe call.
type PubSub struct {
	publisher  *Publisher
	topology   *Topology
	amqpURL    string
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
	log        *logger.Logger
}

// PubSubOption configures explicit dependencies on a PubSub.
type PubSubOption func(*pubSubBuild)

// pubSubBuild is the mutable accumulator used by options. Constructors copy
// resolved values into the immutable PubSub struct.
type pubSubBuild struct {
	tracer       trace.Tracer
	propagator   propagation.TextMapPropagator
	topology     *Topology
	topologyPath string
	log          *logger.Logger
}

// WithTracer overrides the OpenTelemetry tracer used for publish/consume spans.
// Pass nil to keep the default behavior (otel.Tracer("messaging") at call time).
func WithTracer(tracer trace.Tracer) PubSubOption {
	return func(p *pubSubBuild) { p.tracer = tracer }
}

// WithPropagator overrides the propagator used to inject/extract W3C trace
// context on AMQP headers.
func WithPropagator(propagator propagation.TextMapPropagator) PubSubOption {
	return func(p *pubSubBuild) { p.propagator = propagator }
}

// WithTopology supplies an in-memory topology so NewPubSub does not have to
// read the filesystem or any environment variable. Tests should prefer this.
func WithTopology(topo *Topology) PubSubOption {
	return func(p *pubSubBuild) { p.topology = topo }
}

// WithTopologyFile supplies an explicit path to a topology JSON file. Takes
// precedence over the legacy env-var fallback.
func WithTopologyFile(path string) PubSubOption {
	return func(p *pubSubBuild) { p.topologyPath = path }
}

// WithLogger injects an explicit logger instance for messaging reconnect/retry
// diagnostics. When nil, the messaging package falls back to the package-level
// global logger so existing call sites keep working.
func WithLogger(l *logger.Logger) PubSubOption {
	return func(p *pubSubBuild) { p.log = l }
}

// NewPubSub builds a Messager. The topology source is selected in order:
//
//  1. WithTopology — the supplied *Topology is used as-is.
//  2. WithTopologyFile — the file at the supplied path is loaded.
//  3. Legacy fallback: RABBITMQ_TOPOLOGY_FILE env var. Returns an error if
//     none of the three are available; no directory walking is performed.
//
// Telemetry dependencies (tracer, propagator) default to the global OTel
// values when not supplied via options.
func NewPubSub(amqpURL string, opts ...PubSubOption) (Messager, error) {
	build := &pubSubBuild{}
	for _, opt := range opts {
		opt(build)
	}

	topo, err := resolveTopology(build)
	if err != nil {
		return nil, err
	}

	pub, err := NewPublisher(amqpURL, topo.Exchanges)
	if err != nil {
		return nil, err
	}

	return &PubSub{
		publisher:  pub,
		topology:   topo,
		amqpURL:    amqpURL,
		tracer:     build.tracer,
		propagator: build.propagator,
		log:        build.log,
	}, nil
}

func resolveTopology(b *pubSubBuild) (*Topology, error) {
	switch {
	case b.topology != nil:
		return b.topology, nil
	case b.topologyPath != "":
		return LoadTopologyFromFile(b.topologyPath)
	default:
		return LoadTopologyFromEnv()
	}
}

func (p *PubSub) tracerOrDefault() trace.Tracer {
	if p.tracer != nil {
		return p.tracer
	}
	return otel.Tracer("messaging")
}

func (p *PubSub) propagatorOrDefault() propagation.TextMapPropagator {
	if p.propagator != nil {
		return p.propagator
	}
	return otel.GetTextMapPropagator()
}

// Publish publishes payload to the exchange that the topology binds to
// queueName.  The routing keys are resolved from the topology — the caller
// never needs to know them.  See README "Why Publish Uses a Queue Name" for
// the reasoning behind this indirection.
func (p *PubSub) Publish(ctx context.Context, queueName string, payload Event) error {
	queue, binding, err := p.getQueueAndBinding(queueName)
	if err != nil {
		return err
	}

	if payload.OccurredAt.IsZero() {
		payload.OccurredAt = time.Now()
	}

	eventMsg := EventMessage{
		EventID:      uuid.New().String(),
		EventVersion: "v1",
		EventType:    queueName,
		RoutingKeys:  binding.RoutingKeys,
		Event:        payload,
	}

	data, err := json.Marshal(eventMsg)
	if err != nil {
		return errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("failed to marshal event"),
			errorkit.WithWrapped(err),
		)
	}

	// Producer span (one per PublishCtx call)
	ctx, span := p.tracerOrDefault().Start(ctx,
		"messaging.publish "+queueName,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", queueName),
			attribute.String("messaging.rabbitmq.exchange", binding.Exchange),
			attribute.String("messaging.operation", "publish"),
			attribute.String("event.id", eventMsg.EventID),
			attribute.String("event.type", eventMsg.EventType),
			attribute.String("correlation.id", payload.CorrelationID),
		),
	)
	defer span.End()

	// Populate OTel Baggage with business identifiers so downstream services
	// can read them from span context without parsing AMQP headers manually.
	if payload.CorrelationID != "" {
		if member, err := baggage.NewMember("correlation_id", payload.CorrelationID); err == nil {
			if bag, err := baggage.New(member); err == nil {
				ctx = baggage.ContextWithBaggage(ctx, bag)
			}
		}
	}

	publishOnce := func() error {
		for _, rk := range binding.RoutingKeys {
			// Inject W3C context (TraceContext + Baggage) into AMQP headers
			headers := amqp.Table{
				"event_id":       eventMsg.EventID,
				"event_type":     eventMsg.EventType,
				"correlation_id": payload.CorrelationID,
				"event_version":  eventMsg.EventVersion,
				"service_name":   payload.ServiceName,
			}

			p.propagatorOrDefault().Inject(ctx, newCarrier(headers))

			if err := p.publisher.PublishWithHeaders(binding.Exchange, rk, data, headers); err != nil {
				return err
			}
		}
		return nil
	}

	if err := executeWithRetry(publishOnce, queue.Retry); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "publish failed")
		return errorkit.NewError(ERR_MQ_PUBLISH_FAILED).With(
			errorkit.WithReason("failed to publish"),
			errorkit.WithWrapped(err),
		)
	}

	return nil
}

// Subscribe blocks and consumes messages for queueName, calling handler for
// each delivery.  On connection drops the subscriber reconnects with
// exponential backoff automatically.  Returns when ctx is cancelled or an
// unrecoverable error occurs.
func (p *PubSub) Subscribe(ctx context.Context, queueName string, serviceName string, handler MessageHandler) error {
	queue, binding, err := p.getQueueAndBinding(queueName)
	if err != nil {
		return err
	}

	exchange, ok := p.topology.GetExchangeByName(binding.Exchange)
	if !ok {
		return errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("exchange not found in topology"),
			errorkit.WithWrapped(fmt.Errorf("exchange %s referenced by queue %s not found in topology", binding.Exchange, queueName)),
		)
	}

	args := buildQueueArgs(queue)

	sub, err := NewSubscriber(p.amqpURL, exchange, queue, serviceName, args, binding.RoutingKeys)
	if err != nil {
		return errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("failed to create subscriber"),
			errorkit.WithWrapped(err),
		)
	}
	defer sub.Close()

	msgs, err := sub.Consume()
	if err != nil {
		return errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("failed to start consuming"),
			errorkit.WithWrapped(err),
		)
	}

	for {
		select {
		case <-ctx.Done():
			return nil

		case msg, ok := <-msgs:
			if !ok {
				if err := reconnectWithBackoff(ctx, sub, p.log); err != nil {
					return err
				}
				msgs, err = sub.Consume()
				if err != nil {
					return err
				}
				continue
			}

			p.handleDelivery(ctx, queueName, msg, queue, handler)
		}
	}
}

func (p *PubSub) handleDelivery(ctx context.Context, queueName string, msg amqp.Delivery, queue QueueTopology, handler MessageHandler) {
	carrier := newCarrier(msg.Headers)
	parentCtx := p.propagatorOrDefault().Extract(ctx, carrier)

	recvCtx, recvSpan := p.tracerOrDefault().Start(parentCtx,
		"messaging.receive "+queueName,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", queueName),
			attribute.String("messaging.rabbitmq.exchange", msg.Exchange),
			attribute.String("messaging.rabbitmq.routing_key", msg.RoutingKey),
			attribute.String("messaging.operation", "receive"),
		),
	)
	defer recvSpan.End()

	m := Message{
		Raw:        msg.Body,
		Headers:    map[string]any(msg.Headers),
		Exchange:   msg.Exchange,
		RoutingKey: msg.RoutingKey,
		MessageID:  msg.MessageId,
	}

	attempt := 0
	run := func() error {
		attempt++
		procCtx, procSpan := p.tracerOrDefault().Start(recvCtx,
			"messaging.process "+queueName,
			trace.WithAttributes(attribute.Int("retry.attempt", attempt)),
		)
		defer procSpan.End()

		err := handler(procCtx, m)
		if err != nil {
			procSpan.RecordError(err)
			procSpan.SetStatus(codes.Error, "handler error")
		}
		return err
	}

	if err := executeWithRetry(run, queue.Retry); err != nil {
		recvSpan.RecordError(err)
		recvSpan.SetStatus(codes.Error, "handler failed after retries")
		_ = msg.Nack(false, false)
		return
	}

	_ = msg.Ack(false)
}

// Close closes the shared publisher connection.
func (p *PubSub) Close() error {
	return p.publisher.Close()
}

// ---------------------------------------------------------------------------
// helpers (unexported)
// ---------------------------------------------------------------------------

// getQueueAndBinding looks up queue + binding in the topology, returning a
// clear error when either is missing.
func (p *PubSub) getQueueAndBinding(queueName string) (QueueTopology, Binding, error) {
	queue, ok := p.topology.GetQueueByName(queueName)
	if !ok {
		return QueueTopology{}, Binding{}, errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("queue not found in topology"),
			errorkit.WithWrapped(fmt.Errorf("queue %s not found in topology", queueName)),
		)
	}
	binding, ok := p.topology.GetBindingByQueue(queueName)
	if !ok {
		return QueueTopology{}, Binding{}, errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("binding not found in topology"),
			errorkit.WithWrapped(fmt.Errorf("no binding found for queue %s in topology", queueName)),
		)
	}
	return queue, binding, nil
}

// executeWithRetry runs fn with exponential backoff when retry is non-nil.
// If retry is nil fn is called exactly once — no retry wrapper is created.
// This eliminates the duplicated if/else retry block that previously existed
// in both Publish and Subscribe.
func executeWithRetry(fn func() error, retry *RetryConfig) error {
	if retry == nil {
		return fn()
	}
	bo := algorithms.NewExponentialBackoff(algorithms.ExponentialBackoffConfig{
		InitialInterval: time.Duration(retry.InitialDelay) * time.Millisecond,
		MaxInterval:     time.Duration(retry.MaxDelay) * time.Millisecond,
		Multiplier:      retry.BackoffMult,
		MaxRetries:      uint64(retry.MaxAttempts),
	})
	return backoff.Retry(fn, bo)
}

// reconnectWithBackoff retries sub.Reconnect with exponential delay until
// success or ctx cancellation.  Delay starts at DefaultReconnectDelay and
// doubles up to MaxReconnectDelay. The log argument may be nil — in that
// case the package-level global logger is used.
func reconnectWithBackoff(ctx context.Context, sub *Subscriber, log *logger.Logger) error {
	delay := DefaultReconnectDelay
	for {
		select {
		case <-ctx.Done():
			return errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
				errorkit.WithReason("subscriber reconnect canceled"),
				errorkit.WithWrapped(ctx.Err()),
			)
		case <-time.After(delay):
			if err := sub.Reconnect(); err != nil {
				warnReconnect(log, sub.queue.Name, err, delay)
				delay *= 2
				if delay > MaxReconnectDelay {
					delay = MaxReconnectDelay
				}
				continue
			}
			return nil
		}
	}
}

func warnReconnect(log *logger.Logger, queue string, err error, delay time.Duration) {
	if log != nil {
		log.Warn("reconnect attempt failed. queue %s, error %s, retry_in %d", queue, err, delay)
		return
	}
	logger.Warn("reconnect attempt failed. queue %s, error %s, retry_in %d", queue, err, delay)
}

// buildQueueArgs translates the topology QueueTopology into the amqp.Table
// that RabbitMQ expects on QueueDeclare.  Custom arguments (e.g. x-max-priority)
// are merged last so they can override defaults if needed.
// JSON numbers arrive as float64; whole-number floats are converted to int64
// because several AMQP arguments (priority, max-length …) require integers.
func buildQueueArgs(queue QueueTopology) amqp.Table {
	args := amqp.Table{}

	if queue.TTL > 0 {
		args["x-message-ttl"] = queue.TTL
	}
	if queue.MaxLength > 0 {
		args["x-max-length"] = queue.MaxLength
	}
	if queue.Overflow != "" {
		args["x-overflow"] = queue.Overflow
	}
	if queue.QueueMode != "" {
		args["x-queue-mode"] = queue.QueueMode
	}
	if queue.DLX != nil {
		args["x-dead-letter-exchange"] = queue.DLX.Exchange
		if queue.DLX.RoutingKey != "" {
			args["x-dead-letter-routing-key"] = queue.DLX.RoutingKey
		}
	}

	for k, v := range queue.Arguments {
		if f, ok := v.(float64); ok && f == float64(int64(f)) {
			args[k] = int64(f)
		} else {
			args[k] = v
		}
	}

	return args
}
