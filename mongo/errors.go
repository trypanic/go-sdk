package mongodb

import (
	"context"
	"errors"

	"github.com/trypanic/go-sdk/errorkit"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ParseObjectID parses a hex ObjectID and normalizes format errors.
func ParseObjectID(id string) (bson.ObjectID, error) {
	objectID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return bson.NilObjectID, errorkit.NewError(errorkit.ERR_VALIDATION_INVALID_FORMAT).With(
			errorkit.WithReason("invalid document ID"),
			errorkit.WithWrapped(err),
		)
	}

	return objectID, nil
}

// ExtractInsertedIDHex converts InsertedID to hex and normalizes type errors.
func ExtractInsertedIDHex(insertedID any) (string, error) {
	id, ok := insertedID.(bson.ObjectID)
	if !ok {
		return "", errorkit.NewError(ERR_DB_MONGO_ERROR).With(
			errorkit.WithReason("inserted id is not ObjectID"),
		)
	}
	if id == bson.NilObjectID {
		return "", errorkit.NewError(ERR_DB_MONGO_ERROR).With(
			errorkit.WithReason("mongo inserted id is nil"),
		)
	}
	return id.Hex(), nil
}

// WrapOperationError maps raw Mongo driver errors to standardized errorkit codes.
func WrapOperationError(err error, operation string) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return errorkit.NewError(ERR_DB_MONGO_TIMEOUT).With(
			errorkit.WithReason("mongo %s timed out", operation),
			errorkit.WithWrapped(err),
		)
	}

	if errors.Is(err, mongo.ErrNoDocuments) {
		return errorkit.NewError(ERR_DB_MONGO_NOT_FOUND).With(
			errorkit.WithReason("mongo %s returned no documents", operation),
			errorkit.WithWrapped(err),
		)
	}

	if mongo.IsDuplicateKeyError(err) {
		return errorkit.NewError(errorkit.ERR_VALIDATION_DUPLICATE).With(
			errorkit.WithReason("mongo %s violated unique constraint", operation),
			errorkit.WithWrapped(err),
		)
	}

	return errorkit.NewError(ERR_DB_MONGO_ERROR).With(
		errorkit.WithReason("mongo %s failed", operation),
		errorkit.WithWrapped(err),
	)
}

// WrapDecodeError normalizes decode failures across repositories.
func WrapDecodeError(err error, operation string) error {
	if err == nil {
		return nil
	}
	return errorkit.NewError(ERR_DB_MONGO_DECODE_FAILED).With(
		errorkit.WithReason("mongo %s decode failed", operation),
		errorkit.WithWrapped(err),
	)
}
