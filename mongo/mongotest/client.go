// Package mongotest provides callback-based MongoDB test doubles without a server.
package mongotest

import (
	"context"

	mongodb "github.com/trypanic/go-sdk/mongo"
	driver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Client implements ClientPort with explicit callbacks. Unset callbacks panic.
// Configure before use; callbacks own any synchronization and resource cleanup.
type Client struct {
	CollectionFunc func(string) mongodb.Collection
	PingFunc       func(context.Context) error
	CloseFunc      func(context.Context) error
}

func (c *Client) Collection(name string) mongodb.Collection { return c.CollectionFunc(name) }
func (c *Client) Ping(ctx context.Context) error            { return c.PingFunc(ctx) }
func (c *Client) Close(ctx context.Context) error           { return c.CloseFunc(ctx) }

var _ mongodb.ClientPort = (*Client)(nil)

// SingleResult adapts a decoding callback to the SDK result contract.
type SingleResult func(any) error

func (s SingleResult) Decode(out any) error { return s(out) }

// Collection delegates operations to callbacks. Unset callbacks panic, exposing
// unexpected calls. Configure before use; callbacks own synchronization.
type Collection struct {
	InsertOneFunc      func(ctx context.Context, doc any, opts ...options.Lister[options.InsertOneOptions]) (mongodb.InsertOneResult, error)
	InsertManyFunc     func(ctx context.Context, docs any, opts ...options.Lister[options.InsertManyOptions]) (mongodb.InsertManyResult, error)
	FindFunc           func(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) (mongodb.Cursor, error)
	FindOneFunc        func(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) mongodb.SingleResult
	BulkWriteFunc      func(ctx context.Context, models []driver.WriteModel, opts ...options.Lister[options.BulkWriteOptions]) (mongodb.BulkWriteResult, error)
	ReplaceOneFunc     func(ctx context.Context, filter, replacement any, opts ...options.Lister[options.ReplaceOptions]) (mongodb.UpdateResult, error)
	CountDocumentsFunc func(ctx context.Context, filter any, opts ...options.Lister[options.CountOptions]) (int64, error)
	UpdateOneFunc      func(ctx context.Context, filter, update any, opts ...options.Lister[options.UpdateOneOptions]) (mongodb.UpdateResult, error)
	DeleteOneFunc      func(ctx context.Context, filter any, opts ...options.Lister[options.DeleteOneOptions]) (mongodb.DeleteResult, error)
}

func (c *Collection) InsertOne(ctx context.Context, doc any, opts ...options.Lister[options.InsertOneOptions]) (mongodb.InsertOneResult, error) {
	return c.InsertOneFunc(ctx, doc, opts...)
}
func (c *Collection) InsertMany(ctx context.Context, docs any, opts ...options.Lister[options.InsertManyOptions]) (mongodb.InsertManyResult, error) {
	return c.InsertManyFunc(ctx, docs, opts...)
}
func (c *Collection) Find(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) (mongodb.Cursor, error) {
	return c.FindFunc(ctx, filter, opts...)
}
func (c *Collection) FindOne(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) mongodb.SingleResult {
	return c.FindOneFunc(ctx, filter, opts...)
}
func (c *Collection) BulkWrite(ctx context.Context, models []driver.WriteModel, opts ...options.Lister[options.BulkWriteOptions]) (mongodb.BulkWriteResult, error) {
	return c.BulkWriteFunc(ctx, models, opts...)
}
func (c *Collection) ReplaceOne(ctx context.Context, filter, replacement any, opts ...options.Lister[options.ReplaceOptions]) (mongodb.UpdateResult, error) {
	return c.ReplaceOneFunc(ctx, filter, replacement, opts...)
}
func (c *Collection) CountDocuments(ctx context.Context, filter any, opts ...options.Lister[options.CountOptions]) (int64, error) {
	return c.CountDocumentsFunc(ctx, filter, opts...)
}
func (c *Collection) UpdateOne(ctx context.Context, filter, update any, opts ...options.Lister[options.UpdateOneOptions]) (mongodb.UpdateResult, error) {
	return c.UpdateOneFunc(ctx, filter, update, opts...)
}
func (c *Collection) DeleteOne(ctx context.Context, filter any, opts ...options.Lister[options.DeleteOneOptions]) (mongodb.DeleteResult, error) {
	return c.DeleteOneFunc(ctx, filter, opts...)
}

var _ mongodb.Collection = (*Collection)(nil)

// Cursor implements streaming and batch decoding through explicit callbacks.
// Unset callbacks panic. Like a driver cursor, use it from one goroutine at a time.
type Cursor struct {
	NextFunc   func(context.Context) bool
	DecodeFunc func(any) error
	ErrFunc    func() error
	AllFunc    func(context.Context, any) error
	CloseFunc  func(context.Context) error
}

func (c *Cursor) Next(ctx context.Context) bool          { return c.NextFunc(ctx) }
func (c *Cursor) Decode(out any) error                   { return c.DecodeFunc(out) }
func (c *Cursor) Err() error                             { return c.ErrFunc() }
func (c *Cursor) All(ctx context.Context, out any) error { return c.AllFunc(ctx, out) }
func (c *Cursor) Close(ctx context.Context) error        { return c.CloseFunc(ctx) }

var _ mongodb.Cursor = (*Cursor)(nil)
