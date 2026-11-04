package otlp_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/logger"
	"github.com/trypanic/go-sdk/logger/otlp"
)

func TestTypedNilWriterPreservesLoggerCompatibility(t *testing.T) {
	var writer *otlp.Writer
	require.NotPanics(t, func() {
		global, closer := logger.Init("nil-exporter", "test", writer)
		global.Info().Msg("optional exporter absent")
		require.NoError(t, closer.Close())
	})
	var local bytes.Buffer
	require.NotPanics(t, func() {
		logger.New(logger.Config{Env: logger.Prod, Writer: &local, OTLPWriter: writer}).Info("local event")
	})
	require.Contains(t, local.String(), "local event")
}
