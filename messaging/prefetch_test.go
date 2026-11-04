package messaging

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// C5-003: an omitted prefetch no longer means "unlimited" (Qos 0); the
// subscriber asks for DefaultPrefetch, and an explicit value is kept.
func TestSubscriberPrefetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prefetch int
		want     int
	}{
		{"omitted uses the default", 0, DefaultPrefetch},
		{"explicit value is kept", 25, 25},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := newFakeBroker()
			s := &Subscriber{
				amqpURL:     "amqp://fake",
				exchange:    Exchange{Name: "events", Type: "topic"},
				queue:       QueueTopology{Name: "q", Prefetch: tc.prefetch},
				routingKeys: []string{"a"},
				dial:        b.dial,
			}
			require.NoError(t, s.connect())

			b.mu.Lock()
			defer b.mu.Unlock()
			assert.Equal(t, []int{tc.want}, b.qos)
		})
	}
	assert.Equal(t, 1, DefaultPrefetch, "handlers run one at a time, so one unacked message is enough")
}
