package messaging

import (
	"errors"
	"fmt"
	"sync"

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
type Publisher struct {
	mu        sync.Mutex
	conn      *amqp.Connection
	channel   *amqp.Channel
	exchanges []Exchange // declared on every (re)connect
	amqpURL   string
}

// NewPublisher creates a Publisher and declares all provided exchanges on the broker.
func NewPublisher(amqpURL string, exchanges []Exchange) (*Publisher, error) {
	p := &Publisher{
		amqpURL:   amqpURL,
		exchanges: exchanges,
	}
	if err := p.connect(); err != nil {
		return nil, errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
			errorkit.WithReason("failed to connect"),
			errorkit.WithWrapped(err),
		)
	}
	return p, nil
}

func (p *Publisher) connect() error {
	conn, err := amqp.DialConfig(p.amqpURL, amqp.Config{
		Heartbeat: DefaultHeartbeat,
	})
	if err != nil {
		return errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
			errorkit.WithReason("failed to connect"),
			errorkit.WithWrapped(err),
		)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
			errorkit.WithReason("failed to open channel"),
			errorkit.WithWrapped(err),
		)
	}

	for _, ex := range p.exchanges {
		if err := ch.ExchangeDeclare(
			ex.Name, ex.Type, ex.Durable, ex.AutoDelete,
			false, // internal
			false, // noWait
			nil,
		); err != nil {
			conn.Close()
			return errorkit.NewError(ERR_MQ_EXCHANGE_ERROR).With(
				errorkit.WithReason("failed to declare exchange"),
				errorkit.WithWrapped(err),
			)
		}
	}

	p.conn = conn
	p.channel = ch
	return nil
}

func (p *Publisher) reconnect() error {
	if p.conn != nil && !p.conn.IsClosed() {
		p.conn.Close()
	}
	return p.connect()
}

// Publish sends body to the given exchange with the specified routing key.
// It reconnects automatically on a closed connection or a mid-flight error
// (single retry per call; higher-level backoff is handled by PubSub).
func (p *Publisher) Publish(exchange, routingKey string, body []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn == nil || p.conn.IsClosed() {
		logger.Info("publisher connection closed, reconnecting")
		if err := p.reconnect(); err != nil {
			return errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
				errorkit.WithReason("failed to reconnect"),
				errorkit.WithWrapped(err),
			)
		}
		logger.Info("publisher reconnected")
	}

	err := p.channel.Publish(exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
	})
	if err != nil {
		// Channel may have died between the connection check and Publish.
		// Reconnect once and retry.
		logger.Info("publish failed, attempting reconnect + single retry. error %s", err)
		if reconnErr := p.reconnect(); reconnErr != nil {
			return errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
				errorkit.WithReason("failed to reconnect"),
				errorkit.WithWrapped(reconnErr),
			)
		}
		if err := p.channel.Publish(exchange, routingKey, false, false, amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
		}); err != nil {
			return errorkit.NewError(ERR_MQ_PUBLISH_FAILED).With(
				errorkit.WithReason("failed to publish message after reconnect"),
				errorkit.WithWrapped(err),
			)
		}
		return nil
	}
	return nil
}

func (p *Publisher) PublishWithHeaders(exchange, routingKey string, body []byte, headers amqp.Table) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.conn == nil || p.conn.IsClosed() {
		logger.Info("publisher connection closed, reconnecting")
		if err := p.reconnect(); err != nil {
			return errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
				errorkit.WithReason("failed to reconnect"),
				errorkit.WithWrapped(err),
			)
		}
	}

	pub := amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
		Headers:      headers,
	}

	if err := p.channel.Publish(exchange, routingKey, false, false, pub); err != nil {
		logger.Info("publish failed, attempting reconnect + single retry. error %s", err)
		if reconnErr := p.reconnect(); reconnErr != nil {
			return errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
				errorkit.WithReason("failed to reconnect"),
				errorkit.WithWrapped(reconnErr),
			)
		}
		if err := p.channel.Publish(exchange, routingKey, false, false, pub); err != nil {
			return errorkit.NewError(ERR_MQ_PUBLISH_FAILED).With(
				errorkit.WithReason("failed to publish message with headers after reconnect"),
				errorkit.WithWrapped(err),
			)
		}
		return nil
	}
	return nil
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
	conn        *amqp.Connection
	channel     *amqp.Channel
	exchange    Exchange
	queue       QueueTopology
	consumer    string
	queueArgs   amqp.Table
	routingKeys []string
	amqpURL     string
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
	conn, err := amqp.DialConfig(s.amqpURL, amqp.Config{
		Heartbeat: DefaultHeartbeat,
	})
	if err != nil {
		return errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
			errorkit.WithReason("failed to connect"),
			errorkit.WithWrapped(err),
		)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
			errorkit.WithReason("failed to open channel"),
			errorkit.WithWrapped(err),
		)
	}

	// Declare exchange (idempotent — ensures it exists after a reconnect).
	if err := ch.ExchangeDeclare(
		s.exchange.Name, s.exchange.Type, s.exchange.Durable, s.exchange.AutoDelete,
		false, false, nil,
	); err != nil {
		conn.Close()
		return errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
			errorkit.WithReason("failed to declare exchange"),
			errorkit.WithWrapped(err),
		)
	}

	// Declare queue with the full set of arguments from the topology.
	if _, err := ch.QueueDeclare(
		s.queue.Name, s.queue.Durable, s.queue.AutoDelete,
		false, // exclusive
		false, // noWait
		s.queueArgs,
	); err != nil {
		conn.Close()
		return errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
			errorkit.WithReason("failed to declare queue"),
			errorkit.WithWrapped(err),
		)
	}

	// Bind using the exact routing keys from the topology — never a wildcard.
	for _, rk := range s.routingKeys {
		if err := ch.QueueBind(s.queue.Name, rk, s.exchange.Name, false, nil); err != nil {
			conn.Close()
			return errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
				errorkit.WithReason("failed to bind queue"),
				errorkit.WithWrapped(err),
			)
		}
	}

	if err := ch.Qos(s.queue.Prefetch, 0, false); err != nil {
		conn.Close()
		return errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
			errorkit.WithReason("failed to set QoS"),
			errorkit.WithWrapped(err),
		)
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
	err := s.connect()
	if err != nil {
		return errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
			errorkit.WithReason("failed to reconnect"),
			errorkit.WithWrapped(err),
		)
	}
	return nil
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
		return nil, errorkit.NewError(ERR_MQ_CONNECTION_FAILED).With(
			errorkit.WithReason("failed to start consuming"),
			errorkit.WithWrapped(err),
		)
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
