package messaging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTopologyFromBytesParsesValidJSON(t *testing.T) {
	t.Parallel()

	topo, err := LoadTopologyFromBytes([]byte(`{
		"version": "1.0.0",
		"exchanges": [{"name": "events", "type": "topic", "durable": true}],
		"queues": [{"name": "q1", "durable": true}],
		"bindings": [{"queue": "q1", "exchange": "events", "routingKeys": ["a.b"]}]
	}`))
	if err != nil {
		t.Fatalf("LoadTopologyFromBytes returned error: %v", err)
	}
	if topo.Version != "1.0.0" || len(topo.Exchanges) != 1 || len(topo.Queues) != 1 || len(topo.Bindings) != 1 {
		t.Fatalf("unexpected topology shape: %+v", topo)
	}
}

func TestLoadTopologyFromBytesRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	if _, err := LoadTopologyFromBytes([]byte(`{`)); err == nil {
		t.Fatal("expected error from invalid JSON, got nil")
	}
}

func TestLoadTopologyFromFileReadsExistingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "topology.json")
	if err := os.WriteFile(path, []byte(`{"version":"1.0.0"}`), 0o600); err != nil {
		t.Fatalf("write topology: %v", err)
	}

	topo, err := LoadTopologyFromFile(path)
	if err != nil {
		t.Fatalf("LoadTopologyFromFile error: %v", err)
	}
	if topo.Version != "1.0.0" {
		t.Fatalf("unexpected version: %s", topo.Version)
	}
}

func TestLoadTopologyFromFileEmptyPathReturnsError(t *testing.T) {
	t.Parallel()

	if _, err := LoadTopologyFromFile(""); err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
}

func TestLoadTopologyFromFileMissingFileReturnsError(t *testing.T) {
	t.Parallel()

	if _, err := LoadTopologyFromFile(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadTopologyFromEnvUsesEnvVar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "topology.json")
	if err := os.WriteFile(path, []byte(`{"version":"1.0.0"}`), 0o600); err != nil {
		t.Fatalf("write topology: %v", err)
	}
	t.Setenv(TopologyEnvVar, path)

	topo, err := LoadTopologyFromEnv()
	if err != nil {
		t.Fatalf("LoadTopologyFromEnv error: %v", err)
	}
	if topo.Version != "1.0.0" {
		t.Fatalf("unexpected version: %s", topo.Version)
	}
}

func TestLoadTopologyFromEnvMissingEnvReturnsError(t *testing.T) {
	t.Setenv(TopologyEnvVar, "")

	if _, err := LoadTopologyFromEnv(); err == nil {
		t.Fatal("expected error when env var unset, got nil")
	}
}

// LoadTopology no longer walks parent directories looking for migrations/rabbitmq/topology.json.
// Empty env + missing file must return an error directly.
func TestLoadTopologyDoesNotWalkParentDirs(t *testing.T) {
	repoRoot := t.TempDir()
	fallbackPath := filepath.Join(repoRoot, "migrations", "rabbitmq", "topology.json")
	if err := os.MkdirAll(filepath.Dir(fallbackPath), 0o755); err != nil {
		t.Fatalf("mkdir fallback dir: %v", err)
	}
	if err := os.WriteFile(fallbackPath, []byte(`{"version":"1.0.0"}`), 0o600); err != nil {
		t.Fatalf("write fallback topology: %v", err)
	}

	workdir := filepath.Join(repoRoot, "deep", "nested", "workdir")
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		t.Fatalf("mkdir workdir: %v", err)
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	if err := os.Chdir(workdir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	t.Setenv(TopologyEnvVar, "")
	if _, err := LoadTopology(); err == nil {
		t.Fatal("LoadTopology must not walk parent dirs; expected error, got nil")
	}
}
