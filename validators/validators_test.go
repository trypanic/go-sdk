package validators

import (
	"errors"
	"strings"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestValidateDateRange_InvalidDates_ReturnsAppError(t *testing.T) {
	err := ValidateDateRange("bad", "2024-01-01")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	appErr, ok := errors.AsType[*errorkit.AppError](err)
	if !ok {
		t.Fatalf("expected *errorkit.AppError, got %T", err)
	}
	if appErr.Code() != errorkit.ERR_VALIDATION_INVALID_FORMAT {
		t.Fatalf("expected code %s, got %s", errorkit.ERR_VALIDATION_INVALID_FORMAT, appErr.Code())
	}
}

func requireCode(t *testing.T, err error, want errorkit.ErrorCode) *errorkit.AppError {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	appErr, ok := errors.AsType[*errorkit.AppError](err)
	if !ok {
		t.Fatalf("error type = %T, want *errorkit.AppError", err)
	}
	if appErr.Code() != want {
		t.Fatalf("code = %s, want %s (reason %q)", appErr.Code(), want, appErr.Reason)
	}
	return appErr
}

// A wrong type is a format problem in every validator; only an absent
// value is a missing field.
func TestValidatorsUseConsistentCodes(t *testing.T) {
	t.Parallel()

	requireCode(t, IsValidDatePointer("2024-01-01"), errorkit.ERR_VALIDATION_INVALID_FORMAT)
	requireCode(t, IsValidDatePointer(42), errorkit.ERR_VALIDATION_INVALID_FORMAT)
	requireCode(t, IsValidDatePointer((*string)(nil)), errorkit.ERR_VALIDATION_MISSING_FIELD)
	requireCode(t, IsValidDatePointer(nil), errorkit.ERR_VALIDATION_MISSING_FIELD)
	requireCode(t, IsAlphanumericWithSpaces(42), errorkit.ERR_VALIDATION_INVALID_FORMAT)
	requireCode(t, IsWithinRange(1, 5)(int64(3)), errorkit.ERR_VALIDATION_INVALID_FORMAT)
}

func TestIsWithinRangeRejectsInvertedBounds(t *testing.T) {
	t.Parallel()

	appErr := requireCode(t, IsWithinRange(10, 1)(5), errorkit.ERR_SYSTEM_CONFIG_INVALID)
	if !strings.Contains(appErr.Reason, "10") || !strings.Contains(appErr.Reason, "1") {
		t.Fatalf("reason %q should name the bounds", appErr.Reason)
	}
}

// "must be within range -5--1" was ambiguous for negative bounds.
func TestIsWithinRangeMessageIsUnambiguous(t *testing.T) {
	t.Parallel()

	appErr := requireCode(t, IsWithinRange(-5, -1)(0), errorkit.ERR_VALIDATION_INCONSISTENT)
	if want := "must be between -5 and -1"; appErr.Reason != want {
		t.Fatalf("reason = %q, want %q", appErr.Reason, want)
	}
}

// Equal dates are accepted, so "before" was inaccurate, and the message
// used a consumer's field names.
func TestValidateDateRangeMessage(t *testing.T) {
	t.Parallel()

	appErr := requireCode(t, ValidateDateRange("2024-02-02", "2024-02-01"), errorkit.ERR_VALIDATION_INCONSISTENT)
	if want := "from date must not be after to date"; appErr.Reason != want {
		t.Fatalf("reason = %q, want %q", appErr.Reason, want)
	}
}
