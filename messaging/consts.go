package messaging

import (
	"time"
)

const (
	// DefaultHeartbeat is the AMQP heartbeat interval used on every connection
	// to detect dead links early.
	DefaultHeartbeat = 60 * time.Second

	// DefaultReconnectDelay is the starting backoff interval when the subscriber
	// enters its reconnect loop after a connection drop.
	DefaultReconnectDelay = 5 * time.Second

	// MaxReconnectDelay caps the exponential backoff in the subscriber reconnect loop.
	MaxReconnectDelay = 60 * time.Second
)
