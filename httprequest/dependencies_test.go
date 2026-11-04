package httprequest_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestDoesNotImportLogger(t *testing.T) {
	output, err := exec.Command("go", "list", "-f", "{{join .Imports \"\\n\"}}", ".").CombinedOutput()
	require.NoError(t, err, string(output))
	require.NotContains(t, strings.Fields(string(output)), "github.com/trypanic/go-sdk/v2/logger")
}
