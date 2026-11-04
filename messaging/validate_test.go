package messaging

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/v2/errorkit"
)

const validTopologyJSON = `{
	"version": "1.0.0",
	"exchanges": [{"name": "events", "type": "topic"}, {"name": "dlx", "type": "direct"}],
	"queues": [
		{"name": "q", "prefetch": 10, "dlx": {"exchange": "dlx", "routingKey": "q.dead"},
		 "retry": {"maxAttempts": 3, "initialDelay": 100, "maxDelay": 1000, "backoffMultiplier": 2}},
		{"name": "q.dlq"}
	],
	"bindings": [
		{"queue": "q", "exchange": "events", "routingKeys": ["a"]},
		{"queue": "q.dlq", "exchange": "dlx", "routingKeys": ["q.dead"]}
	]
}`

func TestLoadTopologyAcceptsValidTopology(t *testing.T) {
	t.Parallel()

	_, err := LoadTopologyFromBytes([]byte(validTopologyJSON))
	assert.NoError(t, err)
}

// C5-010: configuration mistakes are rejected at load time with
// ERR_SYSTEM_CONFIG_INVALID instead of surfacing as message loss or
// unbounded retries in production.
func TestLoadTopologyRejectsInvalidTopologies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		json string
	}{
		{"unknown field (typo)", `{"exchanges":[{"name":"e","type":"topic"}],"queues":[{"name":"q","prefech":10}],"bindings":[{"queue":"q","exchange":"e","routingKeys":["a"]}]}`},
		{"trailing data", `{"version":"1"} {"version":"2"}`},
		{"negative maxAttempts", `{"exchanges":[{"name":"e","type":"topic"}],"queues":[{"name":"q","retry":{"maxAttempts":-1,"initialDelay":1,"maxDelay":1,"backoffMultiplier":1}}],"bindings":[{"queue":"q","exchange":"e","routingKeys":["a"]}]}`},
		{"zero maxAttempts", `{"exchanges":[{"name":"e","type":"topic"}],"queues":[{"name":"q","retry":{"maxAttempts":0,"initialDelay":1,"maxDelay":1,"backoffMultiplier":1}}],"bindings":[{"queue":"q","exchange":"e","routingKeys":["a"]}]}`},
		{"negative delay", `{"exchanges":[{"name":"e","type":"topic"}],"queues":[{"name":"q","retry":{"maxAttempts":2,"initialDelay":-1,"maxDelay":1,"backoffMultiplier":1}}],"bindings":[{"queue":"q","exchange":"e","routingKeys":["a"]}]}`},
		{"multiplier below 1", `{"exchanges":[{"name":"e","type":"topic"}],"queues":[{"name":"q","retry":{"maxAttempts":2,"initialDelay":1,"maxDelay":1}}],"bindings":[{"queue":"q","exchange":"e","routingKeys":["a"]}]}`},
		{"negative prefetch", `{"exchanges":[{"name":"e","type":"topic"}],"queues":[{"name":"q","prefetch":-1}],"bindings":[{"queue":"q","exchange":"e","routingKeys":["a"]}]}`},
		{"duplicate binding for a queue", `{"exchanges":[{"name":"a","type":"topic"},{"name":"b","type":"topic"}],"queues":[{"name":"q"}],"bindings":[{"queue":"q","exchange":"a","routingKeys":["x"]},{"queue":"q","exchange":"b","routingKeys":["y"]}]}`},
		{"binding to undeclared exchange", `{"queues":[{"name":"q"}],"bindings":[{"queue":"q","exchange":"missing","routingKeys":["a"]}]}`},
		{"binding to undeclared queue", `{"exchanges":[{"name":"e","type":"topic"}],"bindings":[{"queue":"missing","exchange":"e","routingKeys":["a"]}]}`},
		{"binding without routing keys", `{"exchanges":[{"name":"e","type":"topic"}],"queues":[{"name":"q"}],"bindings":[{"queue":"q","exchange":"e","routingKeys":[]}]}`},
		{"undeclared DLX exchange", `{"exchanges":[{"name":"e","type":"topic"}],"queues":[{"name":"q","dlx":{"exchange":"missing"}}],"bindings":[{"queue":"q","exchange":"e","routingKeys":["a"]}]}`},
		{"duplicate queue name", `{"exchanges":[{"name":"e","type":"topic"}],"queues":[{"name":"q"},{"name":"q"}]}`},
		{"duplicate exchange name", `{"exchanges":[{"name":"e","type":"topic"},{"name":"e","type":"direct"}]}`},
		{"empty queue name", `{"queues":[{"name":""}]}`},
		{"exchange without type", `{"exchanges":[{"name":"e"}]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := LoadTopologyFromBytes([]byte(tc.json))

			appErr := requireAppError(t, err)
			assert.Equal(t, errorkit.ERR_SYSTEM_CONFIG_INVALID, appErr.Code())
		})
	}
}

// C5-010: a topology supplied in memory is validated by NewPubSub before
// anything is dialed.
func TestNewPubSubValidatesInMemoryTopology(t *testing.T) {
	t.Parallel()

	b := newFakeBroker()
	topo := testTopology()
	topo.Queues[0].Retry = &RetryConfig{MaxAttempts: -1, BackoffMult: 1}

	_, err := newPubSub("amqp://fake", b.dial, WithTopology(topo))

	assert.Equal(t, errorkit.ERR_SYSTEM_CONFIG_INVALID, requireAppError(t, err).Code())
	assert.Equal(t, 0, b.dialCount())
}

// C5-010 / C6-005: a negative MaxAttempts that bypasses validation does not
// wrap to an effectively unbounded uint64 retry count.
func TestExecuteWithRetryNegativeMaxAttemptsIsBounded(t *testing.T) {
	t.Parallel()

	attempts := 0
	err := executeWithRetry(t.Context(), func() error {
		attempts++
		if attempts < 1000 {
			return errors.New("fail")
		}
		return nil
	}, &RetryConfig{MaxAttempts: -1, BackoffMult: 1})

	require.Error(t, err)
	assert.Equal(t, 1, attempts)
}
