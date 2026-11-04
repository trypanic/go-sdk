package postgres

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/v2/errorkit"
)

func TestJSONArgumentFailureIsNotRetriable(t *testing.T) {
	// A nil pool proves encoding fails before any query can be attempted.
	sp := StoredProcedure[int]{}
	_, err := sp.QueryJSON(t.Context(), "SELECT * FROM write($1)", make(chan int))
	app, ok := errorkit.As(err)
	require.True(t, ok)
	require.Equal(t, errorkit.ErrorCode("ERR_DB_POSTGRES_ENCODE_FAILED"), app.Code())
	require.False(t, app.Metadata.Retriable)
	_, ok = errors.AsType[*json.UnsupportedTypeError](err)
	require.True(t, ok)
}
