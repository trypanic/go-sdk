package errorkit_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/errorkit"
)

func TestNewWithoutStackPreservesErrorContract(t *testing.T) {
	cause := errors.New("cause")
	e := errorkit.NewWithoutStack(errorkit.ERR_VALIDATION).With(errorkit.WithReason("invalid"), errorkit.WithWrapped(cause))
	require.Empty(t, e.Trace)
	require.NotEmpty(t, e.ID)
	require.NotEmpty(t, e.Timestamp)
	require.Equal(t, errorkit.ERR_VALIDATION, e.Code())
	require.Equal(t, 400, e.Metadata.HTTPStatus)
	require.ErrorIs(t, e, cause)
	require.True(t, errorkit.HasCode(e, errorkit.ERR_VALIDATION))
	require.NotEmpty(t, errorkit.NewError(errorkit.ERR_VALIDATION).Trace)
	unknown := errorkit.NewWithoutStack("UNREGISTERED_STACKLESS_TEST")
	require.Equal(t, errorkit.ErrorCode("UNREGISTERED_STACKLESS_TEST"), unknown.Code())
	require.Equal(t, 500, unknown.Metadata.HTTPStatus)
}
