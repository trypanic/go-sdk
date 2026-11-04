package messaging

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// multiKeyTopology binds queue "q" to exchange "events" with two routing
// keys and the given retry policy.
func multiKeyTopology(retry *RetryConfig) *Topology {
	return &Topology{
		Exchanges: []Exchange{{Name: "events", Type: "topic"}},
		Queues:    []QueueTopology{{Name: "q", Retry: retry}},
		Bindings:  []Binding{{Queue: "q", Exchange: "events", RoutingKeys: []string{"a", "b"}}},
	}
}

func routingKeys(pubs []fakePublish) []string {
	keys := make([]string, 0, len(pubs))
	for _, p := range pubs {
		keys = append(keys, p.RoutingKey)
	}
	return keys
}

// C5-009: a retry after a partial failure re-sends only the routing keys
// that have not been published yet.
func TestPublishRetryDoesNotResendCompletedKeys(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	failures := 0
	b.publishErr = func(_ int, _, key string) error {
		if key == "b" && failures < 2 { // outlasts any single in-call retry
			failures++
			return errors.New("transient")
		}
		return nil
	}
	retry := &RetryConfig{MaxAttempts: 3, InitialDelay: 1, MaxDelay: 1, BackoffMult: 1}
	p := newFakePubSub(b, multiKeyTopology(retry))

	require.NoError(t, p.Publish(context.Background(), "q", Event{}))

	assert.Equal(t, []string{"a", "b"}, routingKeys(b.publishes()), "key a must be published once")
}

// C5-009: with a retry policy, MaxAttempts is the number of publish attempts
// per routing key; the publisher does not add its own reconnect + retry on
// top of each attempt.
func TestPublishRetryPolicyIsTheOnlyRetryLayer(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	b.publishErr = func(int, string, string) error { return errors.New("always") }
	retry := &RetryConfig{MaxAttempts: 3, InitialDelay: 1, MaxDelay: 1, BackoffMult: 1}
	p := newFakePubSub(b, multiKeyTopology(retry))

	err := p.Publish(context.Background(), "q", Event{})

	assert.Equal(t, ERR_MQ_PUBLISH_FAILED, requireAppError(t, err).Code())
	b.mu.Lock()
	defer b.mu.Unlock()
	assert.Equal(t, 3, b.attempts, "MaxAttempts attempts for the failing key, not 2x")
	assert.Equal(t, 3, b.dials, "one dial per attempt (each retry reopens the channel), not two")
}

// C5-009: without a retry policy the publisher's own reconnect + single
// retry is the only retry layer.
func TestPublishWithoutRetryPolicyRetriesOnce(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	b.publishErr = func(attempt int, _, _ string) error {
		if attempt == 1 {
			return errors.New("channel died")
		}
		return nil
	}
	p := newFakePubSub(b, testTopology())

	require.NoError(t, p.Publish(context.Background(), "q", Event{}))

	b.mu.Lock()
	defer b.mu.Unlock()
	assert.Equal(t, 2, b.attempts)
	assert.Equal(t, 2, b.dials, "the single retry runs on a new connection")
}
