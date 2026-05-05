package validators

import (
	"errors"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestValidateDateRange_InvalidDates_ReturnsAppError(t *testing.T) {
	err := ValidateDateRange("bad", "2024-01-01")
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
