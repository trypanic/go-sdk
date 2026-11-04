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
// Publishes on the shared channel run one at a time. Waiting for that turn,
// reconnecting and publishing all honour the caller's ctx, and the dial never
// runs under the mutex that guards the connection, so Close and callers with
// a deadline are never stuck behind an unreachable broker.
type Publisher struct {
	// turn serializes use of the publishing channel, including reconnects
	// (so at most one dial is in flight). Acquiring it honours ctx.
	turnOnce sync.Once
	turn     chan struct{}

	mu        sync.Mutex // guards conn and channel; never held across a dial
	conn      amqpConnection
	channel   amqpChannel
	exchanges []Exchange // declared on every (re)connect
	amqpURL   string
	dial      dialFunc // nil means dialAMQP
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
	conn, ch, err := p.open(context.Background())
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.conn, p.channel = conn, ch
	p.mu.Unlock()
	return nil
}

// open dials a new connection, opens a channel and declares the exchanges.
// It does not touch the publisher state.
func (p *Publisher) open(ctx context.Context) (amqpConnection, amqpChannel, error) {
	conn, err := p.dial.open(ctx, p.amqpURL)
	if err != nil {
		return nil, nil, mqError(ERR_MQ_CONNECTION_FAILED, "failed to connect", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, mqError(ERR_MQ_CHANNEL_ERROR, "failed to open channel", err)
	}

	for _, ex := range p.exchanges {
		if err := ch.ExchangeDeclare(
			ex.Name, ex.Type, ex.Durable, ex.AutoDelete,
			false, // internal
			false, // noWait
			nil,
		); err != nil {
			conn.Close()
			return nil, nil, mqError(ERR_MQ_EXCHANGE_ERROR, "failed to declare exchange", err)
		}
	}
	return conn, ch, nil
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

// currentChannel returns the open channel, reconnecting first when the
// connection is missing or closed. The caller holds the publishing turn.
func (p *Publisher) currentChannel(ctx context.Context) (amqpChannel, error) {
	p.mu.Lock()
	if p.conn != nil && !p.conn.IsClosed() {
		ch := p.channel
		p.mu.Unlock()
		return ch, nil
	}
	p.mu.Unlock()

	logger.Info("publisher connection closed, reconnecting")
	return p.reconnect(ctx)
}

// reconnect replaces the connection. The caller holds the publishing turn,
// so reconnects are single-flight; mu is held only to swap the state, never
// across the dial.
func (p *Publisher) reconnect(ctx context.Context) (amqpChannel, error) {
	if err := ctx.Err(); err != nil {
		return nil, publishCtxError(err)
	}

	p.mu.Lock()
	old := p.conn
	p.mu.Unlock()
	if old != nil && !old.IsClosed() {
		old.Close()
	}

	conn, ch, err := p.open(ctx)
	if err != nil {
		if ctxErr := ctxEnded(ctx); ctxErr != nil {
			return nil, publishCtxError(ctxErr)
		}
		return nil, err
	}

	p.mu.Lock()
	p.conn, p.channel = conn, ch
	p.mu.Unlock()
	return ch, nil
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
// using routingKey, as a persistent application/json message. It reconnects
// first when the connection is closed and, when the publish itself fails,
// reconnects once more and retries. ctx bounds the wait for the publishing
// turn, the reconnect and the publish; when it ends the returned error
// wraps ctx.Err() (ERR_MQ_TIMEOUT for a deadline).
func (p *Publisher) PublishWithContext(ctx context.Context, exchange, routingKey string, body []byte, headers amqp.Table) error {
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

	ch, err := p.currentChannel(ctx)
	if err != nil {
		return err
	}

	if err := ch.PublishWithContext(ctx, exchange, routingKey, false, false, pub); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return publishCtxError(ctxErr)
		}
		// Channel may have died between the connection check and Publish.
		// Reconnect once and retry.
		logger.Info("publish failed, attempting reconnect + single retry. error %s", err)
		ch, err = p.reconnect(ctx)
		if err != nil {
			return err
		}
		if err := ch.PublishWithContext(ctx, exchange, routingKey, false, false, pub); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return publishCtxError(ctxErr)
			}
			return mqError(ERR_MQ_PUBLISH_FAILED, "failed to publish message after reconnect", err)
		}
	}
	return nil
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
// so that a channel-close failure does not leak the connection.
func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

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
type Subscriber struct {
	conn        amqpConnection
	channel     amqpChannel
	exchange    Exchange
	queue       QueueTopology
	consumer    string
	queueArgs   amqp.Table
	routingKeys []string
	amqpURL     string
	dial        dialFunc // nil means dialAMQP
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
	conn, err := s.dial.open(context.Background(), s.amqpURL)
	if err != nil {
		return mqError(ERR_MQ_CONNECTION_FAILED, "failed to connect", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return mqError(ERR_MQ_CHANNEL_ERROR, "failed to open channel", err)
	}

	// Declare exchange (idempotent — ensures it exists after a reconnect).
	if err := ch.ExchangeDeclare(
		s.exchange.Name, s.exchange.Type, s.exchange.Durable, s.exchange.AutoDelete,
		false, false, nil,
	); err != nil {
		conn.Close()
		return mqError(ERR_MQ_EXCHANGE_ERROR, "failed to declare exchange", err)
	}

	// Declare queue with the full set of arguments from the topology.
	if _, err := ch.QueueDeclare(
		s.queue.Name, s.queue.Durable, s.queue.AutoDelete,
		false, // exclusive
		false, // noWait
		s.queueArgs,
	); err != nil {
		conn.Close()
		return mqError(ERR_MQ_QUEUE_ERROR, "failed to declare queue", err)
	}

	// Bind using the exact routing keys from the topology — never a wildcard.
	for _, rk := range s.routingKeys {
		if err := ch.QueueBind(s.queue.Name, rk, s.exchange.Name, false, nil); err != nil {
			conn.Close()
			return mqError(ERR_MQ_QUEUE_ERROR, "failed to bind queue", err)
		}
	}

	if err := ch.Qos(s.queue.Prefetch, 0, false); err != nil {
		conn.Close()
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
		s.conn.Close()
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

// Close gracefully closes the channel and connection. Both are always attempted.
func (s *Subscriber) Close() error {
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
	var appErr *errorkit.AppError
	if errors.As(err, &appErr) {
		return appErr.Metadata.Retriable
	}
	return true
}
