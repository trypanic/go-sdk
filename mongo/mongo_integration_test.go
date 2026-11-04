//go:build integration

package mongo_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/internal/integrationtest"
	"github.com/trypanic/go-sdk/mongo"
)

func TestMongoIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("requires local MongoDB container")
	}
	container := integrationtest.Start(t, testcontainers.ContainerRequest{
		Image: "mongo:9.0.4-resolute@sha256:714cc3db1e396e5a464b8443ccde50e53283368809401f53f1997b684ddb5be0", ExposedPorts: []string{"27017/tcp"},
		WaitingFor: wait.ForLog("Waiting for connections").WithStartupTimeout(time.Minute),
	})
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "27017/tcp")
	require.NoError(t, err)
	setup, finishSetup := context.WithCancel(ctx)
	defer finishSetup()
	client, err := mongo.New(setup, mongo.Config{URI: "mongodb://" + net.JoinHostPort(host, port.Port()), Database: "sdk", RetryWrites: new(false)})
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		require.NoError(t, client.Close(cleanup))
		require.ErrorIs(t, client.Unwrap().Ping(cleanup, readpref.Primary()), mongodriver.ErrClientDisconnected)
	})
	finishSetup()
	require.NoError(t, client.Ping(ctx))
	t.Run("write results and streaming cursor", func(t *testing.T) {
		collection := client.Collection("results")
		inserted, err := collection.InsertOne(ctx, bson.M{"_id": 1, "name": "first"})
		require.NoError(t, err)
		require.EqualValues(t, 1, inserted.GetInsertedID())
		many, err := collection.InsertMany(ctx, []bson.M{{"_id": 2, "name": "second"}, {"_id": 3, "name": "third"}})
		require.NoError(t, err)
		require.Len(t, many.GetInsertedIDs(), 2)
		updated, err := collection.UpdateOne(ctx, bson.M{"_id": 1}, bson.M{"$set": bson.M{"name": "changed"}})
		require.NoError(t, err)
		require.EqualValues(t, 1, updated.GetMatchedCount())
		require.EqualValues(t, 1, updated.GetModifiedCount())
		bulk, err := collection.BulkWrite(ctx, []mongodriver.WriteModel{
			mongodriver.NewUpdateOneModel().SetFilter(bson.M{"_id": 4}).SetUpdate(bson.M{"$set": bson.M{"name": "fourth"}}).SetUpsert(true),
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, bulk.GetUpsertedCount())
		require.Len(t, bulk.GetUpsertedIDs(), 1)
		cursor, err := collection.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetBatchSize(1))
		require.NoError(t, err)
		t.Cleanup(func() {
			cleanup, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			require.NoError(t, cursor.Close(cleanup))
		})
		type item struct {
			ID   int    `bson:"_id"`
			Name string `bson:"name"`
		}
		var got []item
		for cursor.Next(ctx) {
			var row item
			require.NoError(t, cursor.Decode(&row))
			got = append(got, row)
		}
		require.NoError(t, cursor.Err())
		require.Equal(t, []item{{1, "changed"}, {2, "second"}, {3, "third"}, {4, "fourth"}}, got)
		deleted, err := collection.DeleteOne(ctx, bson.M{"_id": 1})
		require.NoError(t, err)
		require.EqualValues(t, 1, deleted.GetDeletedCount())
		count, err := collection.CountDocuments(ctx, bson.M{})
		require.NoError(t, err)
		require.EqualValues(t, 3, count)
	})
	t.Run("classified errors and cursor decode", func(t *testing.T) {
		collection := client.Collection("errors")
		_, err := collection.InsertOne(ctx, bson.M{"_id": 1, "name": "value"})
		require.NoError(t, err)
		_, err = collection.InsertOne(ctx, bson.M{"_id": 1})
		require.True(t, errorkit.HasCode(err, errorkit.ERR_VALIDATION_DUPLICATE))
		require.True(t, mongodriver.IsDuplicateKeyError(err))
		var row bson.M
		err = collection.FindOne(ctx, bson.M{"_id": 99}).Decode(&row)
		require.True(t, errorkit.HasCode(err, mongo.ERR_DB_MONGO_NOT_FOUND))
		require.ErrorIs(t, err, mongodriver.ErrNoDocuments)
		cursor, err := collection.Find(ctx, bson.M{}, options.Find().SetBatchSize(1))
		require.NoError(t, err)
		defer func() {
			cleanup, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			require.NoError(t, cursor.Close(cleanup))
		}()
		var invalid []struct {
			Name int `bson:"name"`
		}
		err = cursor.All(ctx, &invalid)
		require.True(t, errorkit.HasCode(err, mongo.ERR_DB_MONGO_DECODE_FAILED))
		canceled, stop := context.WithCancel(ctx)
		stop()
		_, err = collection.CountDocuments(canceled, bson.M{})
		require.True(t, errorkit.HasCode(err, mongo.ERR_DB_MONGO_CANCELED))
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("borrowed adapter preserves owner connection", func(t *testing.T) {
		borrowed, err := mongo.NewFromClient(client.Unwrap(), "sdk", nil)
		require.NoError(t, err)
		require.NoError(t, borrowed.Close(ctx))
		require.NoError(t, client.Unwrap().Ping(ctx, readpref.Primary()))
	})
}
