package messaging

import (
	"encoding/json"
	"os"

	"github.com/trypanic/go-sdk/errorkit"
)

// TopologyEnvVar is the canonical environment variable read by
// LoadTopologyFromEnv. SDK consumers that prefer a different name should
// supply the path themselves via LoadTopologyFromFile.
const TopologyEnvVar = "RABBITMQ_TOPOLOGY_FILE"

// Topology represents the full declarative description of the RabbitMQ setup.
type Topology struct {
	Version   string          `json:"version"`
	Exchanges []Exchange      `json:"exchanges"`
	Queues    []QueueTopology `json:"queues"`
	Bindings  []Binding       `json:"bindings"`
}

// Exchange describes a RabbitMQ exchange.
type Exchange struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Durable    bool   `json:"durable"`
	AutoDelete bool   `json:"autoDelete"`
}

// Binding maps a queue to an exchange via one or more routing keys.
type Binding struct {
	Queue       string   `json:"queue"`
	Exchange    string   `json:"exchange"`
	RoutingKeys []string `json:"routingKeys"`
}

// GetQueueByName returns the queue definition and true, or a zero value and false.
func (t *Topology) GetQueueByName(name string) (QueueTopology, bool) {
	for _, q := range t.Queues {
		if q.Name == name {
			return q, true
		}
	}
	return QueueTopology{}, false
}

// GetBindingByQueue returns the binding for the given queue and true, or a zero value and false.
func (t *Topology) GetBindingByQueue(queueName string) (Binding, bool) {
	for _, b := range t.Bindings {
		if b.Queue == queueName {
			return b, true
		}
	}
	return Binding{}, false
}

// GetExchangeByName returns the exchange definition and true, or a zero value and false.
func (t *Topology) GetExchangeByName(name string) (Exchange, bool) {
	for _, e := range t.Exchanges {
		if e.Name == name {
			return e, true
		}
	}
	return Exchange{}, false
}

// LoadTopologyFromBytes parses an in-memory topology JSON document.
func LoadTopologyFromBytes(data []byte) (*Topology, error) {
	var topo Topology
	if err := json.Unmarshal(data, &topo); err != nil {
		return nil, errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("failed to unmarshal topology"),
			errorkit.WithWrapped(err),
		)
	}
	return &topo, nil
}

// LoadTopologyFromFile reads and parses a topology JSON file from the supplied
// path. No directory discovery, no parent traversal.
func LoadTopologyFromFile(path string) (*Topology, error) {
	if path == "" {
		return nil, errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("topology file path is empty"),
		)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("failed to read topology file"),
			errorkit.WithWrapped(err),
		)
	}
	return LoadTopologyFromBytes(data)
}

// LoadTopologyFromEnv reads TopologyEnvVar (RABBITMQ_TOPOLOGY_FILE) and
// forwards the resolved path to LoadTopologyFromFile. Returns an error when
// the env var is unset or empty — there is no parent-directory fallback.
func LoadTopologyFromEnv() (*Topology, error) {
	path := os.Getenv(TopologyEnvVar)
	if path == "" {
		return nil, errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("%s environment variable is not set", TopologyEnvVar),
		)
	}
	return LoadTopologyFromFile(path)
}

// LoadTopology is the legacy entry point retained for backwards compatibility.
// New code should call one of the explicit loaders above. This function now
// reads only the environment variable; the previous behavior of walking the
// parent directories looking for `migrations/rabbitmq/topology.json` has been
// removed because it leaked implementation details of the original monorepo
// into every consumer of this SDK.
func LoadTopology() (*Topology, error) {
	return LoadTopologyFromEnv()
}
