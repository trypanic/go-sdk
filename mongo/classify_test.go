package mongodb

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "i/o timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

// TestWrapOperationErrorMatrix covers C4-010 (3): cancellations, closed
// clients and invalid arguments were retriable 503s, and driver timeouts or
// network errors that do not wrap context.DeadlineExceeded were not
// detected.
func TestWrapOperationErrorMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		err           error
		wantCode      errorkit.ErrorCode
		wantRetriable bool
	}{
		{"deadline exceeded", fmt.Errorf("op: %w", context.DeadlineExceeded), ERR_DB_MONGO_TIMEOUT, true},
		{"network timeout label", mongo.CommandError{Name: "x", Labels: []string{"NetworkTimeoutError"}}, ERR_DB_MONGO_TIMEOUT, true},
		{"net.Error timeout", fmt.Errorf("read: %w", timeoutNetError{}), ERR_DB_MONGO_TIMEOUT, true},
		{"max time expired", mongo.CommandError{Code: 50, Name: "MaxTimeMSExpired"}, ERR_DB_MONGO_TIMEOUT, true},
		{"canceled", fmt.Errorf("op: %w", context.Canceled), ERR_DB_MONGO_CANCELED, false},
		{"network error", mongo.CommandError{Name: "x", Labels: []string{"NetworkError"}}, ERR_DB_MONGO_UNAVAILABLE, true},
		{"client disconnected", mongo.ErrClientDisconnected, errorkit.ERR_SYSTEM_UNEXPECTED, false},
		{"nil document", mongo.ErrNilDocument, errorkit.ERR_VALIDATION, false},
		{"not found", mongo.ErrNoDocuments, ERR_DB_MONGO_NOT_FOUND, false},
		{"duplicate key", mongo.WriteException{WriteErrors: []mongo.WriteError{{Code: 11000, Message: "E11000"}}}, errorkit.ERR_VALIDATION_DUPLICATE, false},
		{"generic server error", mongo.CommandError{Code: 2, Name: "BadValue"}, ERR_DB_MONGO_ERROR, true},
		{"unknown", errors.New("boom"), ERR_DB_MONGO_ERROR, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			appErr := requireCode(t, WrapOperationError(tt.err, "find"), tt.wantCode)
			if appErr.Metadata.Retriable != tt.wantRetriable {
				t.Fatalf("retriable = %v, want %v", appErr.Metadata.Retriable, tt.wantRetriable)
			}
			// Driver error structs with slices are not comparable, so compare
			// the wrapped cause by its text rather than with errors.Is.
			if cause := appErr.Unwrap(); cause == nil || cause.Error() != tt.err.Error() {
				t.Fatalf("cause not wrapped: got %v, want %v", cause, tt.err)
			}
		})
	}

	if WrapOperationError(nil, "find") != nil {
		t.Fatal("WrapOperationError(nil) must be nil")
	}
}

// TestNewConfigErrorIsConfigInvalid covers C4-010 (4): an unparsable URI is a
// configuration error, not a retriable ERR_DB_MONGO_UNAVAILABLE. The URI is
// rejected before any I/O.
func TestNewConfigErrorIsConfigInvalid(t *testing.T) {
	t.Parallel()

	_, err := NewWithoutTracing(context.Background(), Config{URI: "mongodb://localhost:notaport"})
	appErr := requireCode(t, err, errorkit.ERR_SYSTEM_CONFIG_INVALID)
	if appErr.Metadata.Retriable {
		t.Fatal("configuration errors must not be retriable")
	}
}

func TestWrapDecodeError(t *testing.T) {
	t.Parallel()

	if WrapDecodeError(nil, "find") != nil {
		t.Fatal("WrapDecodeError(nil) must be nil")
	}
	cause := errors.New("cannot decode 32-bit integer into a string type")
	appErr := requireCode(t, WrapDecodeError(cause, "find"), ERR_DB_MONGO_DECODE_FAILED)
	if appErr.Metadata.Retriable || appErr.Metadata.HTTPStatus != 500 {
		t.Fatalf("decode failure: retriable=%v http=%d, want false/500", appErr.Metadata.Retriable, appErr.Metadata.HTTPStatus)
	}
	if !errors.Is(appErr, cause) {
		t.Fatalf("cause not wrapped: %v", appErr)
	}
}
