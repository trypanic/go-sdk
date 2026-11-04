package messaging

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/v2/errorkit"
)

// writeTopologyFile writes a valid topology file and returns its path.
func writeTopologyFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "topology.json")
	require.NoError(t, os.WriteFile(path, []byte(validTopologyJSON), 0o600))
	return path
}

// C5-015: NewPubSub never reads RABBITMQ_TOPOLOGY_FILE; without a topology
// option it fails before dialing.
func TestNewPubSubDoesNotReadTopologyEnv(t *testing.T) {
	t.Setenv(TopologyEnvVar, writeTopologyFile(t))
	b := newFakeBroker()

	_, err := newPubSub("amqp://fake", b.dial)

	appErr := requireAppError(t, err)
	assert.Equal(t, errorkit.ERR_SYSTEM_CONFIG_INVALID, appErr.Code())
	assert.Contains(t, appErr.Error(), "NewPubSubFromEnv")
	assert.Equal(t, 0, b.dialCount())
}

// C5-015: NewPubSubFromEnv loads the topology named by
// RABBITMQ_TOPOLOGY_FILE (and then dials the broker).
func TestNewPubSubFromEnvLoadsTopology(t *testing.T) {
	t.Setenv(TopologyEnvVar, writeTopologyFile(t))
	url, accepts := newDroppingListener(t)

	_, err := NewPubSubFromEnv(url)

	assert.Equal(t, ERR_MQ_CONNECTION_FAILED, requireAppError(t, err).Code(), "topology loaded, broker dialed")
	assert.Positive(t, accepts.Load())
}

// C5-015: NewPubSubFromEnv fails, without dialing, when the variable is
// unset.
func TestNewPubSubFromEnvRequiresVariable(t *testing.T) {
	t.Setenv(TopologyEnvVar, "")
	url, accepts := newDroppingListener(t)

	_, err := NewPubSubFromEnv(url)

	assert.Equal(t, errorkit.ERR_SYSTEM_CONFIG_INVALID, requireAppError(t, err).Code())
	assert.Zero(t, accepts.Load())
}

// An explicit topology option still wins over the environment.
func TestNewPubSubFromEnvExplicitTopologyWins(t *testing.T) {
	t.Setenv(TopologyEnvVar, filepath.Join(t.TempDir(), "missing.json"))

	topo, err := resolveTopology(&pubSubBuild{topologyFromEnv: true, topology: testTopology()})

	require.NoError(t, err)
	assert.Equal(t, "q", topo.Queues[0].Name)
}
