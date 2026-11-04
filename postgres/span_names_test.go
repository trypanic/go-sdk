package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQualifiedProcedureSpanNames(t *testing.T) {
	for _, query := range []string{"SELECT * FROM audit.clean($1)", "CALL audit.clean($1)"} {
		require.Equal(t, "postgres.audit.clean.query", procSpanName(query))
	}
}
