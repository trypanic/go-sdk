package messaging

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// amqpConnection is the subset of *amqp.Connection used by this package.
// It exists so that publisher and subscriber logic can be tested with fakes;
// production code always wraps a real *amqp.Connection.
type amqpConnection interface {
	Channel() (amqpChannel, error)
	IsClosed() bool
	Close() error
}

// amqpChannel is the subset of *amqp.Channel used by this package.
type amqpChannel interface {
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
	Qos(prefetchCount, prefetchSize int, global bool) error
	Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)
	Confirm(noWait bool) error
	NotifyReturn(c chan amqp.Return) chan amqp.Return
	// PublishDeferred publishes msg on a channel in confirm mode and returns
	// the broker's pending confirmation.
	PublishDeferred(ctx context.Context, exchange, key string, mandatory bool, msg amqp.Publishing) (publishConfirmation, error)
	Close() error
}

// publishConfirmation is the broker's pending answer (ack or nack) to one
// publish on a channel in confirm mode.
type publishConfirmation interface {
	// WaitContext reports whether the broker acked the message, or ctx.Err()
	// when ctx ends first.
	WaitContext(ctx context.Context) (bool, error)
}

// dialFunc opens an AMQP connection. nil means dialAMQP.
type dialFunc func(url string, cfg amqp.Config) (amqpConnection, error)

// dialAMQP dials a real broker with amqp.DialConfig.
func dialAMQP(url string, cfg amqp.Config) (amqpConnection, error) {
	conn, err := amqp.DialConfig(url, cfg)
	if err != nil {
		return nil, err
	}
	return realConnection{conn: conn}, nil
}

// connOptions are the connection settings shared by the publisher and the
// subscribers of a PubSub. Zero values mean the package defaults.
type connOptions struct {
	heartbeat   time.Duration
	dialTimeout time.Duration
	tls         *tls.Config
	name        string
}

// config builds the amqp.Config of one dial bounded by ctx.
func (o connOptions) config(ctx context.Context) amqp.Config {
	heartbeat := DefaultHeartbeat
	if o.heartbeat > 0 {
		heartbeat = o.heartbeat
	}
	timeout := DefaultDialTimeout
	if o.dialTimeout > 0 {
		timeout = o.dialTimeout
	}
	cfg := amqp.Config{
		Heartbeat:       heartbeat,
		TLSClientConfig: o.tls,
		Dial:            ctxDialer(ctx, timeout),
	}
	if o.name != "" {
		cfg.Properties = amqp.NewConnectionProperties()
		cfg.Properties.SetClientConnectionName(o.name)
	}
	return cfg
}

// open opens a connection to url with the options o, through d when it is
// non-nil. ctx bounds the TCP connect and the handshake; an ended ctx never
// dials.
func (d dialFunc) open(ctx context.Context, url string, o connOptions) (amqpConnection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d == nil {
		d = dialAMQP
	}
	return d(url, o.config(ctx))
}

// ctxDialer returns an amqp.Config.Dial func whose TCP connect is cancelled
// with ctx and whose TLS/AMQP handshake deadline is the earlier of timeout
// and the ctx deadline. amqp091-go clears the deadline once the connection
// is open.
func ctxDialer(ctx context.Context, timeout time.Duration) func(network, addr string) (net.Conn, error) {
	return func(network, addr string) (net.Conn, error) {
		d := net.Dialer{Timeout: timeout}
		conn, err := d.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		deadline := time.Now().Add(timeout)
		if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
			deadline = ctxDeadline
		}
		if err := conn.SetDeadline(deadline); err != nil {
			_ = conn.Close() // the SetDeadline error is the one returned
			return nil, err
		}
		return conn, nil
	}
}

// realConnection adapts *amqp.Connection to amqpConnection.
type realConnection struct {
	conn *amqp.Connection
}

func (c realConnection) Channel() (amqpChannel, error) {
	ch, err := c.conn.Channel()
	if err != nil {
		return nil, err
	}
	return realChannel{Channel: ch}, nil
}

// realChannel adapts *amqp.Channel to amqpChannel.
type realChannel struct {
	*amqp.Channel
}

func (c realChannel) PublishDeferred(ctx context.Context, exchange, key string, mandatory bool, msg amqp.Publishing) (publishConfirmation, error) {
	dc, err := c.PublishWithDeferredConfirmWithContext(ctx, exchange, key, mandatory, false, msg)
	if err != nil {
		return nil, err
	}
	if dc == nil {
		// The channel is not in confirm mode: the frame was written and no
		// broker answer will follow.
		return ackedConfirmation{}, nil
	}
	return dc, nil
}

// ackedConfirmation is an already positive confirmation.
type ackedConfirmation struct{}

func (ackedConfirmation) WaitContext(context.Context) (bool, error) { return true, nil }

func (c realConnection) IsClosed() bool { return c.conn.IsClosed() }

func (c realConnection) Close() error { return c.conn.Close() }
