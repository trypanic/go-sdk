package messaging

import (
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
	Publish(exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
	Close() error
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

// open opens a connection to url with the package defaults, through d when
// it is non-nil.
func (d dialFunc) open(url string) (amqpConnection, error) {
	if d == nil {
		d = dialAMQP
	}
	return d(url, amqp.Config{Heartbeat: DefaultHeartbeat})
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
	return ch, nil
}

func (c realConnection) IsClosed() bool { return c.conn.IsClosed() }

func (c realConnection) Close() error { return c.conn.Close() }
