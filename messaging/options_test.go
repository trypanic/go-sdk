package messaging

import (
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func TestResolveTopologyExplicitWins(t *testing.T) {
	t.Parallel()

	topo := &Topology{Version: "explicit"}
	got, err := resolveTopology(&pubSubBuild{topology: topo})
	if err != nil {
		t.Fatalf("resolveTopology error: %v", err)
	}
	if got != topo {
		t.Fatalf("expected the explicit *Topology to be returned unchanged")
	}
}

func TestResolveTopologyEnvFallbackErrorsWhenUnset(t *testing.T) {
	// No path, no env, no in-memory topology — must error.
	t.Setenv(TopologyEnvVar, "")

	if _, err := resolveTopology(&pubSubBuild{}); err == nil {
		t.Fatal("resolveTopology must error when no source is provided")
	}
}

func TestPubSubDefaultsToGlobalTelemetry(t *testing.T) {
	t.Parallel()

	p := &PubSub{}
	if p.tracerOrDefault() != otel.Tracer("messaging") {
		t.Fatalf("expected default tracer to come from otel.Tracer(\"messaging\")")
	}
	if p.propagatorOrDefault() != otel.GetTextMapPropagator() {
		t.Fatalf("expected default propagator to come from otel.GetTextMapPropagator()")
	}
}

func TestWithTracerOverridesDefault(t *testing.T) {
	t.Parallel()

	custom := otel.Tracer("custom")
	build := &pubSubBuild{}
	WithTracer(custom)(build)
	p := &PubSub{tracer: build.tracer}
	if p.tracerOrDefault() != custom {
		t.Fatalf("WithTracer did not apply")
	}
}

func TestWithPropagatorOverridesDefault(t *testing.T) {
	t.Parallel()

	custom := propagation.TraceContext{}
	build := &pubSubBuild{}
	WithPropagator(custom)(build)
	p := &PubSub{propagator: build.propagator}
	if p.propagatorOrDefault() != custom {
		t.Fatalf("WithPropagator did not apply")
	}
}
