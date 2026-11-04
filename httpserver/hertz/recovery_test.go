package hertz

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/stretchr/testify/require"

	"github.com/trypanic/go-sdk/httpserver"
)

func panickingServer(t *testing.T, enableRecovery bool) *Server {
	t.Helper()
	opts := testOptions()
	opts.EnableAudit = false
	opts.EnableRecovery = enableRecovery
	s := newTestServer(t, opts)
	s.GET("/boom", func(context.Context, httpserver.Context) {
		panic("boom secret=xyz")
	})
	return s
}

func TestRecoveryDisabledLetsPanicsPropagate(t *testing.T) {
	t.Parallel()

	s := panickingServer(t, false)
	ctx, _ := logContext(t)

	require.PanicsWithValue(t, "boom secret=xyz", func() {
		perform(ctx, s, http.MethodGet, "/boom", nil)
	}, "EnableRecovery=false must not install any recovery layer")
}

// TestRecoveryEnabledLogsOnceThroughSDKLogger is not parallel: it
// redirects Hertz's global hlog output.
func TestRecoveryEnabledLogsOnceThroughSDKLogger(t *testing.T) {
	var hertzLog bytes.Buffer
	hlog.SetOutput(&hertzLog)
	t.Cleanup(func() { hlog.SetOutput(os.Stderr) })

	s := panickingServer(t, true)
	ctx, logs := logContext(t)

	res := perform(ctx, s, http.MethodGet, "/boom", nil)

	require.Equal(t, http.StatusInternalServerError, res.StatusCode())
	require.NotContains(t, hertzLog.String(), "[Recovery]", "Hertz's own recovery must not run")

	var lines []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m), line)
		lines = append(lines, m)
	}
	require.Len(t, lines, 1, "one structured log line per panic")
	require.Equal(t, "ERR_SYSTEM_UNEXPECTED", lines[0]["error_code"])
	require.Contains(t, lines[0]["panic_stack"], "recovery_test.go", "the panic stack is in the structured line")
}
