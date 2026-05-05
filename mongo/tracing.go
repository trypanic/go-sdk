package mongodb

import (
	"context"

	"github.com/trypanic/go-sdk/telemetry"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// WrapWithTracing wraps a ClientPort with automatic span creation.
//
// Each call to Collection(name) returns a tracing-enabled collection.
// All collection operations open a span named:
//
//	"mongo.<collectionName>.<operation>"
//
// Examples:
//
//	Collection("products").InsertOne(...) → span: "mongo.products.insertOne"
//	Collection("orders").UpdateOne(...)   → span: "mongo.orders.updateOne"
//
// The span becomes the parent of all MongoDB driver spans produced by otelmongo,
// making the trace tree immediately readable in observability tools without any
// instrumentation code in individual repositories.
//
// Usage (in bootstrap):
//
//	repo.NewRepository(mongodb.WrapWithTracing(mongoClient))
func WrapWithTracing(inner ClientPort) ClientPort {
	return WrapWithInstrumenter(inner, telemetry.NewInstrumenter(telemetry.InstrumenterConfig{}))
}

// WrapWithInstrumenter wraps a ClientPort with an explicit telemetry instrumenter.
// Pass nil to skip the tracing wrapper entirely (returns inner unchanged).
func WrapWithInstrumenter(inner ClientPort, instrumenter *telemetry.Instrumenter) ClientPort {
	if instrumenter == nil {
		return inner
	}
	return &tracingClientPort{inner: inner, instrumenter: instrumenter}
}

type tracingClientPort struct {
	inner        ClientPort
	instrumenter *telemetry.Instrumenter
}

func (t *tracingClientPort) Collection(name string) Collection {
	return tracingCollection{inner: t.inner.Collection(name), name: name, instrumenter: t.instrumenter}
}

func (t *tracingClientPort) Ping(ctx context.Context) error {
	return t.inner.Ping(ctx)
}

func (t *tracingClientPort) Close(ctx context.Context) error {
	return t.inner.Close(ctx)
}

type tracingCollection struct {
	inner        Collection
	name         string
	instrumenter *telemetry.Instrumenter
}

func (c tracingCollection) span(ctx context.Context, op string) (context.Context, telemetry.Span) {
	return c.instrumenter.Start(ctx, "mongo."+c.name+"."+op)
}

func (c tracingCollection) InsertOne(ctx context.Context, doc any, opts ...options.Lister[options.InsertOneOptions]) (InsertOneResult, error) {
	ctx, span := c.span(ctx, "insertOne")
	defer span.End()
	return c.inner.InsertOne(ctx, doc, opts...)
}

func (c tracingCollection) InsertMany(ctx context.Context, docs any, opts ...options.Lister[options.InsertManyOptions]) (InsertManyResult, error) {
	ctx, span := c.span(ctx, "insertMany")
	defer span.End()
	return c.inner.InsertMany(ctx, docs, opts...)
}

func (c tracingCollection) Find(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) (Cursor, error) {
	ctx, span := c.span(ctx, "find")
	defer span.End()
	return c.inner.Find(ctx, filter, opts...)
}

func (c tracingCollection) FindOne(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) SingleResult {
	ctx, span := c.span(ctx, "findOne")
	defer span.End()
	return c.inner.FindOne(ctx, filter, opts...)
}

func (c tracingCollection) BulkWrite(ctx context.Context, models []mongo.WriteModel, opts ...options.Lister[options.BulkWriteOptions]) (BulkWriteResult, error) {
	ctx, span := c.span(ctx, "bulkWrite")
	defer span.End()
	return c.inner.BulkWrite(ctx, models, opts...)
}

func (c tracingCollection) ReplaceOne(ctx context.Context, filter any, replacement any, opts ...options.Lister[options.ReplaceOptions]) (UpdateResult, error) {
	ctx, span := c.span(ctx, "replaceOne")
	defer span.End()
	return c.inner.ReplaceOne(ctx, filter, replacement, opts...)
}

func (c tracingCollection) CountDocuments(ctx context.Context, filter any, opts ...options.Lister[options.CountOptions]) (int64, error) {
	ctx, span := c.span(ctx, "countDocuments")
	defer span.End()
	return c.inner.CountDocuments(ctx, filter, opts...)
}

func (c tracingCollection) UpdateOne(ctx context.Context, filter any, update any, opts ...options.Lister[options.UpdateOneOptions]) (UpdateResult, error) {
	ctx, span := c.span(ctx, "updateOne")
	defer span.End()
	return c.inner.UpdateOne(ctx, filter, update, opts...)
}

func (c tracingCollection) DeleteOne(ctx context.Context, filter any, opts ...options.Lister[options.DeleteOneOptions]) (DeleteResult, error) {
	ctx, span := c.span(ctx, "deleteOne")
	defer span.End()
	return c.inner.DeleteOne(ctx, filter, opts...)
}
