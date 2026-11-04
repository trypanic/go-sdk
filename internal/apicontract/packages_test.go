package apicontract_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/batch"
	"github.com/trypanic/go-sdk/mongo"
	"github.com/trypanic/go-sdk/postgres"
)

func TestCanonicalPackageNames(t *testing.T) {
	_ = postgres.Config{}
	_ = mongo.Config{}
	require.Equal(t, [][]int{{1, 2}, {3}}, batch.Chunk([]int{1, 2, 3}, 2))
	require.Equal(t, []string{"a", "b"}, batch.MergeUniqueStrings([]string{"a"}, []string{"a", "b"}))
}
