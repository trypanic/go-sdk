package mongodb

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/trypanic/go-sdk/telemetry"
)

func TestWrapWithInstrumenterNilReturnsInner(t *testing.T) {
	t.Parallel()

	stub := &stubClientPort{}
	if got := WrapWithInstrumenter(stub, nil); got != stub {
		t.Fatalf("WrapWithInstrumenter(nil) must return inner unchanged")
	}
}

func TestWrapWithInstrumenterAddsSpanWrapper(t *testing.T) {
	t.Parallel()

	stub := &stubClientPort{}
	wrapped := WrapWithInstrumenter(stub, telemetry.NewInstrumenter(telemetry.InstrumenterConfig{}))
	if _, ok := wrapped.(*tracingClientPort); !ok {
		t.Fatalf("expected tracingClientPort, got %T", wrapped)
	}
}

type stubClientPort struct{}

func (s *stubClientPort) Collection(string) Collection { return stubCollection{} }
func (s *stubClientPort) Ping(context.Context) error   { return nil }
func (s *stubClientPort) Close(context.Context) error  { return nil }

type stubCollection struct{}

func (stubCollection) InsertOne(context.Context, any, ...options.Lister[options.InsertOneOptions]) (InsertOneResult, error) {
	return nil, nil
}
func (stubCollection) InsertMany(context.Context, any, ...options.Lister[options.InsertManyOptions]) (InsertManyResult, error) {
	return nil, nil
}
func (stubCollection) Find(context.Context, any, ...options.Lister[options.FindOptions]) (Cursor, error) {
	return nil, nil
}
func (stubCollection) FindOne(context.Context, any, ...options.Lister[options.FindOneOptions]) SingleResult {
	return nil
}
func (stubCollection) BulkWrite(context.Context, []mongo.WriteModel, ...options.Lister[options.BulkWriteOptions]) (BulkWriteResult, error) {
	return nil, nil
}
func (stubCollection) ReplaceOne(context.Context, any, any, ...options.Lister[options.ReplaceOptions]) (UpdateResult, error) {
	return nil, nil
}
func (stubCollection) CountDocuments(context.Context, any, ...options.Lister[options.CountOptions]) (int64, error) {
	return 0, nil
}
func (stubCollection) UpdateOne(context.Context, any, any, ...options.Lister[options.UpdateOneOptions]) (UpdateResult, error) {
	return nil, nil
}
func (stubCollection) DeleteOne(context.Context, any, ...options.Lister[options.DeleteOneOptions]) (DeleteResult, error) {
	return nil, nil
}
