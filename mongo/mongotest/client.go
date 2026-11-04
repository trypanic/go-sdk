package mongotest

import (
	"context"

	"github.com/trypanic/go-sdk/v2/mongo"
	driver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Client implements ClientPort with explicit callbacks. Unset callbacks panic.
// Configure before use; callbacks own any synchronization and resource cleanup.
type Client struct {
	CollectionFunc func(string) mongo.Collection
	PingFunc       func(context.Context) error
	CloseFunc      func(context.Context) error
}

// Collection delegates to the configured callback.
func (c *Client) Collection(name string) mongo.Collection { return c.CollectionFunc(name) }

// Ping delegates to the configured callback.
func (c *Client) Ping(ctx context.Context) error { return c.PingFunc(ctx) }

// Close delegates to the configured callback.
func (c *Client) Close(ctx context.Context) error { return c.CloseFunc(ctx) }

var _ mongo.ClientPort = (*Client)(nil)

// SingleResult adapts a decoding callback to the SDK result contract.
type SingleResult func(any) error

// Decode delegates to the configured callback.
func (s SingleResult) Decode(out any) error { return s(out) }

// Collection delegates operations to callbacks. Unset callbacks panic, exposing
// unexpected calls. Configure before use; callbacks own synchronization.
type Collection struct {
	InsertOneFunc      func(ctx context.Context, doc any, opts ...options.Lister[options.InsertOneOptions]) (mongo.InsertOneResult, error)
	InsertManyFunc     func(ctx context.Context, docs any, opts ...options.Lister[options.InsertManyOptions]) (mongo.InsertManyResult, error)
	FindFunc           func(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) (mongo.Cursor, error)
	FindOneFunc        func(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) mongo.SingleResult
	BulkWriteFunc      func(ctx context.Context, models []driver.WriteModel, opts ...options.Lister[options.BulkWriteOptions]) (mongo.BulkWriteResult, error)
	ReplaceOneFunc     func(ctx context.Context, filter, replacement any, opts ...options.Lister[options.ReplaceOptions]) (mongo.UpdateResult, error)
	CountDocumentsFunc func(ctx context.Context, filter any, opts ...options.Lister[options.CountOptions]) (int64, error)
	UpdateOneFunc      func(ctx context.Context, filter, update any, opts ...options.Lister[options.UpdateOneOptions]) (mongo.UpdateResult, error)
	DeleteOneFunc      func(ctx context.Context, filter any, opts ...options.Lister[options.DeleteOneOptions]) (mongo.DeleteResult, error)
}

// InsertOne delegates to the configured callback.
func (c *Collection) InsertOne(ctx context.Context, doc any, opts ...options.Lister[options.InsertOneOptions]) (mongo.InsertOneResult, error) {
	return c.InsertOneFunc(ctx, doc, opts...)
}

// InsertMany delegates to the configured callback.
func (c *Collection) InsertMany(ctx context.Context, docs any, opts ...options.Lister[options.InsertManyOptions]) (mongo.InsertManyResult, error) {
	return c.InsertManyFunc(ctx, docs, opts...)
}

// Find delegates to the configured callback.
func (c *Collection) Find(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) (mongo.Cursor, error) {
	return c.FindFunc(ctx, filter, opts...)
}

// FindOne delegates to the configured callback.
func (c *Collection) FindOne(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) mongo.SingleResult {
	return c.FindOneFunc(ctx, filter, opts...)
}

// BulkWrite delegates to the configured callback.
func (c *Collection) BulkWrite(ctx context.Context, models []driver.WriteModel, opts ...options.Lister[options.BulkWriteOptions]) (mongo.BulkWriteResult, error) {
	return c.BulkWriteFunc(ctx, models, opts...)
}

// ReplaceOne delegates to the configured callback.
func (c *Collection) ReplaceOne(ctx context.Context, filter, replacement any, opts ...options.Lister[options.ReplaceOptions]) (mongo.UpdateResult, error) {
	return c.ReplaceOneFunc(ctx, filter, replacement, opts...)
}

// CountDocuments delegates to the configured callback.
func (c *Collection) CountDocuments(ctx context.Context, filter any, opts ...options.Lister[options.CountOptions]) (int64, error) {
	return c.CountDocumentsFunc(ctx, filter, opts...)
}

// UpdateOne delegates to the configured callback.
func (c *Collection) UpdateOne(ctx context.Context, filter, update any, opts ...options.Lister[options.UpdateOneOptions]) (mongo.UpdateResult, error) {
	return c.UpdateOneFunc(ctx, filter, update, opts...)
}

// DeleteOne delegates to the configured callback.
func (c *Collection) DeleteOne(ctx context.Context, filter any, opts ...options.Lister[options.DeleteOneOptions]) (mongo.DeleteResult, error) {
	return c.DeleteOneFunc(ctx, filter, opts...)
}

var _ mongo.Collection = (*Collection)(nil)

// Cursor implements streaming and batch decoding through explicit callbacks.
// Unset callbacks panic. Like a driver cursor, use it from one goroutine at a time.
type Cursor struct {
	NextFunc   func(context.Context) bool
	DecodeFunc func(any) error
	ErrFunc    func() error
	AllFunc    func(context.Context, any) error
	CloseFunc  func(context.Context) error
}

// Next delegates to the configured callback.
func (c *Cursor) Next(ctx context.Context) bool { return c.NextFunc(ctx) }

// Decode delegates to the configured callback.
func (c *Cursor) Decode(out any) error { return c.DecodeFunc(out) }

// Err delegates to the configured callback.
func (c *Cursor) Err() error { return c.ErrFunc() }

// All delegates to the configured callback.
func (c *Cursor) All(ctx context.Context, out any) error { return c.AllFunc(ctx, out) }

// Close delegates to the configured callback.
func (c *Cursor) Close(ctx context.Context) error { return c.CloseFunc(ctx) }

var _ mongo.Cursor = (*Cursor)(nil)
