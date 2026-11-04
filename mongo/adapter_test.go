package mongodb

import (
	"context"
	"errors"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type namedDoc struct {
	Name string `bson:"name"`
}

func requireCode(t *testing.T, err error, want errorkit.ErrorCode) *errorkit.AppError {
	t.Helper()

	appErr, ok := errors.AsType[*errorkit.AppError](err)
	if !ok {
		t.Fatalf("expected *errorkit.AppError, got %T (%v)", err, err)
	}
	if appErr.Code() != want {
		t.Fatalf("code = %s, want %s (err: %v)", appErr.Code(), want, err)
	}
	return appErr
}

func newCursor(t *testing.T, docs []any, preloadedErr error) cursorAdapter {
	t.Helper()

	cursor, err := mongo.NewCursorFromDocuments(docs, preloadedErr, nil)
	if err != nil {
		t.Fatalf("NewCursorFromDocuments: %v", err)
	}
	return cursorAdapter{cursor: cursor}
}

// TestSingleResultAdapterDecode covers C4-010 (1): a decode failure on
// FindOne must be ERR_DB_MONGO_DECODE_FAILED as the README documents, while
// errors from the operation itself keep the operation mapping.
func TestSingleResultAdapterDecode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		doc      any
		opErr    error
		wantCode errorkit.ErrorCode
	}{
		{"decode failure", bson.D{{Key: "name", Value: int32(7)}}, nil, ERR_DB_MONGO_DECODE_FAILED},
		{"no documents", bson.D{}, mongo.ErrNoDocuments, ERR_DB_MONGO_NOT_FOUND},
		{"operation failure", bson.D{}, errors.New("server said no"), ERR_DB_MONGO_ERROR},
		{"operation deadline", bson.D{}, context.DeadlineExceeded, ERR_DB_MONGO_TIMEOUT},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			adapter := singleResultAdapter{result: mongo.NewSingleResultFromDocument(tt.doc, tt.opErr, nil)}
			var out namedDoc
			appErr := requireCode(t, adapter.Decode(&out), tt.wantCode)
			if tt.opErr != nil && !errors.Is(appErr, tt.opErr) {
				t.Fatalf("cause %v not wrapped: %v", tt.opErr, appErr)
			}
		})
	}
}

func TestSingleResultAdapterDecodeSuccess(t *testing.T) {
	t.Parallel()

	adapter := singleResultAdapter{result: mongo.NewSingleResultFromDocument(bson.D{{Key: "name", Value: "widget"}}, nil, nil)}
	var out namedDoc
	if err := adapter.Decode(&out); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if out.Name != "widget" {
		t.Fatalf("Name = %q, want %q", out.Name, "widget")
	}
}

// TestCursorAdapterAll covers C4-010 (2): only decode failures are
// ERR_DB_MONGO_DECODE_FAILED; an iteration error keeps the operation mapping.
func TestCursorAdapterAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		docs     []any
		iterErr  error
		wantCode errorkit.ErrorCode
	}{
		{"decode failure", []any{bson.D{{Key: "name", Value: "ok"}}, bson.D{{Key: "name", Value: int32(7)}}}, nil, ERR_DB_MONGO_DECODE_FAILED},
		{"iteration failure", []any{bson.D{{Key: "name", Value: "ok"}}}, errors.New("getMore failed"), ERR_DB_MONGO_ERROR},
		{"iteration deadline", nil, context.DeadlineExceeded, ERR_DB_MONGO_TIMEOUT},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out []namedDoc
			appErr := requireCode(t, newCursor(t, tt.docs, tt.iterErr).All(context.Background(), &out), tt.wantCode)
			if tt.iterErr != nil && !errors.Is(appErr, tt.iterErr) {
				t.Fatalf("cause %v not wrapped: %v", tt.iterErr, appErr)
			}
		})
	}
}

func TestCursorAdapterAllSuccess(t *testing.T) {
	t.Parallel()

	docs := []any{bson.D{{Key: "name", Value: "a"}}, bson.D{{Key: "name", Value: "b"}}}
	out := []namedDoc{{Name: "stale"}, {Name: "stale"}, {Name: "stale"}}
	if err := newCursor(t, docs, nil).All(context.Background(), &out); err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(out) != 2 || out[0].Name != "a" || out[1].Name != "b" {
		t.Fatalf("out = %+v, want [a b]", out)
	}

	var empty []namedDoc
	if err := newCursor(t, nil, nil).All(context.Background(), &empty); err != nil {
		t.Fatalf("All on empty cursor: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty result = %#v, want no elements", empty)
	}
}

func TestCursorAdapterAllRejectsNonSlice(t *testing.T) {
	t.Parallel()

	var out namedDoc
	requireCode(t, newCursor(t, nil, nil).All(context.Background(), &out), ERR_DB_MONGO_DECODE_FAILED)
}

// TestCursorAdapterStreams covers C4-021: Cursor exposed only All, so every
// Find had to materialize the whole result in memory.
func TestCursorAdapterStreams(t *testing.T) {
	t.Parallel()

	var cursor Cursor = newCursor(t, []any{bson.D{{Key: "name", Value: "a"}}, bson.D{{Key: "name", Value: "b"}}}, nil)
	var names []string
	for cursor.Next(t.Context()) {
		var doc namedDoc
		if err := cursor.Decode(&doc); err != nil {
			t.Fatalf("Decode: %v", err)
		}
		names = append(names, doc.Name)
	}
	if err := cursor.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}
	if err := cursor.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("names = %v, want [a b]", names)
	}
}

func TestCursorAdapterStreamErrors(t *testing.T) {
	t.Parallel()

	t.Run("decode failure", func(t *testing.T) {
		t.Parallel()
		var cursor Cursor = newCursor(t, []any{bson.D{{Key: "name", Value: int32(7)}}}, nil)
		if !cursor.Next(t.Context()) {
			t.Fatal("Next = false, want a document")
		}
		var doc namedDoc
		requireCode(t, cursor.Decode(&doc), ERR_DB_MONGO_DECODE_FAILED)
	})

	t.Run("iteration failure", func(t *testing.T) {
		t.Parallel()
		cause := context.DeadlineExceeded
		var cursor Cursor = newCursor(t, []any{bson.D{{Key: "name", Value: "a"}}}, cause)
		if cursor.Next(t.Context()) {
			t.Fatal("Next = true, want false after an iteration error")
		}
		appErr := requireCode(t, cursor.Err(), ERR_DB_MONGO_TIMEOUT)
		if !errors.Is(appErr, cause) {
			t.Fatalf("cause not wrapped: %v", appErr)
		}
	})
}

func TestBulkWriteResultAdapterGetters(t *testing.T) {
	t.Parallel()

	var result BulkWriteResult = bulkWriteResultAdapter{result: &mongo.BulkWriteResult{
		InsertedCount: 1, MatchedCount: 2, ModifiedCount: 3, DeletedCount: 4, UpsertedCount: 5,
		UpsertedIDs: map[int64]any{7: "id-7"},
	}}
	if result.GetInsertedCount() != 1 || result.GetMatchedCount() != 2 || result.GetModifiedCount() != 3 ||
		result.GetDeletedCount() != 4 || result.GetUpsertedCount() != 5 {
		t.Fatalf("counts = %d %d %d %d %d, want 1 2 3 4 5", result.GetInsertedCount(), result.GetMatchedCount(),
			result.GetModifiedCount(), result.GetDeletedCount(), result.GetUpsertedCount())
	}
	if ids := result.GetUpsertedIDs(); len(ids) != 1 || ids[7] != "id-7" {
		t.Fatalf("upserted IDs = %v, want map[7:id-7]", ids)
	}
}
