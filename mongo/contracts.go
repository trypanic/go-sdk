package mongodb

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type ClientPort interface {
	Collection(name string) Collection
	Ping(ctx context.Context) error
	Close(ctx context.Context) error
}

type Collection interface {
	InsertOne(ctx context.Context, doc any, opts ...options.Lister[options.InsertOneOptions]) (InsertOneResult, error)
	InsertMany(ctx context.Context, docs any, opts ...options.Lister[options.InsertManyOptions]) (InsertManyResult, error)
	Find(ctx context.Context, filter any, opts ...options.Lister[options.FindOptions]) (Cursor, error)
	FindOne(ctx context.Context, filter any, opts ...options.Lister[options.FindOneOptions]) SingleResult
	BulkWrite(ctx context.Context, models []mongo.WriteModel, opts ...options.Lister[options.BulkWriteOptions]) (BulkWriteResult, error)
	ReplaceOne(
		ctx context.Context,
		filter any,
		replacement any,
		opts ...options.Lister[options.ReplaceOptions],
	) (UpdateResult, error)
	CountDocuments(ctx context.Context, filter any, opts ...options.Lister[options.CountOptions]) (int64, error)
	UpdateOne(
		ctx context.Context,
		filter any,
		update any,
		opts ...options.Lister[options.UpdateOneOptions],
	) (UpdateResult, error)
	DeleteOne(ctx context.Context, filter any, opts ...options.Lister[options.DeleteOneOptions]) (DeleteResult, error)
}

type InsertOneResult interface {
	GetInsertedID() any
}

type InsertManyResult interface {
	GetInsertedIDs() []any
}

type Cursor interface {
	All(ctx context.Context, results any) error
	Close(ctx context.Context) error
}

type SingleResult interface {
	Decode(v any) error
}

// BulkWriteResult is a marker interface for bulk write operation results.
// The mongo driver does not expose a typed interface for BulkWriteResult,
// so this is an empty interface used for consistent return types.
type BulkWriteResult interface{}

type UpdateResult interface {
	GetMatchedCount() int64
	GetModifiedCount() int64
}

type DeleteResult interface {
	GetDeletedCount() int64
}
