package messaging

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

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

// Validate reports every configuration mistake in t as one
// ERR_SYSTEM_CONFIG_INVALID error, or nil. It checks that:
//
//   - exchange and queue names are non-empty and unique, and every exchange
//     has a type;
//   - every binding names a declared queue and a declared exchange, has at
//     least one routing key, and no queue has more than one binding;
//   - every dead-letter exchange is declared;
//   - prefetch, ttl and maxLength are not negative;
//   - a retry policy has maxAttempts >= 1, delays >= 0, maxDelay >=
//     initialDelay and backoffMultiplier >= 1.
//
// Because every referenced exchange must be declared in t, a dead-letter
// exchange of "" (the default exchange) is rejected, and so is a binding to
// a predeclared amq.* exchange that t does not list (RabbitMQ refuses to
// declare amq.* names, so a topology must use its own exchanges).
//
// Retry values that pass validation are interpreted by the algorithms
// backoff: an initialDelay or maxDelay of 0 is replaced by its default
// (1 s and 30 s) rather than meaning "no delay", and maxAttempts is capped
// at algorithms.MaxRetriesLimit (1000).
//
// The loaders and NewPubSub call it.
func (t *Topology) Validate() error {
	if t == nil {
		return topologyError(errors.New("topology is nil"))
	}
	var problems []error
	add := func(format string, args ...any) { problems = append(problems, fmt.Errorf(format, args...)) }

	exchanges := map[string]bool{}
	for i, e := range t.Exchanges {
		switch {
		case e.Name == "":
			add("exchanges[%d]: name is empty", i)
		case exchanges[e.Name]:
			add("exchange %q is declared more than once", e.Name)
		}
		if e.Type == "" {
			add("exchange %q: type is empty", e.Name)
		}
		exchanges[e.Name] = true
	}

	queues := map[string]bool{}
	for i, q := range t.Queues {
		switch {
		case q.Name == "":
			add("queues[%d]: name is empty", i)
		case queues[q.Name]:
			add("queue %q is declared more than once", q.Name)
		}
		queues[q.Name] = true
		if q.Prefetch < 0 || q.TTL < 0 || q.MaxLength < 0 {
			add("queue %q: prefetch, ttl and maxLength must not be negative", q.Name)
		}
		if q.DLX != nil && !exchanges[q.DLX.Exchange] {
			add("queue %q: dead-letter exchange %q is not declared", q.Name, q.DLX.Exchange)
		}
		if r := q.Retry; r != nil {
			if r.MaxAttempts < 1 {
				add("queue %q: retry.maxAttempts must be >= 1, got %d", q.Name, r.MaxAttempts)
			}
			if r.InitialDelay < 0 || r.MaxDelay < 0 {
				add("queue %q: retry delays must not be negative", q.Name)
			}
			if r.MaxDelay < r.InitialDelay {
				add("queue %q: retry.maxDelay must be >= retry.initialDelay", q.Name)
			}
			if r.BackoffMult < 1 {
				add("queue %q: retry.backoffMultiplier must be >= 1, got %v", q.Name, r.BackoffMult)
			}
		}
	}

	bound := map[string]bool{}
	for i, b := range t.Bindings {
		if !queues[b.Queue] {
			add("bindings[%d]: queue %q is not declared", i, b.Queue)
		}
		if !exchanges[b.Exchange] {
			add("bindings[%d]: exchange %q is not declared", i, b.Exchange)
		}
		if len(b.RoutingKeys) == 0 {
			add("bindings[%d]: no routing keys (use [\"\"] for a fanout exchange)", i)
		}
		if bound[b.Queue] {
			add("queue %q has more than one binding", b.Queue)
		}
		bound[b.Queue] = true
	}

	if len(problems) > 0 {
		return topologyError(errors.Join(problems...))
	}
	return nil
}

// topologyError reports an invalid topology.
func topologyError(err error) *errorkit.AppError {
	return errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
		errorkit.WithReason("invalid topology: %s", strings.ReplaceAll(err.Error(), "\n", "; ")),
		errorkit.WithWrapped(err),
	)
}

// LoadTopologyFromBytes parses and validates an in-memory topology JSON
// document. Unknown fields and trailing data are rejected.
func LoadTopologyFromBytes(data []byte) (*Topology, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var topo Topology
	if err := dec.Decode(&topo); err != nil {
		return nil, topologyError(fmt.Errorf("parse: %w", err))
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, topologyError(errors.New("unexpected data after the topology document"))
	}
	if err := topo.Validate(); err != nil {
		return nil, err
	}
	return &topo, nil
}

// LoadTopologyFromFile reads, parses and validates a topology JSON file from
// the supplied path. No directory discovery, no parent traversal.
func LoadTopologyFromFile(path string) (*Topology, error) {
	if path == "" {
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
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
		return nil, errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
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
