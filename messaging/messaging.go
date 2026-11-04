package messaging

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
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
	dial       dialFunc // used by subscribers; nil means dialAMQP
	connOpts   connOptions
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
	log        *logger.Logger

	// life is cancelled by Close; every Subscribe stops with it.
	lifeOnce sync.Once
	life     context.Context
	stop     context.CancelFunc
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
	declareTopo  bool
	conn         connOptions
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

// WithTLSConfig sets the TLS configuration used for amqps:// URLs: a private
// CA (RootCAs), client certificates for mTLS, a minimum TLS version. nil
// keeps amqp091-go's default (system roots).
func WithTLSConfig(cfg *tls.Config) PubSubOption {
	return func(p *pubSubBuild) { p.conn.tls = cfg }
}

// WithHeartbeat sets the AMQP heartbeat interval of every connection
// (default DefaultHeartbeat). amqp091-go treats a link as dead after three
// intervals without traffic, so a shorter interval detects a silent network
// partition sooner. d <= 0 keeps the default.
func WithHeartbeat(d time.Duration) PubSubOption {
	return func(p *pubSubBuild) { p.conn.heartbeat = d }
}

// WithDialTimeout bounds the TCP connect and the TLS/AMQP handshake of every
// dial (default DefaultDialTimeout). A Publish ctx deadline that ends sooner
// still wins. d <= 0 keeps the default.
func WithDialTimeout(d time.Duration) PubSubOption {
	return func(p *pubSubBuild) { p.conn.dialTimeout = d }
}

// WithConnectionName sets the client connection name that the RabbitMQ
// management UI shows for the publisher and subscriber connections.
func WithConnectionName(name string) PubSubOption {
	return func(p *pubSubBuild) { p.conn.name = name }
}

// WithDeclareTopology makes NewPubSub declare every queue and binding of the
// topology (including dead-letter queues), not only the exchanges, and
// declare them again on every publisher reconnect. Without it queues are
// declared lazily by Subscribe, and a message published to a queue that no
// subscriber has declared yet is unroutable: Publish returns
// ERR_MQ_PUBLISH_FAILED for it.
func WithDeclareTopology() PubSubOption {
	return func(p *pubSubBuild) { p.declareTopo = true }
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
	ps, err := newPubSub(amqpURL, nil, opts...)
	if err != nil {
		return nil, err
	}
	return ps, nil
}

// newPubSub is NewPubSub with the publisher dialing through dial (nil means
// dialAMQP).
func newPubSub(amqpURL string, dial dialFunc, opts ...PubSubOption) (*PubSub, error) {
	build := &pubSubBuild{}
	for _, opt := range opts {
		opt(build)
	}

	topo, err := resolveTopology(build)
	if err != nil {
		return nil, err
	}

	pub := &Publisher{amqpURL: amqpURL, exchanges: topo.Exchanges, dial: dial, opts: build.conn}
	if build.declareTopo {
		pub.declare = topo
	}
	if err := pub.connect(); err != nil {
		return nil, err
	}

	return &PubSub{
		publisher:  pub,
		topology:   topo,
		amqpURL:    amqpURL,
		dial:       dial,
		connOpts:   build.conn,
		tracer:     build.tracer,
		propagator: build.propagator,
		log:        build.log,
	}, nil
}

func resolveTopology(b *pubSubBuild) (*Topology, error) {
	switch {
	case b.topology != nil:
		if err := b.topology.Validate(); err != nil {
			return nil, err
		}
		return b.topology, nil
	case b.topologyPath != "":
		return LoadTopologyFromFile(b.topologyPath)
	default:
		return LoadTopologyFromEnv()
	}
}

// lifetime returns the context that Close cancels.
func (p *PubSub) lifetime() context.Context {
	p.lifeOnce.Do(func() { p.life, p.stop = context.WithCancel(context.Background()) })
	return p.life
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

	// Each routing key is retried on its own, so a retry never re-sends a
	// key that was already confirmed. With a retry policy it is the only
	// retry layer; without one the publisher's reconnect + single retry is.
	retryOnce := queue.Retry == nil
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

		publishKey := func() error {
			err := p.publisher.publish(ctx, binding.Exchange, rk, data, headers, retryOnce)
			if err != nil && !isRetriable(err) {
				return backoff.Permanent(err) // e.g. ERR_MQ_CLOSED
			}
			return err
		}
		if err := executeWithRetry(ctx, publishKey, queue.Retry); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "publish failed")
			return asAppError(err, ERR_MQ_PUBLISH_FAILED, "failed to publish")
		}
	}

	return nil
}

// Subscribe blocks and consumes messages for queueName, calling handler for
// each delivery.  On connection drops the subscriber reconnects with
// exponential backoff automatically.  Returns when ctx is cancelled, Close
// is called, or an unrecoverable error occurs. After Close it fails with
// ERR_MQ_CLOSED.
func (p *PubSub) Subscribe(ctx context.Context, queueName string, serviceName string, handler MessageHandler) error {
	life := p.lifetime()
	if life.Err() != nil {
		return errorkit.NewError(ERR_MQ_CLOSED).With(errorkit.WithReason("pubsub is closed"))
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopOnClose := context.AfterFunc(life, cancel)
	defer stopOnClose()

	queue, binding, err := p.getQueueAndBinding(queueName)
	if err != nil {
		return err
	}

	exchange, ok := p.topology.GetExchangeByName(binding.Exchange)
	if !ok {
		return errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("exchange not found in topology"),
			errorkit.WithWrapped(fmt.Errorf("exchange %s referenced by queue %s not found in topology", binding.Exchange, queueName)),
		)
	}

	args := buildQueueArgs(queue)

	sub := &Subscriber{
		amqpURL:     p.amqpURL,
		exchange:    exchange,
		queue:       queue,
		consumer:    serviceName,
		queueArgs:   args,
		routingKeys: binding.RoutingKeys,
		opts:        p.connOpts,
		dial:        p.dial,
	}
	if err := sub.connect(); err != nil {
		return asAppError(err, ERR_MQ_CONNECTION_FAILED, "failed to create subscriber")
	}
	defer sub.Close()

	msgs, err := sub.Consume()
	if err != nil {
		return asAppError(err, ERR_MQ_CONNECTION_FAILED, "failed to start consuming")
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
	// The subscription is shutting down: hand the message back to the broker
	// untouched instead of running the handler against a cancelled ctx.
	if ctx.Err() != nil {
		p.reportSettleFailure(ctx, nil, queueName, "nack", msg.Nack(false, true))
		return
	}

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
	run := func() (err error) {
		attempt++
		procCtx, procSpan := p.tracerOrDefault().Start(recvCtx,
			"messaging.process "+queueName,
			trace.WithAttributes(attribute.Int("retry.attempt", attempt)),
		)
		defer procSpan.End()

		// A panic in the handler is a poison message: record it with its
		// stack, never retry it, and let the caller dead-letter it.
		defer func() {
			if r := recover(); r != nil {
				panicErr := fmt.Errorf("messaging: handler panicked: %v", r)
				procSpan.RecordError(panicErr, trace.WithStackTrace(true))
				procSpan.SetStatus(codes.Error, "handler panicked")
				err = backoff.Permanent(panicErr)
			}
		}()

		err = handler(procCtx, m)
		if err != nil {
			procSpan.RecordError(err)
			procSpan.SetStatus(codes.Error, "handler error")
		}
		return err
	}

	if err := executeWithRetry(ctx, run, queue.Retry); err != nil {
		recvSpan.RecordError(err)
		if ctx.Err() != nil {
			// Shutdown interrupted processing: requeue so the message is not
			// dead-lettered (or dropped when the queue has no DLX).
			recvSpan.SetStatus(codes.Error, "subscription canceled; message requeued")
			p.reportSettleFailure(recvCtx, recvSpan, queueName, "nack", msg.Nack(false, true))
			return
		}
		recvSpan.SetStatus(codes.Error, "handler failed after retries")
		p.reportSettleFailure(recvCtx, recvSpan, queueName, "nack", msg.Nack(false, false))
		return
	}

	p.reportSettleFailure(recvCtx, recvSpan, queueName, "ack", msg.Ack(false))
}

// reportSettleFailure makes a failed Ack or Nack (op) visible on span (when
// non-nil) and in the log. The message stays unacked on the broker, which
// redelivers it once the channel closes, so its handler can run again.
func (p *PubSub) reportSettleFailure(ctx context.Context, span trace.Span, queueName, op string, err error) {
	if err == nil {
		return
	}
	appErr := mqError(ERR_MQ_CHANNEL_ERROR, op+" failed; the broker will redeliver the message", err)
	if span != nil {
		span.RecordError(appErr)
		span.SetStatus(codes.Error, op+" failed")
	}
	msg := fmt.Sprintf("messaging: %s failed for queue %s; the broker will redeliver the message", op, queueName)
	if p.log != nil {
		p.log.WarnCtx(ctx, appErr, msg)
		return
	}
	logger.WarnCtx(ctx, appErr, msg)
}

// Close stops every Subscribe call of this PubSub (each returns nil once its
// in-flight message is settled; see the README) and closes the shared
// publisher connection. Close is terminal: Publish and Subscribe then fail
// with ERR_MQ_CLOSED. Close does not wait for the subscriptions to return.
func (p *PubSub) Close() error {
	p.lifetime()
	p.stop()
	if p.publisher == nil {
		return nil
	}
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
		return QueueTopology{}, Binding{}, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("queue not found in topology"),
			errorkit.WithWrapped(fmt.Errorf("queue %s not found in topology", queueName)),
		)
	}
	binding, ok := p.topology.GetBindingByQueue(queueName)
	if !ok {
		return QueueTopology{}, Binding{}, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
			errorkit.WithReason("binding not found in topology"),
			errorkit.WithWrapped(fmt.Errorf("no binding found for queue %s in topology", queueName)),
		)
	}
	return queue, binding, nil
}

// executeWithRetry runs fn with exponential backoff when retry is non-nil.
// If retry is nil fn is called exactly once — no retry wrapper is created.
// The backoff is bound to ctx: once ctx is done no further attempt is made
// and no backoff delay is waited.
func executeWithRetry(ctx context.Context, fn func() error, retry *RetryConfig) error {
	if retry == nil {
		err := fn()
		var permanent *backoff.PermanentError
		if errors.As(err, &permanent) {
			return permanent.Err
		}
		return err
	}
	// Topology.Validate rejects MaxAttempts < 1; clamp anyway so a value that
	// bypassed validation cannot wrap to an unbounded uint64.
	attempts := max(retry.MaxAttempts, 1)
	bo := algorithms.NewExponentialBackoff(algorithms.ExponentialBackoffConfig{
		InitialInterval: time.Duration(retry.InitialDelay) * time.Millisecond,
		MaxInterval:     time.Duration(retry.MaxDelay) * time.Millisecond,
		Multiplier:      retry.BackoffMult,
		MaxRetries:      uint64(attempts),
	})
	return backoff.Retry(fn, backoff.WithContext(bo, ctx))
}

// reconnectWithBackoff retries sub.Reconnect with exponential delay until
// success, a non-retriable error (returned as is) or ctx cancellation.  Delay starts at DefaultReconnectDelay and
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
				if !isRetriable(err) {
					// e.g. ERR_MQ_AUTH_FAILED: retrying cannot succeed.
					return err
				}
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

// asAppError returns err unchanged when it already is (or wraps) an
// *errorkit.AppError, so SDK codes and their retriability reach the caller
// (AGENTS.md §2.1). Any other error is wrapped once with code and reason; a
// deadline is reported as ERR_MQ_TIMEOUT.
func asAppError(err error, code errorkit.ErrorCode, reason string) error {
	var appErr *errorkit.AppError
	if errors.As(err, &appErr) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		code = ERR_MQ_TIMEOUT
	}
	return errorkit.NewError(code).With(
		errorkit.WithReason("%s", reason),
		errorkit.WithWrapped(err),
	)
}
