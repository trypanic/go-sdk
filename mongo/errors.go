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

// WrapOperationError maps raw Mongo driver errors to standardized errorkit
// codes and keeps the driver error wrapped:
//
//	mongo.ErrNoDocuments                → ERR_DB_MONGO_NOT_FOUND (404, not retriable)
//	context.Canceled                    → ERR_DB_MONGO_CANCELED (499, not retriable)
//	mongo.IsTimeout (deadline, network
//	timeout, MaxTimeMSExpired, …)       → ERR_DB_MONGO_TIMEOUT (retriable)
//	mongo.IsDuplicateKeyError           → errorkit.ERR_VALIDATION_DUPLICATE
//	mongo.InvalidArgumentError
//	(e.g. mongo.ErrNilDocument)         → errorkit.ERR_VALIDATION
//	mongo.ErrClientDisconnected         → errorkit.ERR_SYSTEM_UNEXPECTED (not retriable)
//	mongo.IsNetworkError                → ERR_DB_MONGO_UNAVAILABLE (retriable)
//	anything else                       → ERR_DB_MONGO_ERROR (retriable)
func WrapOperationError(err error, operation string) error {
	if err == nil {
		return nil
	}

	code, reason := operationErrorCode(err)
	return errorkit.NewError(code).With(
		errorkit.WithReason("mongo %s %s", operation, reason),
		errorkit.WithWrapped(err),
	)
}

func operationErrorCode(err error) (errorkit.ErrorCode, string) {
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return ERR_DB_MONGO_NOT_FOUND, "returned no documents"
	case errors.Is(err, context.Canceled):
		return ERR_DB_MONGO_CANCELED, "was cancelled"
	case mongo.IsTimeout(err):
		return ERR_DB_MONGO_TIMEOUT, "timed out"
	case mongo.IsDuplicateKeyError(err):
		return errorkit.ERR_VALIDATION_DUPLICATE, "violated unique constraint"
	case isInvalidArgument(err):
		return errorkit.ERR_VALIDATION, "rejected an invalid argument"
	case errors.Is(err, mongo.ErrClientDisconnected):
		return errorkit.ERR_SYSTEM_UNEXPECTED, "used a disconnected client"
	case mongo.IsNetworkError(err):
		return ERR_DB_MONGO_UNAVAILABLE, "failed on the network"
	default:
		return ERR_DB_MONGO_ERROR, "failed"
	}
}

func isInvalidArgument(err error) bool {
	_, ok := errors.AsType[mongo.InvalidArgumentError](err)
	return ok
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
