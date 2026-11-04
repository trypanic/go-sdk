package mongotest_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/v2/mongo"
	"github.com/trypanic/go-sdk/v2/mongo/mongotest"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestRepositoryUsesFake(t *testing.T) {
	collection := &mongotest.Collection{FindOneFunc: func(_ context.Context, filter any, _ ...options.Lister[options.FindOneOptions]) mongo.SingleResult {
		require.Equal(t, "id", filter)
		return mongotest.SingleResult(func(out any) error { *out.(*string) = "Ada"; return nil })
	}}
	client := &mongotest.Client{CollectionFunc: func(name string) mongo.Collection { require.Equal(t, "users", name); return collection }}
	var port mongo.ClientPort = client
	var name string
	require.NoError(t, port.Collection("users").FindOne(t.Context(), "id").Decode(&name))
	require.Equal(t, "Ada", name)
	require.Panics(t, func() { _ = client.Ping(t.Context()) })
}

func TestCursorCanDriveStreamingConsumer(t *testing.T) {
	remaining := 1
	closed := false
	cursor := &mongotest.Cursor{
		NextFunc:   func(context.Context) bool { remaining--; return remaining >= 0 },
		DecodeFunc: func(out any) error { *out.(*string) = "row"; return nil },
		ErrFunc:    func() error { return nil },
		CloseFunc:  func(context.Context) error { closed = true; return nil },
	}
	var port mongo.Cursor = cursor
	var rows []string
	for port.Next(t.Context()) {
		var row string
		require.NoError(t, port.Decode(&row))
		rows = append(rows, row)
	}
	require.NoError(t, port.Err())
	require.NoError(t, port.Close(t.Context()))
	require.True(t, closed)
	require.Equal(t, []string{"row"}, rows)
}
