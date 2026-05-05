package validators

import (
	"strings"
	"time"

	"github.com/trypanic/go-sdk/errorkit"
)

const DateLayout = "2006-01-02"

// IsValidDatePointer validates date format for pointer fields
func IsValidDatePointer(value any) error {
	datePtr, ok := value.(*string)
	if !ok || datePtr == nil {
		return errorkit.NewError(errorkit.ERR_VALIDATION_MISSING_FIELD).With(
			errorkit.WithReason("value must be a string"),
		)
	}

	if _, err := time.Parse(DateLayout, *datePtr); err != nil {
		return errorkit.NewError(errorkit.ERR_VALIDATION_INVALID_FORMAT).With(
			errorkit.WithReason("must be in format YYYY-MM-DD"),
			errorkit.WithWrapped(err),
		)
	}

	return nil
}

// IsAlphanumericWithSpaces validates alphanumeric characters and spaces
func IsAlphanumericWithSpaces(value any) error {
	str, ok := value.(string)
	if !ok {
		return errorkit.NewError(errorkit.ERR_VALIDATION_INVALID_FORMAT).With(
			errorkit.WithReason("value must be a string"),
		)
	}

	if strings.TrimSpace(str) == "" {
		return errorkit.NewError(errorkit.ERR_VALIDATION_MISSING_FIELD).With(
			errorkit.WithReason("cannot be empty or only whitespace"),
		)
	}

	for _, char := range str {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == ' ') {
			return errorkit.NewError(errorkit.ERR_VALIDATION_INVALID_FORMAT).With(
				errorkit.WithReason("must contain only alphanumeric characters and spaces"),
			)
		}
	}

	return nil
}

// IsWithinRange returns a validator that checks whether an integer is within [min, max].
func IsWithinRange(min, max int) func(any) error {
	return func(value any) error {
		num, ok := value.(int)
		if !ok {
			return errorkit.NewError(errorkit.ERR_VALIDATION_INVALID_FORMAT).With(
				errorkit.WithReason("value must be an integer"),
			)
		}
		if num < min || num > max {
			return errorkit.NewError(errorkit.ERR_VALIDATION_INCONSISTENT).With(
				errorkit.WithReason("must be within range %d-%d", min, max),
			)
		}
		return nil
	}
}

// ValidateDateRange validates that from <= to using YYYY-MM-DD format.
func ValidateDateRange(from, to string) error {
	fromDate, err := time.Parse(DateLayout, from)
	if err != nil {
		return errorkit.NewError(errorkit.ERR_VALIDATION_INVALID_FORMAT).With(
			errorkit.WithReason("invalid from date format, expected YYYY-MM-DD"),
			errorkit.WithWrapped(err),
		)
	}

	toDate, err := time.Parse(DateLayout, to)
	if err != nil {
		return errorkit.NewError(errorkit.ERR_VALIDATION_INVALID_FORMAT).With(
			errorkit.WithReason("invalid to date format, expected YYYY-MM-DD"),
			errorkit.WithWrapped(err),
		)
	}

	if fromDate.After(toDate) {
		return errorkit.NewError(errorkit.ERR_VALIDATION_INCONSISTENT).With(
			errorkit.WithReason("filter_from must be before filter_to"),
		)
	}

	return nil
}
