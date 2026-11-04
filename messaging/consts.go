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

// DefaultPrefetch is the QoS prefetch count a subscriber uses when the queue
// topology omits prefetch (or sets it to 0). Handlers run one message at a
// time, so a larger window only moves the backlog from the broker into
// process memory; 0 would mean unlimited.
const DefaultPrefetch = 1
