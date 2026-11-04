package mongodb

import (
	"context"
	"fmt"
	"reflect"

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

// All decodes every remaining document into results, which must be a
// pointer to a slice, and closes the cursor. It iterates with Next/Decode
// instead of mongo.Cursor.All so that a failure while fetching a batch is
// classified by WrapOperationError and only a failure while decoding a
// document is ERR_DB_MONGO_DECODE_FAILED. Existing elements of the slice are
// reused and the slice is truncated to the number of documents, as
// mongo.Cursor.All does.
func (c cursorAdapter) All(ctx context.Context, results any) error {
	defer func() { _ = c.cursor.Close(context.WithoutCancel(ctx)) }()

	resultsVal := reflect.ValueOf(results)
	if resultsVal.Kind() != reflect.Pointer || resultsVal.IsNil() {
		return WrapDecodeError(fmt.Errorf("results argument must be a pointer to a slice, got %T", results), "find")
	}
	sliceVal := resultsVal.Elem()
	if sliceVal.Kind() == reflect.Interface {
		sliceVal = sliceVal.Elem()
	}
	if sliceVal.Kind() != reflect.Slice {
		return WrapDecodeError(fmt.Errorf("results argument must be a pointer to a slice, got %T", results), "find")
	}

	elemType := sliceVal.Type().Elem()
	index := 0
	for c.cursor.Next(ctx) {
		if index == sliceVal.Len() {
			sliceVal = reflect.Append(sliceVal, reflect.New(elemType).Elem())
			sliceVal = sliceVal.Slice(0, sliceVal.Cap())
		}
		if err := c.cursor.Decode(sliceVal.Index(index).Addr().Interface()); err != nil {
			return WrapDecodeError(err, "find")
		}
		index++
	}
	if err := c.cursor.Err(); err != nil {
		return WrapOperationError(err, "find")
	}

	resultsVal.Elem().Set(sliceVal.Slice(0, index))
	return nil
}

func (c cursorAdapter) Next(ctx context.Context) bool { return c.cursor.Next(ctx) }

// Decode decodes the current document; a failure is ERR_DB_MONGO_DECODE_FAILED.
func (c cursorAdapter) Decode(v any) error {
	return WrapDecodeError(c.cursor.Decode(v), "find")
}

// Err classifies the error that stopped iteration with WrapOperationError.
func (c cursorAdapter) Err() error {
	return WrapOperationError(c.cursor.Err(), "find")
}

func (c cursorAdapter) Close(ctx context.Context) error {
	return c.cursor.Close(ctx)
}

type singleResultAdapter struct {
	result *mongo.SingleResult
}

// Decode reports the error of the FindOne operation (including
// mongo.ErrNoDocuments) through WrapOperationError and a failure to decode
// the returned document as ERR_DB_MONGO_DECODE_FAILED.
func (s singleResultAdapter) Decode(v any) error {
	if err := s.result.Err(); err != nil {
		return WrapOperationError(err, "find one")
	}
	if err := s.result.Decode(v); err != nil {
		return WrapDecodeError(err, "find one")
	}
	return nil
}

type bulkWriteResultAdapter struct {
	result *mongo.BulkWriteResult
}

func (r bulkWriteResultAdapter) GetInsertedCount() int64       { return r.result.InsertedCount }
func (r bulkWriteResultAdapter) GetMatchedCount() int64        { return r.result.MatchedCount }
func (r bulkWriteResultAdapter) GetModifiedCount() int64       { return r.result.ModifiedCount }
func (r bulkWriteResultAdapter) GetDeletedCount() int64        { return r.result.DeletedCount }
func (r bulkWriteResultAdapter) GetUpsertedCount() int64       { return r.result.UpsertedCount }
func (r bulkWriteResultAdapter) GetUpsertedIDs() map[int64]any { return r.result.UpsertedIDs }

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
