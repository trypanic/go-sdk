package validators

import (
	"strings"
	"time"

	"github.com/trypanic/go-sdk/v2/errorkit"
)

// DateLayout is the date format (YYYY-MM-DD) used by IsValidDatePointer and
// ValidateDateRange.
const DateLayout = "2006-01-02"

// Error-code policy shared by every validator: a value of the wrong type is
// ERR_VALIDATION_INVALID_FORMAT, an absent value (nil, nil pointer, blank
// string) is ERR_VALIDATION_MISSING_FIELD, and a well-formed value that
// violates a constraint is ERR_VALIDATION_INCONSISTENT.

// IsValidDatePointer validates that value is a non-nil *string holding a date
// in DateLayout (YYYY-MM-DD) format.
func IsValidDatePointer(value any) error {
	if value == nil {
		return missingField("value is required")
	}
	datePtr, ok := value.(*string)
	if !ok {
		return invalidFormat("value must be a *string")
	}
	if datePtr == nil {
		return missingField("value is required")
	}

	if _, err := time.Parse(DateLayout, *datePtr); err != nil {
		return errorkit.NewWithoutStack(errorkit.ERR_VALIDATION_INVALID_FORMAT).With(
			errorkit.WithReason("must be in format YYYY-MM-DD"),
			errorkit.WithWrapped(err),
		)
	}

	return nil
}

// IsAlphanumericWithSpaces validates that value is a string that is not blank
// and contains only ASCII letters, ASCII digits and spaces.
func IsAlphanumericWithSpaces(value any) error {
	str, ok := value.(string)
	if !ok {
		return invalidFormat("value must be a string")
	}

	if strings.TrimSpace(str) == "" {
		return missingField("cannot be empty or only whitespace")
	}

	for _, char := range str {
		if !isAlphanumericOrSpace(char) {
			return invalidFormat("must contain only alphanumeric characters and spaces")
		}
	}

	return nil
}

func isAlphanumericOrSpace(r rune) bool {
	switch {
	case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z', '0' <= r && r <= '9', r == ' ':
		return true
	default:
		return false
	}
}

// IsWithinRange returns a validator that checks whether an int is within
// [lo, hi]. If lo > hi the bounds are a programming error and the validator
// always fails with ERR_SYSTEM_CONFIG_INVALID.
func IsWithinRange(lo, hi int) func(any) error {
	return func(value any) error {
		if lo > hi {
			return errorkit.NewError(errorkit.ERR_SYSTEM_CONFIG_INVALID).With(
				errorkit.WithReason("invalid range: lower bound %d is greater than upper bound %d", lo, hi),
			)
		}
		num, ok := value.(int)
		if !ok {
			return invalidFormat("value must be an int")
		}
		if num < lo || num > hi {
			return errorkit.NewWithoutStack(errorkit.ERR_VALIDATION_INCONSISTENT).With(
				errorkit.WithReason("must be between %d and %d", lo, hi),
			)
		}
		return nil
	}
}

// ValidateDateRange validates that from <= to using YYYY-MM-DD format.
// Equal dates are valid.
func ValidateDateRange(from, to string) error {
	fromDate, err := time.Parse(DateLayout, from)
	if err != nil {
		return errorkit.NewWithoutStack(errorkit.ERR_VALIDATION_INVALID_FORMAT).With(
			errorkit.WithReason("invalid from date format, expected YYYY-MM-DD"),
			errorkit.WithWrapped(err),
		)
	}

	toDate, err := time.Parse(DateLayout, to)
	if err != nil {
		return errorkit.NewWithoutStack(errorkit.ERR_VALIDATION_INVALID_FORMAT).With(
			errorkit.WithReason("invalid to date format, expected YYYY-MM-DD"),
			errorkit.WithWrapped(err),
		)
	}

	if fromDate.After(toDate) {
		return errorkit.NewWithoutStack(errorkit.ERR_VALIDATION_INCONSISTENT).With(
			errorkit.WithReason("from date must not be after to date"),
		)
	}

	return nil
}

func invalidFormat(reason string) error {
	return errorkit.NewWithoutStack(errorkit.ERR_VALIDATION_INVALID_FORMAT).With(errorkit.WithReason("%s", reason))
}

func missingField(reason string) error {
	return errorkit.NewWithoutStack(errorkit.ERR_VALIDATION_MISSING_FIELD).With(errorkit.WithReason("%s", reason))
}
