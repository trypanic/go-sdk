//go:build integration

package integrationtest

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

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
	host, err := container.Host(ctx)
	require.NoError(t, err)
	require.True(t, host == "localhost" || net.ParseIP(host).IsLoopback(), "container endpoints must be loopback")
	return container
}
