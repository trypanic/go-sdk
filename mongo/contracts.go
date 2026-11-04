package mongodb

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ClientPort is a connected MongoDB client bound to one database.
type ClientPort interface {
	Collection(name string) Collection
	Ping(ctx context.Context) error
	Close(ctx context.Context) error
}

// Collection mirrors the *mongo.Collection methods the SDK supports. Errors
// are classified by WrapOperationError.
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

// InsertOneResult reports the _id of the inserted document.
type InsertOneResult interface {
	GetInsertedID() any
}

// InsertManyResult reports the _id of each inserted document, in order.
type InsertManyResult interface {
	GetInsertedIDs() []any
}

// Cursor iterates over the documents returned by Find. Stream large
// results with Next/Decode/Err instead of buffering them with All:
//
//	defer cursor.Close(ctx)
//	for cursor.Next(ctx) {
//		var doc Product
//		if err := cursor.Decode(&doc); err != nil {
//			return err // ERR_DB_MONGO_DECODE_FAILED
//		}
//	}
//	return cursor.Err() // nil, or the classified iteration error
type Cursor interface {
	// Next advances to the next document. It returns false when the cursor
	// is exhausted or an error occurred; check Err afterwards.
	Next(ctx context.Context) bool
	// Decode decodes the current document into v.
	Decode(v any) error
	// Err returns the error that stopped iteration, if any.
	Err() error
	// All decodes every remaining document into results (a pointer to a
	// slice) and closes the cursor.
	All(ctx context.Context, results any) error
	Close(ctx context.Context) error
}

// SingleResult is the document returned by FindOne. Decode reports the
// operation error (mongo.ErrNoDocuments is ERR_DB_MONGO_NOT_FOUND) or
// ERR_DB_MONGO_DECODE_FAILED.
type SingleResult interface {
	Decode(v any) error
}

// BulkWriteResult reports the counts of a BulkWrite operation.
type BulkWriteResult interface {
	GetInsertedCount() int64
	GetMatchedCount() int64
	GetModifiedCount() int64
	GetDeletedCount() int64
	GetUpsertedCount() int64
	// GetUpsertedIDs maps the index of each upserting write model to the
	// _id of the upserted document.
	GetUpsertedIDs() map[int64]any
}

// UpdateResult reports the counts of an UpdateOne or ReplaceOne operation.
type UpdateResult interface {
	GetMatchedCount() int64
	GetModifiedCount() int64
}

// DeleteResult reports the number of deleted documents.
type DeleteResult interface {
	GetDeletedCount() int64
}
