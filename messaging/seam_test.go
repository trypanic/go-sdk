package messaging

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The publisher talks to the broker only through the unexported seam, so its
// connect/publish path can be exercised without a broker.
func TestPublisherUsesSeam(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	p := &Publisher{amqpURL: "amqp://fake", exchanges: []Exchange{{Name: "events", Type: "topic"}}, dial: b.dial}
	require.NoError(t, p.connect())

	require.NoError(t, p.PublishWithHeaders("events", "a", []byte(`{}`), nil))

	assert.Equal(t, 1, b.dialCount())
	assert.Equal(t, []string{"events"}, b.exchanges)
	got := b.publishes()
	require.Len(t, got, 1)
	assert.Equal(t, "a", got[0].RoutingKey)
}
