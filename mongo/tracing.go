package mongo

import (
	"context"

	"go.opentelemetry.io/otel/trace"

	"github.com/trypanic/go-sdk/v2/telemetry"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
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
// Usage (in bootstrap), on a client from NewWithoutTracing:
//
//	repo.NewRepository(mongo.WrapWithTracing(mongoClient))
//
// New already applies this wrapper; wrapping its result again opens two
// spans per operation.
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

func (t *tracingClientPort) Ping(ctx context.Context) (err error) {
	ctx, span := t.instrumenter.DB(ctx, "mongo", "client", "ping", telemetry.WithSpanKind(trace.SpanKindClient))
	defer func() { telemetry.End(span, err) }()
	return t.inner.Ping(ctx)
}

func (t *tracingClientPort) Close(ctx context.Context) (err error) {
	ctx, span := t.instrumenter.DB(ctx, "mongo", "client", "close", telemetry.WithSpanKind(trace.SpanKindClient))
	defer func() { telemetry.End(span, err) }()
	return t.inner.Close(ctx)
}

type tracingCollection struct {
	inner        Collection
	name         string
	instrumenter *telemetry.Instrumenter
}

func (c tracingCollection) span(ctx context.Context, op string) (context.Context, telemetry.Span) {
	return c.instrumenter.DB(ctx, "mongo", c.name, op, telemetry.WithSpanKind(trace.SpanKindClient))
}

func (c tracingCollection) InsertOne(ctx context.Context, doc any, opts ...options.Lister[options.InsertOneOptions]) (result InsertOneResult, err error) {
	ctx, span := c.span(ctx, "insertOne")
	defer func() { telemetry.End(span, err) }()
	return c.inner.InsertOne(ctx, doc, opts...)
}

func (c tracingCollection) InsertMany(ctx context.Context, docs any, opts ...options.Lister[options.InsertManyOptions]) (result InsertManyResult, err error) {
	ctx, span := c.span(ctx, "insertMany")
	defer func() { telemetry.End(span, err) }()
	return c.inner.InsertMany(ctx, docs, opts...)
}

// Find traces the query in "mongo.<collection>.find". The returned cursor
// traces All in its own span, "mongo.<collection>.find.all", because the
// find span ends when Find returns while All still fetches and decodes.
func (c tracingCollection) Find(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) (result Cursor, err error) {
	ctx, span := c.span(ctx, "find")
	defer func() { telemetry.End(span, err) }()
	cursor, err := c.inner.Find(ctx, filter, opts...)
	if err != nil || cursor == nil {
		return cursor, err
	}
	return tracingCursor{inner: cursor, spanName: "mongo." + c.name + ".find.all", instrumenter: c.instrumenter}, nil
}

func (c tracingCollection) FindOne(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) SingleResult {
	ctx, span := c.span(ctx, "findOne")
	defer span.End()
	inner := c.inner.FindOne(ctx, filter, opts...)
	if inner == nil {
		return nil
	}
	return tracingSingleResult{inner: inner, instrumenter: c.instrumenter, name: "mongo." + c.name + ".findOne.decode", parent: trace.SpanContextFromContext(ctx)}
}

func (c tracingCollection) BulkWrite(ctx context.Context, models []mongodriver.WriteModel, opts ...options.Lister[options.BulkWriteOptions]) (result BulkWriteResult, err error) {
	ctx, span := c.span(ctx, "bulkWrite")
	defer func() { telemetry.End(span, err) }()
	return c.inner.BulkWrite(ctx, models, opts...)
}

func (c tracingCollection) ReplaceOne(ctx context.Context, filter any, replacement any, opts ...options.Lister[options.ReplaceOptions]) (result UpdateResult, err error) {
	ctx, span := c.span(ctx, "replaceOne")
	defer func() { telemetry.End(span, err) }()
	return c.inner.ReplaceOne(ctx, filter, replacement, opts...)
}

func (c tracingCollection) CountDocuments(ctx context.Context, filter any, opts ...options.Lister[options.CountOptions]) (result int64, err error) {
	ctx, span := c.span(ctx, "countDocuments")
	defer func() { telemetry.End(span, err) }()
	return c.inner.CountDocuments(ctx, filter, opts...)
}

func (c tracingCollection) UpdateOne(ctx context.Context, filter any, update any, opts ...options.Lister[options.UpdateOneOptions]) (result UpdateResult, err error) {
	ctx, span := c.span(ctx, "updateOne")
	defer func() { telemetry.End(span, err) }()
	return c.inner.UpdateOne(ctx, filter, update, opts...)
}

func (c tracingCollection) DeleteOne(ctx context.Context, filter any, opts ...options.Lister[options.DeleteOneOptions]) (result DeleteResult, err error) {
	ctx, span := c.span(ctx, "deleteOne")
	defer func() { telemetry.End(span, err) }()
	return c.inner.DeleteOne(ctx, filter, opts...)
}

type tracingCursor struct {
	inner        Cursor
	spanName     string
	instrumenter *telemetry.Instrumenter
}

func (c tracingCursor) All(ctx context.Context, results any) (err error) {
	ctx, span := c.instrumenter.Start(ctx, c.spanName, telemetry.WithSpanKind(trace.SpanKindClient))
	defer func() { telemetry.End(span, err) }()
	return c.inner.All(ctx, results)
}

// Next, Decode and Err pass through without a span per document; the
// driver's getMore round trips run under the caller's context.
func (c tracingCursor) Next(ctx context.Context) bool { return c.inner.Next(ctx) }
func (c tracingCursor) Decode(v any) error            { return c.inner.Decode(v) }
func (c tracingCursor) Err() error                    { return c.inner.Err() }

func (c tracingCursor) Close(ctx context.Context) (err error) {
	return c.inner.Close(ctx)
}

// Unwrap returns the client enclosed by this decorator.
func (t *tracingClientPort) Unwrap() ClientPort { return t.inner }

type tracingSingleResult struct {
	parent       trace.SpanContext
	inner        SingleResult
	instrumenter *telemetry.Instrumenter
	name         string
}

func (r tracingSingleResult) Decode(value any) (err error) {
	_, span := r.instrumenter.Start(trace.ContextWithSpanContext(context.Background(), r.parent), r.name, telemetry.WithSpanKind(trace.SpanKindClient))
	defer func() { telemetry.End(span, err) }()
	return r.inner.Decode(value)
}
