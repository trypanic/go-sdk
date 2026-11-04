package messaging

import (
	"context"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

// fakePublish is one message accepted by a fakeChannel.
type fakePublish struct {
	Exchange   string
	RoutingKey string
	Mandatory  bool
	Msg        amqp.Publishing
}

// fakeBroker is an in-memory stand-in for a RabbitMQ broker used through the
// unexported amqpConnection/amqpChannel seam. It records every call and
// lets a test inject failures. It never opens a socket.
type fakeBroker struct {
	mu sync.Mutex

	dials    int
	configs  []amqp.Config
	dialErr  error
	dialHook func() // runs (outside mu) at the start of every dial

	channelErr   error
	exchangeErr  error
	queueErr     error
	bindErr      error
	qosErr       error
	consumeErr   error
	connCloseErr error

	// publishErr, when set, decides the outcome of each publish attempt.
	publishErr func(attempt int, exchange, key string) error

	exchanges  []string
	queues     []string
	bindings   []string
	qos        []int
	published  []fakePublish
	attempts   int
	conns      []*fakeConn
	deliveries chan amqp.Delivery
}

func newFakeBroker() *fakeBroker {
	return &fakeBroker{deliveries: make(chan amqp.Delivery)}
}

func (b *fakeBroker) dial(_ string, cfg amqp.Config) (amqpConnection, error) {
	b.mu.Lock()
	hook := b.dialHook
	b.mu.Unlock()
	if hook != nil {
		hook()
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.dials++
	b.configs = append(b.configs, cfg)
	if b.dialErr != nil {
		return nil, b.dialErr
	}
	c := &fakeConn{b: b}
	b.conns = append(b.conns, c)
	return c, nil
}

func (b *fakeBroker) dialCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dials
}

func (b *fakeBroker) publishes() []fakePublish {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]fakePublish(nil), b.published...)
}

type fakeConn struct {
	b      *fakeBroker
	closed bool // guarded by b.mu
}

func (c *fakeConn) Channel() (amqpChannel, error) {
	c.b.mu.Lock()
	defer c.b.mu.Unlock()
	if c.b.channelErr != nil {
		return nil, c.b.channelErr
	}
	return &fakeChannel{b: c.b, conn: c}, nil
}

func (c *fakeConn) IsClosed() bool {
	c.b.mu.Lock()
	defer c.b.mu.Unlock()
	return c.closed
}

func (c *fakeConn) Close() error {
	c.b.mu.Lock()
	defer c.b.mu.Unlock()
	if c.closed {
		return amqp.ErrClosed
	}
	c.closed = true
	return c.b.connCloseErr
}

type fakeChannel struct {
	b    *fakeBroker
	conn *fakeConn
}

func (ch *fakeChannel) ExchangeDeclare(name, _ string, _, _, _, _ bool, _ amqp.Table) error {
	ch.b.mu.Lock()
	defer ch.b.mu.Unlock()
	if ch.b.exchangeErr != nil {
		return ch.b.exchangeErr
	}
	ch.b.exchanges = append(ch.b.exchanges, name)
	return nil
}

func (ch *fakeChannel) QueueDeclare(name string, _, _, _, _ bool, _ amqp.Table) (amqp.Queue, error) {
	ch.b.mu.Lock()
	defer ch.b.mu.Unlock()
	if ch.b.queueErr != nil {
		return amqp.Queue{}, ch.b.queueErr
	}
	ch.b.queues = append(ch.b.queues, name)
	return amqp.Queue{Name: name}, nil
}

func (ch *fakeChannel) QueueBind(name, key, exchange string, _ bool, _ amqp.Table) error {
	ch.b.mu.Lock()
	defer ch.b.mu.Unlock()
	if ch.b.bindErr != nil {
		return ch.b.bindErr
	}
	ch.b.bindings = append(ch.b.bindings, exchange+"->"+name+":"+key)
	return nil
}

func (ch *fakeChannel) Qos(prefetchCount, _ int, _ bool) error {
	ch.b.mu.Lock()
	defer ch.b.mu.Unlock()
	if ch.b.qosErr != nil {
		return ch.b.qosErr
	}
	ch.b.qos = append(ch.b.qos, prefetchCount)
	return nil
}

func (ch *fakeChannel) Consume(_, _ string, _, _, _, _ bool, _ amqp.Table) (<-chan amqp.Delivery, error) {
	ch.b.mu.Lock()
	defer ch.b.mu.Unlock()
	if ch.b.consumeErr != nil {
		return nil, ch.b.consumeErr
	}
	return ch.b.deliveries, nil
}

func (ch *fakeChannel) PublishWithContext(ctx context.Context, exchange, key string, mandatory, _ bool, msg amqp.Publishing) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ch.b.mu.Lock()
	defer ch.b.mu.Unlock()
	ch.b.attempts++
	if ch.conn.closed {
		return amqp.ErrClosed
	}
	if ch.b.publishErr != nil {
		if err := ch.b.publishErr(ch.b.attempts, exchange, key); err != nil {
			return err
		}
	}
	ch.b.published = append(ch.b.published, fakePublish{Exchange: exchange, RoutingKey: key, Mandatory: mandatory, Msg: msg})
	return nil
}

func (ch *fakeChannel) Close() error { return nil }
