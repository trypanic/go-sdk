package messaging

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/logger"
)

// ---------------------------------------------------------------------------
// Publisher
// ---------------------------------------------------------------------------

// Publisher publishes messages to AMQP exchanges. It is safe for concurrent use.
// If the underlying connection drops, Publish reconnects transparently before
// (or after) the attempt.
//
// The publishing channel runs in confirm mode and every message is published
// as mandatory: Publish returns only once the broker has confirmed the
// message, and reports a nack or an unroutable message (basic.return) as
// ERR_MQ_PUBLISH_FAILED.
//
// Publishes on the shared channel run one at a time. Waiting for that turn,
// reconnecting and publishing all honour the caller's ctx, and the dial never
// runs under the mutex that guards the connection, so Close and callers with
// a deadline are never stuck behind an unreachable broker.
type Publisher struct {
	// turn serializes use of the publishing channel, including reconnects
	// (so at most one dial is in flight). Acquiring it honours ctx.
	turnOnce sync.Once
	turn     chan struct{}

	mu        sync.Mutex // guards conn, channel, returns and broken; never held across a dial
	conn      amqpConnection
	channel   amqpChannel
	returns   chan amqp.Return // basic.return notifications of channel
	broken    bool             // channel must not be reused (abandoned confirm)
	closed    bool             // Close was called; terminal
	exchanges []Exchange       // declared on every (re)connect
	declare   *Topology        // when non-nil, its queues and bindings are declared on every (re)connect
	opts      connOptions
	log       *logger.Logger // nil means the ctx/global logger
	amqpURL   string
	dial      dialFunc // nil means dialAMQP
}

// publisherLink is one open connection with its publishing channel.
type publisherLink struct {
	conn    amqpConnection
	channel amqpChannel
	returns chan amqp.Return
}

// NewPublisher creates a Publisher and declares all provided exchanges on the broker.
func NewPublisher(amqpURL string, exchanges []Exchange) (*Publisher, error) {
	p := &Publisher{
		amqpURL:   amqpURL,
		exchanges: exchanges,
	}
	if err := p.connect(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Publisher) connect() error {
	return p.connectContext(context.Background())
}

func (p *Publisher) connectContext(ctx context.Context) error {
	timeout := p.opts.dialTimeout
	if timeout <= 0 {
		timeout = DefaultDialTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	l, err := p.open(ctx)
	if ctxErr := ctxEnded(ctx); ctxErr != nil {
		if l.conn != nil {
			abandon(l.conn)
		}
		return mqError(ERR_MQ_CONNECTION_FAILED, "connection setup interrupted", ctxErr)
	}
	if err != nil {
		return err
	}
	return p.swap(l)
}

// swap installs l as the current link. After Close it closes l instead and
// reports ERR_MQ_CLOSED, so a reconnect racing with Close does not leak.
func (p *Publisher) swap(l publisherLink) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		_ = l.conn.Close() // never used; the closed state is the error to report
		return errPublisherClosed()
	}
	p.conn, p.channel, p.returns, p.broken = l.conn, l.channel, l.returns, false
	return nil
}

func errPublisherClosed() *errorkit.AppError {
	return errorkit.NewError(ERR_MQ_CLOSED).With(errorkit.WithReason("publisher is closed"))
}

// open dials a new connection, opens a channel in confirm mode and declares
// the exchanges (and, with declare, the queues and bindings). It does not
// touch the publisher state.
func (p *Publisher) open(ctx context.Context) (publisherLink, error) {
	conn, err := p.dial.open(ctx, p.amqpURL, p.opts)
	if err != nil {
		return publisherLink{}, mqError(ERR_MQ_CONNECTION_FAILED, "failed to connect", err)
	}

	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		if real, ok := conn.(realConnection); ok {
			_ = real.conn.CloseDeadline(time.Now())
		} else {
			_ = conn.Close()
		}
		close(done)
	})
	defer func() {
		if !stop() {
			<-done
		}
	}()
	ch, err := conn.Channel()
	if err != nil {
		abandon(conn)
		return publisherLink{}, mqError(ERR_MQ_CHANNEL_ERROR, "failed to open channel", err)
	}

	if err := ch.Confirm(false); err != nil {
		abandon(conn)
		return publisherLink{}, mqError(ERR_MQ_CHANNEL_ERROR, "failed to enable publisher confirms", err)
	}
	// Publishes run one at a time and each one drains this channel, so one
	// slot is enough; amqp091-go fills it before it delivers the ack.
	returns := ch.NotifyReturn(make(chan amqp.Return, 1))

	for _, ex := range p.exchanges {
		if err := ch.ExchangeDeclare(
			ex.Name, ex.Type, ex.Durable, ex.AutoDelete,
			false, // internal
			false, // noWait
			nil,
		); err != nil {
			abandon(conn)
			return publisherLink{}, mqError(ERR_MQ_EXCHANGE_ERROR, "failed to declare exchange", err)
		}
	}
	if p.declare != nil {
		if err := declareQueues(ch, p.declare); err != nil {
			abandon(conn)
			return publisherLink{}, err
		}
	}
	return publisherLink{conn: conn, channel: ch, returns: returns}, nil
}

// declareQueues declares every queue of topo, with the same arguments
// Subscribe uses, and every binding.
func declareQueues(ch amqpChannel, topo *Topology) error {
	for _, q := range topo.Queues {
		if _, err := ch.QueueDeclare(q.Name, q.Durable, q.AutoDelete, false, false, buildQueueArgs(q)); err != nil {
			return mqError(ERR_MQ_QUEUE_ERROR, "failed to declare queue", err)
		}
	}
	for _, b := range topo.Bindings {
		for _, rk := range b.RoutingKeys {
			if err := ch.QueueBind(b.Queue, rk, b.Exchange, false, nil); err != nil {
				return mqError(ERR_MQ_QUEUE_ERROR, "failed to bind queue", err)
			}
		}
	}
	return nil
}

// acquire waits for the publishing turn or for ctx to end.
func (p *Publisher) acquire(ctx context.Context) error {
	p.turnOnce.Do(func() { p.turn = make(chan struct{}, 1) })
	if err := ctx.Err(); err != nil {
		return publishCtxError(err)
	}
	select {
	case p.turn <- struct{}{}:
		return nil
	case <-ctx.Done():
		return publishCtxError(ctx.Err())
	}
}

func (p *Publisher) release() { <-p.turn }

// currentLink returns the open link, reconnecting first when the connection
// is missing or closed or the channel is broken. The caller holds the
// publishing turn.
func (p *Publisher) currentLink(ctx context.Context) (publisherLink, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return publisherLink{}, errPublisherClosed()
	}
	if p.conn != nil && !p.conn.IsClosed() && !p.broken {
		l := publisherLink{conn: p.conn, channel: p.channel, returns: p.returns}
		p.mu.Unlock()
		return l, nil
	}
	p.mu.Unlock()

	warnEvent(ctx, p.log).Msg("messaging: publisher connection closed, reconnecting")
	return p.reconnect(ctx)
}

// reconnect replaces the connection. The caller holds the publishing turn,
// so reconnects are single-flight; mu is held only to swap the state, never
// across the dial.
func (p *Publisher) reconnect(ctx context.Context) (publisherLink, error) {
	if err := ctx.Err(); err != nil {
		return publisherLink{}, publishCtxError(err)
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return publisherLink{}, errPublisherClosed()
	}
	// Detach the old link before closing it outside mu, so a concurrent
	// Close does not close it a second time.
	old := p.conn
	p.conn, p.channel, p.returns = nil, nil, nil
	p.mu.Unlock()
	if old != nil && !old.IsClosed() {
		if err := old.Close(); err != nil {
			warnEvent(ctx, p.log).Err(err).Msg("messaging: closing the previous publisher connection failed")
		}
	}

	l, err := p.open(ctx)
	if err != nil {
		if ctxErr := ctxEnded(ctx); ctxErr != nil {
			return publisherLink{}, publishCtxError(ctxErr)
		}
		return publisherLink{}, err
	}

	if err := p.swap(l); err != nil {
		return publisherLink{}, err
	}
	return l, nil
}

// Publish sends body to the given exchange with the specified routing key.
// It reconnects automatically on a closed connection or a mid-flight error
// (single retry per call; higher-level backoff is handled by PubSub).
// It is PublishWithContext with context.Background() and no headers.
func (p *Publisher) Publish(exchange, routingKey string, body []byte) error {
	return p.PublishWithContext(context.Background(), exchange, routingKey, body, nil)
}

// PublishWithHeaders is PublishWithContext with context.Background().
func (p *Publisher) PublishWithHeaders(exchange, routingKey string, body []byte, headers amqp.Table) error {
	return p.PublishWithContext(context.Background(), exchange, routingKey, body, headers)
}

// PublishWithContext sends body with the given AMQP headers to exchange
// using routingKey, as a persistent, mandatory application/json message, and
// waits for the broker's confirm. It reconnects first when the connection is
// closed and, when the publish itself fails, reconnects once more and
// retries. A nack or an unroutable message (basic.return) is reported as
// ERR_MQ_PUBLISH_FAILED. ctx bounds the wait for the publishing turn, the
// reconnect, the publish and the confirm; when it ends the returned error
// wraps ctx.Err() (ERR_MQ_TIMEOUT for a deadline).
func (p *Publisher) PublishWithContext(ctx context.Context, exchange, routingKey string, body []byte, headers amqp.Table) error {
	return p.publish(ctx, exchange, routingKey, body, headers, true)
}

// publish is one publish attempt. With retryOnce, a failed publish
// reconnects and retries once; without it the failure is returned and the
// caller's retry policy is the only retry layer. Either way a channel whose
// publish failed is not reused.
func (p *Publisher) publish(ctx context.Context, exchange, routingKey string, body []byte, headers amqp.Table, retryOnce bool) error {
	if err := p.acquire(ctx); err != nil {
		return err
	}
	defer p.release()

	pub := amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
		Headers:      headers,
	}

	l, err := p.currentLink(ctx)
	if err != nil {
		return err
	}

	drainReturns(l.returns)
	conf, err := l.channel.PublishDeferred(ctx, exchange, routingKey, true, pub)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return publishCtxError(ctxErr)
		}
		if !retryOnce {
			p.markBroken()
			return mqError(ERR_MQ_PUBLISH_FAILED, "failed to publish message", err)
		}
		// Channel may have died between the connection check and Publish.
		// Reconnect once and retry.
		warnEvent(ctx, p.log).Err(err).Str("exchange", exchange).Str("routing_key", routingKey).
			Msg("messaging: publish failed, reconnecting for a single retry")
		l, err = p.reconnect(ctx)
		if err != nil {
			return err
		}
		conf, err = l.channel.PublishDeferred(ctx, exchange, routingKey, true, pub)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return publishCtxError(ctxErr)
			}
			p.markBroken()
			return mqError(ERR_MQ_PUBLISH_FAILED, "failed to publish message after reconnect", err)
		}
	}
	return p.awaitConfirm(ctx, conf, l)
}

// markBroken makes the next publish replace the current link.
func (p *Publisher) markBroken() {
	p.mu.Lock()
	p.broken = true
	p.mu.Unlock()
}

// errNotConfirmed is wrapped when the broker nacks a message or the channel
// closes before confirming it.
var errNotConfirmed = errors.New("broker did not confirm the message (nack or channel closed)")

// awaitConfirm waits for the broker's answer to one publish. RabbitMQ sends
// basic.return before the ack of an unroutable mandatory message, so a
// return present once the ack has arrived belongs to this publish.
func (p *Publisher) awaitConfirm(ctx context.Context, conf publishConfirmation, l publisherLink) error {
	acked, err := conf.WaitContext(ctx)
	if err != nil {
		// The answer may still arrive: do not let the next publish read it.
		p.markBroken()
		return publishCtxError(err)
	}
	if !acked {
		if l.channel.IsClosed() {
			// A channel-level exception closed it: do not waste the next
			// attempt on it.
			p.markBroken()
		}
		return mqError(ERR_MQ_PUBLISH_FAILED, "message not confirmed", errNotConfirmed)
	}
	select {
	case r, ok := <-l.returns:
		if !ok { // channel closed after the ack: nothing was returned
			return nil
		}
		return mqError(ERR_MQ_PUBLISH_FAILED, "message returned by the broker: "+r.ReplyText,
			fmt.Errorf("unroutable message: %d %s (exchange %q, routing key %q)", r.ReplyCode, r.ReplyText, r.Exchange, r.RoutingKey))
	default:
		return nil
	}
}

// drainReturns discards returns left over from earlier publishes (a return
// followed by a nack), so they are not attributed to the next one.
func drainReturns(returns <-chan amqp.Return) {
	for {
		select {
		case _, ok := <-returns:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

// ctxEnded returns ctx.Err(), or context.DeadlineExceeded once the ctx
// deadline has passed even if the ctx timer has not fired yet (a socket
// deadline set from it can expire first).
func ctxEnded(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return nil
}

// publishCtxError reports an ended ctx: ERR_MQ_TIMEOUT for a deadline,
// ERR_MQ_PUBLISH_FAILED for a cancellation. errors.Is(err, ctx.Err()) holds.
func publishCtxError(err error) *errorkit.AppError {
	code := ERR_MQ_PUBLISH_FAILED
	if errors.Is(err, context.DeadlineExceeded) {
		code = ERR_MQ_TIMEOUT
	}
	return errorkit.NewError(code).With(
		errorkit.WithReason("publish aborted: %v", err),
		errorkit.WithWrapped(err),
	)
}

// Close gracefully closes the channel and connection. Both are always attempted
// so that a channel-close failure does not leak the connection. Close is
// terminal: later publishes fail with ERR_MQ_CLOSED and never re-dial, and a
// reconnect in progress closes its new connection. Calling Close again
// returns nil.
func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true

	var errs []error
	if p.channel != nil {
		if err := p.channel.Close(); err != nil {
			errs = append(errs, fmt.Errorf("publisher channel close: %w", err))
		}
	}
	if p.conn != nil {
		if err := p.conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("publisher connection close: %w", err))
		}
	}
	if len(errs) > 0 {
		return errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
			errorkit.WithReason("failed to close connection"),
			errorkit.WithWrapped(errors.Join(errs...)),
		)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Subscriber
// ---------------------------------------------------------------------------

// Subscriber consumes messages from a single AMQP queue.  Reconnect re-builds
// the full connection + channel + bindings so the caller only needs to call
// Consume again after a successful Reconnect.
//
// A Subscriber is not safe for concurrent use: Consume, Reconnect and Close
// read and replace its connection without a lock, so drive it from one
// goroutine (PubSub.Subscribe does).
type Subscriber struct {
	conn        amqpConnection
	channel     amqpChannel
	exchange    Exchange
	queue       QueueTopology
	consumer    string
	queueArgs   amqp.Table
	routingKeys []string
	amqpURL     string
	opts        connOptions
	log         *logger.Logger // nil means the global logger
	dial        dialFunc       // nil means dialAMQP
}

// NewSubscriber dials the broker, declares the exchange and queue, binds the
// queue using the provided routing keys, and sets the channel QoS.
func NewSubscriber(
	amqpURL string,
	exchange Exchange,
	queue QueueTopology,
	consumer string,
	queueArgs amqp.Table,
	routingKeys []string,
) (*Subscriber, error) {
	s := &Subscriber{
		amqpURL:     amqpURL,
		exchange:    exchange,
		queue:       queue,
		consumer:    consumer,
		queueArgs:   queueArgs,
		routingKeys: routingKeys,
	}
	if err := s.connect(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Subscriber) connect() error {
	conn, err := s.dial.open(context.Background(), s.amqpURL, s.opts)
	if err != nil {
		return mqError(ERR_MQ_CONNECTION_FAILED, "failed to connect", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		abandon(conn)
		return mqError(ERR_MQ_CHANNEL_ERROR, "failed to open channel", err)
	}

	// Declare exchange (idempotent — ensures it exists after a reconnect).
	if err := ch.ExchangeDeclare(
		s.exchange.Name, s.exchange.Type, s.exchange.Durable, s.exchange.AutoDelete,
		false, false, nil,
	); err != nil {
		abandon(conn)
		return mqError(ERR_MQ_EXCHANGE_ERROR, "failed to declare exchange", err)
	}

	// Declare queue with the full set of arguments from the topology.
	if _, err := ch.QueueDeclare(
		s.queue.Name, s.queue.Durable, s.queue.AutoDelete,
		false, // exclusive
		false, // noWait
		s.queueArgs,
	); err != nil {
		abandon(conn)
		return mqError(ERR_MQ_QUEUE_ERROR, "failed to declare queue", err)
	}

	// Bind using the exact routing keys from the topology — never a wildcard.
	for _, rk := range s.routingKeys {
		if err := ch.QueueBind(s.queue.Name, rk, s.exchange.Name, false, nil); err != nil {
			abandon(conn)
			return mqError(ERR_MQ_QUEUE_ERROR, "failed to bind queue", err)
		}
	}

	prefetch := s.queue.Prefetch
	if prefetch <= 0 {
		prefetch = DefaultPrefetch // 0 would mean unlimited
	}
	if err := ch.Qos(prefetch, 0, false); err != nil {
		abandon(conn)
		return mqError(ERR_MQ_CHANNEL_ERROR, "failed to set QoS", err)
	}

	s.conn = conn
	s.channel = ch
	return nil
}

// Reconnect tears down the current connection and rebuilds everything.
// It is called by PubSub when the delivery channel closes unexpectedly.
func (s *Subscriber) Reconnect() error {
	if s.conn != nil && !s.conn.IsClosed() {
		if err := s.conn.Close(); err != nil {
			warnEvent(context.Background(), s.log).Err(err).Str("queue", s.queue.Name).
				Msg("messaging: closing the previous subscriber connection failed")
		}
	}
	return s.connect()
}

// Consume starts (or resumes) consuming from the queue and returns the
// delivery channel.  Call again after a successful Reconnect.
func (s *Subscriber) Consume() (<-chan amqp.Delivery, error) {
	msgs, err := s.channel.Consume(
		s.queue.Name, s.consumer,
		false, // autoAck  — we ack/nack explicitly
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,
	)
	if err != nil {
		return nil, mqError(ERR_MQ_CONSUME_FAILED, "failed to start consuming", err)
	}
	return msgs, nil
}

// Close gracefully closes the channel and connection. Both are always
// attempted. A connection that is already closed (the broker dropped it) has
// nothing left to release, so Close returns nil.
func (s *Subscriber) Close() error {
	if s.conn != nil && s.conn.IsClosed() {
		return nil
	}
	var errs []error
	if s.channel != nil {
		if err := s.channel.Close(); err != nil {
			errs = append(errs, fmt.Errorf("subscriber channel close: %w", err))
		}
	}
	if s.conn != nil {
		if err := s.conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("subscriber connection close: %w", err))
		}
	}
	if len(errs) > 0 {
		return errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
			errorkit.WithReason("failed to close connection"),
			errorkit.WithWrapped(errors.Join(errs...)),
		)
	}
	return nil
}

// abandon closes a connection whose setup failed. The setup error is the
// one returned to the caller, so the close error is deliberately dropped.
func abandon(conn amqpConnection) {
	_ = conn.Close()
}

// mqError builds the AppError for a failed AMQP operation. An access-refused
// reply (403: bad credentials, vhost or resource permissions) is reported as
// ERR_MQ_AUTH_FAILED, which is not retriable, whatever the operation was.
func mqError(code errorkit.ErrorCode, reason string, err error) *errorkit.AppError {
	var amqpErr *amqp.Error
	if errors.As(err, &amqpErr) && amqpErr.Code == amqp.AccessRefused {
		code = ERR_MQ_AUTH_FAILED
	}
	return errorkit.NewError(code).With(
		errorkit.WithReason("%s", reason),
		errorkit.WithWrapped(err),
	)
}

// isRetriable reports whether err is worth retrying: an AppError answers
// through its metadata, any other error is assumed transient.
func isRetriable(err error) bool {
	if appErr, ok := errorkit.As(err); ok {
		return appErr.Metadata.Retriable
	}
	return true
}
