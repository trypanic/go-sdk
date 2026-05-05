package mongodb

import (
	"context"
	"errors"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestParseObjectID_InvalidFormat_ReturnsAppError(t *testing.T) {
	_, err := ParseObjectID("invalid")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var appErr *errorkit.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *errorkit.AppError, got %T", err)
	}
	if appErr.Code() != errorkit.ERR_VALIDATION_INVALID_FORMAT {
		t.Fatalf("expected code %s, got %s", errorkit.ERR_VALIDATION_INVALID_FORMAT, appErr.Code())
	}
}

func TestWrapOperationError_Mappings(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode errorkit.ErrorCode
	}{
		{
			name:     "deadline exceeded",
			err:      context.DeadlineExceeded,
			wantCode: ERR_DB_MONGO_TIMEOUT,
		},
		{
			name:     "not found",
			err:      mongo.ErrNoDocuments,
			wantCode: ERR_DB_MONGO_NOT_FOUND,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WrapOperationError(tt.err, "find")

			var appErr *errorkit.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("expected *errorkit.AppError, got %T", err)
			}
			if appErr.Code() != tt.wantCode {
				t.Fatalf("expected code %s, got %s", tt.wantCode, appErr.Code())
			}
		})
	}
}
