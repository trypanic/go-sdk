//go:build integration

package integrationtest

import (
	"context"
	"net/netip"
	"strings"
	"testing"
	"time"

	dockercontainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	dockerclient "github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

const reaperImage = "testcontainers/ryuk:0.14.0"

// Start requires locally cached images and registers cleanup before startup.
func Start(t *testing.T, request testcontainers.ContainerRequest) testcontainers.Container {
	t.Helper()
	if testing.Short() {
		t.Skip("integration infrastructure is disabled in short mode")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	client, err := testcontainers.NewDockerClientWithOpts(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	require.True(t, strings.HasPrefix(client.DaemonHost(), "unix://"), "integration tests require a local Unix Docker socket")
	for _, image := range []string{request.Image, reaperImage} {
		inspected, err := client.ImageInspect(ctx, image)
		require.NoError(t, err, "image %s must be cached before tests; run task test:integration:pull", image)
		if image == reaperImage {
			require.Contains(t, inspected.RepoDigests, "testcontainers/ryuk@sha256:7c1a8a9a47c780ed0f983770a662f80deb115d95cce3e2daa3d12115b8cd28f0", "run task test:integration:pull to restore the pinned reaper")
		}
	}
	provider, err := testcontainers.NewDockerProvider(testcontainers.WithDefaultBridgeNetwork(testcontainers.Bridge))
	require.NoError(t, err)
	provider.SetClient(client)
	// v0.44.0 has no Ryuk HostIP setting. The exported provider hook preserves
	// the upstream socket mount, auto-remove, session reuse and crash cleanup.
	// Upstream Compose also uses this entry point; remove this exception when
	// Testcontainers exposes a supported loopback binding option for Ryuk.
	//lint:ignore SA1019 The pinned provider hook is required to bind Ryuk safely.
	reaper, err := testcontainers.NewReaper(ctx, provider.Config().Config.SessionID, loopbackProvider{provider}, "")
	require.NoError(t, err)
	termination, err := reaper.Connect()
	require.NoError(t, err)
	t.Cleanup(func() { close(termination) })
	bindLoopback(&request)
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: request})
	if container != nil {
		t.Cleanup(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			require.NoError(t, container.Terminate(cleanup))
		})
	}
	require.NoError(t, err)
	require.NoError(t, container.Start(ctx))
	assertSessionBindings(t, ctx, client, provider.Config().Config.SessionID, container.GetContainerID())
	return container
}

// loopbackProvider changes only publication addresses, including the reaper's.
// All lifecycle and cleanup behavior remains owned by Testcontainers.
type loopbackProvider struct {
	*testcontainers.DockerProvider
}

func (p loopbackProvider) RunContainer(ctx context.Context, request testcontainers.ContainerRequest) (testcontainers.Container, error) {
	bindLoopback(&request)
	return p.DockerProvider.RunContainer(ctx, request)
}

func bindLoopback(request *testcontainers.ContainerRequest) {
	previous := request.HostConfigModifier
	request.HostConfigModifier = func(config *dockercontainer.HostConfig) {
		if previous != nil {
			previous(config)
		}
		config.PublishAllPorts = false
		config.PortBindings = make(network.PortMap, len(request.ExposedPorts))
		for _, exposed := range request.ExposedPorts {
			port := network.MustParsePort(exposed)
			config.PortBindings[port] = []network.PortBinding{{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: "0"}}
		}
	}
}

// Inspect Docker's actual published bindings, not the client connection host.
// The session filter includes Ryuk, which intentionally lacks the reap label.
func assertSessionBindings(t *testing.T, ctx context.Context, client *testcontainers.DockerClient, sessionID, serviceID string) {
	t.Helper()
	listed, err := client.ContainerList(ctx, dockerclient.ContainerListOptions{
		Filters: make(dockerclient.Filters).Add("label", "org.testcontainers.sessionId="+sessionID),
	})
	require.NoError(t, err)
	foundReaper := false
	foundService := false
	for _, item := range listed.Items {
		if item.ID != serviceID && item.Labels["org.testcontainers.ryuk"] != "true" {
			continue // Other test processes can remove their services concurrently.
		}
		inspected, err := client.ContainerInspect(ctx, item.ID, dockerclient.ContainerInspectOptions{})
		require.NoError(t, err)
		if item.ID == serviceID {
			foundService = true
			require.NotEmpty(t, inspected.Container.NetworkSettings.Ports)
		}
		for port, bindings := range inspected.Container.NetworkSettings.Ports {
			for _, binding := range bindings {
				require.True(t, binding.HostIP.IsLoopback(), "container %s publishes %s on %s:%s", item.Image, port, binding.HostIP, binding.HostPort)
				t.Logf("local publication: %s %s -> %s:%s", item.Image, port, binding.HostIP, binding.HostPort)
			}
		}
		if item.Labels["org.testcontainers.ryuk"] == "true" {
			foundReaper = true
			require.True(t, inspected.Container.HostConfig.AutoRemove, "Ryuk must retain crash cleanup")
			require.Equal(t, testcontainers.Bridge, string(inspected.Container.HostConfig.NetworkMode))
			require.Contains(t, inspected.Container.HostConfig.Binds, "/var/run/docker.sock:/var/run/docker.sock")
			require.NotEmpty(t, inspected.Container.NetworkSettings.Ports[network.MustParsePort("8080/tcp")])
		}
	}
	require.True(t, foundService, "the service must be running")
	require.True(t, foundReaper, "the session reaper must be running")
}
