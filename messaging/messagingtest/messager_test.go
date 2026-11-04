package messagingtest_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/messaging"
	"github.com/trypanic/go-sdk/messaging/messagingtest"
)

func TestConsumerUsesFake(t *testing.T) {
	var published string
	fake := &messagingtest.Broker{
		PublishFunc: func(_ context.Context, q string, e messaging.Event) error {
			published = q
			require.Equal(t, "id", e.CorrelationID)
			return nil
		},
		SubscribeFunc: func(ctx context.Context, q, s string, h messaging.MessageHandler) error {
			return h(ctx, messaging.Message{Raw: []byte("event")})
		},
		CloseFunc: func() error { return nil },
	}
	var port messaging.Broker = fake
	require.NoError(t, port.Publish(t.Context(), "jobs", messaging.Event{CorrelationID: "id"}))
	require.Equal(t, "jobs", published)
	require.NoError(t, port.Subscribe(t.Context(), "jobs", "worker", func(_ context.Context, m messaging.Message) error {
		require.Equal(t, []byte("event"), m.Raw)
		return nil
	}))
	require.NoError(t, port.Close())
	require.Panics(t, func() { _ = new(messagingtest.Broker).Close() })
}
