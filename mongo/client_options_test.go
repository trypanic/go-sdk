package mongo

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestExplicitDriverOptionsOverrideConfig(t *testing.T) {
	base := clientOptions(Config{URI: "mongodb://127.0.0.1", MaxPoolSize: 3})
	configured := WithClientOptions(options.Client().SetMaxPoolSize(9).SetAppName("worker"))(base)
	require.Equal(t, uint64(9), *configured.MaxPoolSize)
	require.Equal(t, "worker", *configured.AppName)
	require.Equal(t, uint64(3), *base.MaxPoolSize)
}
