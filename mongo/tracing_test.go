package mongo

import (
	"context"
	"slices"
	"testing"

	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/trypanic/go-sdk/telemetry"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
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
func (stubCollection) BulkWrite(context.Context, []mongodriver.WriteModel, ...options.Lister[options.BulkWriteOptions]) (BulkWriteResult, error) {
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

type recordingCursor struct {
	allSpan trace.SpanContext
}

func (c *recordingCursor) All(ctx context.Context, _ any) error {
	c.allSpan = trace.SpanFromContext(ctx).SpanContext()
	return nil
}
func (c *recordingCursor) Close(context.Context) error { return nil }
func (c *recordingCursor) Next(context.Context) bool   { return false }
func (c *recordingCursor) Decode(any) error            { return nil }
func (c *recordingCursor) Err() error                  { return nil }

type cursorCollection struct {
	stubCollection
	cursor Cursor
}

func (c cursorCollection) Find(context.Context, any, ...options.Lister[options.FindOptions]) (Cursor, error) {
	return c.cursor, nil
}

type cursorClientPort struct {
	stubClientPort
	collection Collection
}

func (c *cursorClientPort) Collection(string) Collection { return c.collection }

func newRecordingInstrumenter() (*telemetry.Instrumenter, *tracetest.InMemoryExporter) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	return telemetry.NewInstrumenter(telemetry.InstrumenterConfig{ScopeName: "mongo-test", TracerProvider: provider}), exporter
}

// TestTracingCursorAllRunsInsideASpan covers C4-019: the find span ended
// when Find returned, so Cursor.All (getMore round trips and decoding) ran
// outside any span.
func TestTracingCursorAllRunsInsideASpan(t *testing.T) {
	t.Parallel()

	instrumenter, exporter := newRecordingInstrumenter()
	cursor := &recordingCursor{}
	client := WrapWithInstrumenter(&cursorClientPort{collection: cursorCollection{cursor: cursor}}, instrumenter)

	found, err := client.Collection("products").Find(t.Context(), nil)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	var out []any
	if err := found.All(t.Context(), &out); err != nil {
		t.Fatalf("All: %v", err)
	}

	spans := map[string]sdktrace.ReadOnlySpan{}
	var names []string
	for _, span := range exporter.GetSpans().Snapshots() {
		spans[span.Name()] = span
		names = append(names, span.Name())
	}
	if _, ok := spans["mongo.products.find"]; !ok {
		t.Fatalf("missing span mongodriver.products.find; got %v", names)
	}
	allSpan, ok := spans["mongo.products.find.all"]
	if !ok {
		t.Fatalf("missing span mongodriver.products.find.all; got %v", names)
	}
	if cursor.allSpan.SpanID() != allSpan.SpanContext().SpanID() {
		t.Fatal("Cursor.All did not run inside the mongodriver.products.find.all span")
	}
}

func TestTracingClientPortTracesPingAndClose(t *testing.T) {
	t.Parallel()

	instrumenter, exporter := newRecordingInstrumenter()
	client := WrapWithInstrumenter(&stubClientPort{}, instrumenter)
	if err := client.Ping(t.Context()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if err := client.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	var names []string
	for _, span := range exporter.GetSpans() {
		names = append(names, span.Name)
	}
	if len(names) != 2 || names[0] != "mongo.client.ping" || names[1] != "mongo.client.close" {
		t.Fatalf("spans = %v, want [mongodriver.client.ping mongodriver.client.close]", names)
	}
}

// TestTracingCollectionSpanNames asserts the documented span name
// "mongo.<collection>.<operation>" for every Collection method (C4-020: the
// tracing tests checked only the wrapper type).
func TestTracingCollectionSpanNames(t *testing.T) {
	t.Parallel()

	instrumenter, exporter := newRecordingInstrumenter()
	coll := WrapWithInstrumenter(&stubClientPort{}, instrumenter).Collection("orders")
	ctx := t.Context()

	_, _ = coll.InsertOne(ctx, nil)
	_, _ = coll.InsertMany(ctx, nil)
	_, _ = coll.Find(ctx, nil)
	_ = coll.FindOne(ctx, nil)
	_, _ = coll.BulkWrite(ctx, nil)
	_, _ = coll.ReplaceOne(ctx, nil, nil)
	_, _ = coll.CountDocuments(ctx, nil)
	_, _ = coll.UpdateOne(ctx, nil, nil)
	_, _ = coll.DeleteOne(ctx, nil)

	want := []string{
		"mongo.orders.insertOne", "mongo.orders.insertMany", "mongo.orders.find", "mongo.orders.findOne",
		"mongo.orders.bulkWrite", "mongo.orders.replaceOne", "mongo.orders.countDocuments",
		"mongo.orders.updateOne", "mongo.orders.deleteOne",
	}
	var got []string
	for _, span := range exporter.GetSpans() {
		got = append(got, span.Name)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("span names\n got  %v\n want %v", got, want)
	}
}
