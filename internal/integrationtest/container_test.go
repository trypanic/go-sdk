//go:build integration

package integrationtest

import (
	"testing"

	dockercontainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

func TestBindLoopbackPreservesReaperLifecycle(t *testing.T) {
	request := testcontainers.ContainerRequest{
		ExposedPorts: []string{"8080/tcp"},
		HostConfigModifier: func(config *dockercontainer.HostConfig) {
			config.AutoRemove = true
			config.Binds = []string{"/fixture/docker.sock:/var/run/docker.sock"}
			config.NetworkMode = testcontainers.Bridge
		},
	}
	bindLoopback(&request)
	config := &dockercontainer.HostConfig{}
	request.HostConfigModifier(config)
	require.True(t, config.AutoRemove)
	require.Equal(t, []string{"/fixture/docker.sock:/var/run/docker.sock"}, config.Binds)
	require.Equal(t, testcontainers.Bridge, string(config.NetworkMode))
	require.False(t, config.PublishAllPorts)
	bindings := config.PortBindings[network.MustParsePort("8080/tcp")]
	require.Len(t, bindings, 1)
	require.Equal(t, "127.0.0.1", bindings[0].HostIP.String())
	require.Equal(t, "0", bindings[0].HostPort, "Docker must allocate an ephemeral port")
}
