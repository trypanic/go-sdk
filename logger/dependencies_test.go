package logger_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoreClientsDoNotLinkGRPC(t *testing.T) {
	for _, pkg := range []string{".", "../httprequest", "../llmclient"} {
		output, err := exec.Command("go", "list", "-deps", pkg).CombinedOutput()
		require.NoError(t, err, string(output))
		for dep := range strings.FieldsSeq(string(output)) {
			require.False(t, strings.HasPrefix(dep, "google.golang.org/grpc"), "%s links %s", pkg, dep)
		}
	}
}
