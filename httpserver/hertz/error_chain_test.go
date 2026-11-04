package hertz

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/logger"
)

func TestReplyLoggingSkipsTypedNil(t *testing.T) {
	var absent *errorkit.AppError
	expected := errorkit.NewError(errorkit.ERR_CLIENT_NOT_FOUND)
	var output bytes.Buffer
	log := logger.New(logger.Config{Env: logger.Prod, Writer: &output})
	logReplyError(log.IntoContext(t.Context()), errors.Join(absent, expected))
	require.Contains(t, output.String(), `"level":"warn"`)
	require.Contains(t, output.String(), string(expected.Code()))
}
