package mongodb

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var _ ClientPort = (*Client)(nil)

type collectionAdapter struct {
	collection *mongo.Collection
}

type insertOneResultAdapter struct {
	result *mongo.InsertOneResult
}

func (r insertOneResultAdapter) GetInsertedID() any { return r.result.InsertedID }

type insertManyResultAdapter struct {
	result *mongo.InsertManyResult
}

func (r insertManyResultAdapter) GetInsertedIDs() []any { return r.result.InsertedIDs }

type cursorAdapter struct {
	cursor *mongo.Cursor
}

func (c cursorAdapter) All(ctx context.Context, results any) error {
	if err := c.cursor.All(ctx, results); err != nil {
		return WrapDecodeError(err, "find")
	}
	return nil
}

func (c cursorAdapter) Close(ctx context.Context) error {
	return c.cursor.Close(ctx)
}

type singleResultAdapter struct {
	result *mongo.SingleResult
}

func (s singleResultAdapter) Decode(v any) error {
	if err := s.result.Decode(v); err != nil {
		return WrapOperationError(err, "find one")
	}
	return nil
}

type bulkWriteResultAdapter struct {
	result *mongo.BulkWriteResult
}

type updateResultAdapter struct {
	result *mongo.UpdateResult
}

func (r updateResultAdapter) GetMatchedCount() int64  { return r.result.MatchedCount }
func (r updateResultAdapter) GetModifiedCount() int64 { return r.result.ModifiedCount }

type deleteResultAdapter struct {
	result *mongo.DeleteResult
}

func (r deleteResultAdapter) GetDeletedCount() int64 { return r.result.DeletedCount }

var _ Collection = (*collectionAdapter)(nil)

func (c collectionAdapter) InsertOne(
	ctx context.Context,
	doc any,
	opts ...options.Lister[options.InsertOneOptions],
) (InsertOneResult, error) {
	result, err := c.collection.InsertOne(ctx, doc, opts...)
	if err != nil {
		return nil, WrapOperationError(err, "insert one")
	}
	return insertOneResultAdapter{result: result}, nil
}

func (c collectionAdapter) InsertMany(
	ctx context.Context,
	docs any,
	opts ...options.Lister[options.InsertManyOptions],
) (InsertManyResult, error) {
	result, err := c.collection.InsertMany(ctx, docs, opts...)
	if err != nil {
		return nil, WrapOperationError(err, "insert many")
	}
	return insertManyResultAdapter{result: result}, nil
}

func (c collectionAdapter) Find(
	ctx context.Context,
	filter any,
	opts ...options.Lister[options.FindOptions],
) (Cursor, error) {
	cursor, err := c.collection.Find(ctx, filter, opts...)
	if err != nil {
		return nil, WrapOperationError(err, "find")
	}
	return cursorAdapter{cursor: cursor}, nil
}

func (c collectionAdapter) FindOne(
	ctx context.Context,
	filter any,
	opts ...options.Lister[options.FindOneOptions],
) SingleResult {
	return singleResultAdapter{result: c.collection.FindOne(ctx, filter, opts...)}
}

func (c collectionAdapter) BulkWrite(
	ctx context.Context,
	models []mongo.WriteModel,
	opts ...options.Lister[options.BulkWriteOptions],
) (BulkWriteResult, error) {
	result, err := c.collection.BulkWrite(ctx, models, opts...)
	if err != nil {
		return nil, WrapOperationError(err, "bulk write")
	}
	return bulkWriteResultAdapter{result: result}, nil
}

func (c collectionAdapter) ReplaceOne(
	ctx context.Context,
	filter any,
	replacement any,
	opts ...options.Lister[options.ReplaceOptions],
) (UpdateResult, error) {
	result, err := c.collection.ReplaceOne(ctx, filter, replacement, opts...)
	if err != nil {
		return nil, WrapOperationError(err, "replace one")
	}
	return updateResultAdapter{result: result}, nil
}

func (c collectionAdapter) CountDocuments(
	ctx context.Context,
	filter any,
	opts ...options.Lister[options.CountOptions],
) (int64, error) {
	count, err := c.collection.CountDocuments(ctx, filter, opts...)
	if err != nil {
		return 0, WrapOperationError(err, "count documents")
	}
	return count, nil
}

func (c collectionAdapter) UpdateOne(
	ctx context.Context,
	filter any,
	update any,
	opts ...options.Lister[options.UpdateOneOptions],
) (UpdateResult, error) {
	result, err := c.collection.UpdateOne(ctx, filter, update, opts...)
	if err != nil {
		return nil, WrapOperationError(err, "update one")
	}
	return updateResultAdapter{result: result}, nil
}

func (c collectionAdapter) DeleteOne(
	ctx context.Context,
	filter any,
	opts ...options.Lister[options.DeleteOneOptions],
) (DeleteResult, error) {
	result, err := c.collection.DeleteOne(ctx, filter, opts...)
	if err != nil {
		return nil, WrapOperationError(err, "delete one")
	}
	return deleteResultAdapter{result: result}, nil
}
