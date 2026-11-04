package messaging

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/v2/errorkit"
	"github.com/trypanic/go-sdk/v2/logger"
)

func TestMessagingClassificationSkipsTypedNil(t *testing.T) {
	var absent *errorkit.AppError
	expected := errorkit.NewError(ERR_MQ_CLOSED)
	joined := errors.Join(absent, expected)
	require.NotPanics(t, func() { require.False(t, isRetriable(joined)) })
	fallback := asAppError(absent, ERR_MQ_ERROR, "fallback")
	require.True(t, errorkit.HasCode(fallback, ERR_MQ_ERROR))
	require.Same(t, joined, asAppError(joined, ERR_MQ_ERROR, "fallback"))
	var output bytes.Buffer
	pubsub := &PubSub{log: logger.New(logger.Config{Env: logger.Prod, Writer: &output})}
	pubsub.warn(t.Context(), joined, "operation failed")
	require.Contains(t, output.String(), string(ERR_MQ_CLOSED))
}
