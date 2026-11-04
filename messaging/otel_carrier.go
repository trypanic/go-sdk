// messaging/otel_carrier.go
package messaging

import (
	"fmt"
	"maps"
	"slices"

	amqp "github.com/rabbitmq/amqp091-go"
)

type amqpHeadersCarrier struct {
	h amqp.Table
}

func newCarrier(h amqp.Table) amqpHeadersCarrier {
	if h == nil {
		h = amqp.Table{}
	}
	return amqpHeadersCarrier{h: h}
}

func (c amqpHeadersCarrier) Get(key string) string {
	v, ok := c.h[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprint(t)
	}
}

func (c amqpHeadersCarrier) Set(key, value string) {
	c.h[key] = value
}

func (c amqpHeadersCarrier) Keys() []string {
	return slices.Collect(maps.Keys(c.h))
}
