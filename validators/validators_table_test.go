package validators

import (
	"testing"

	"github.com/trypanic/go-sdk/v2/errorkit"
)

func ptr(s string) *string { return &s }

// checkCode asserts err is nil when want is empty, and otherwise an
// *errorkit.AppError with code want.
func checkCode(t *testing.T, err error, want errorkit.ErrorCode) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("error = %v, want nil", err)
		}
		return
	}
	_ = requireCode(t, err, want)
}

func TestIsValidDatePointerTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value any
		want  errorkit.ErrorCode
	}{
		{"valid date", ptr("2024-01-31"), ""},
		{"leap day in leap year", ptr("2024-02-29"), ""},
		{"leap day in common year", ptr("2023-02-29"), errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"month 13", ptr("2024-13-01"), errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"single-digit month and day", ptr("2024-1-5"), errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"leading space", ptr(" 2024-01-05"), errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"trailing newline", ptr("2024-01-05\n"), errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"full-width digits", ptr("２０２４-01-05"), errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"datetime", ptr("2024-01-05T10:00:00Z"), errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"empty string", ptr(""), errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"nil interface", nil, errorkit.ERR_VALIDATION_MISSING_FIELD},
		{"nil pointer", (*string)(nil), errorkit.ERR_VALIDATION_MISSING_FIELD},
		{"plain string", "2024-01-05", errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"int", 20240105, errorkit.ERR_VALIDATION_INVALID_FORMAT},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			checkCode(t, IsValidDatePointer(tc.value), tc.want)
		})
	}
}

func TestIsAlphanumericWithSpacesTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value any
		want  errorkit.ErrorCode
	}{
		{"letters digits spaces", "hello World 123", ""},
		{"surrounding spaces", "  a  ", ""},
		{"single char", "Z", ""},
		{"empty", "", errorkit.ERR_VALIDATION_MISSING_FIELD},
		{"only spaces", "   ", errorkit.ERR_VALIDATION_MISSING_FIELD},
		{"only tab and newline", "\t\n", errorkit.ERR_VALIDATION_MISSING_FIELD},
		{"dash", "hello-world", errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"underscore", "a_b", errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"accented letter", "José", errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"tab inside", "a\tb", errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"newline inside", "a\nb", errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"no-break space", "a b", errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"string pointer", ptr("abc"), errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"nil", nil, errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"int", 5, errorkit.ERR_VALIDATION_INVALID_FORMAT},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			checkCode(t, IsAlphanumericWithSpaces(tc.value), tc.want)
		})
	}
}

func TestIsWithinRangeTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		lo, hi int
		value  any
		want   errorkit.ErrorCode
	}{
		{"inside", 1, 10, 5, ""},
		{"lower bound inclusive", 1, 10, 1, ""},
		{"upper bound inclusive", 1, 10, 10, ""},
		{"single-value range", 7, 7, 7, ""},
		{"negative range", -5, -1, -3, ""},
		{"below", 1, 10, 0, errorkit.ERR_VALIDATION_INCONSISTENT},
		{"above", 1, 10, 11, errorkit.ERR_VALIDATION_INCONSISTENT},
		{"inverted bounds", 10, 1, 5, errorkit.ERR_SYSTEM_CONFIG_INVALID},
		{"int64", 1, 10, int64(5), errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"int32", 1, 10, int32(5), errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"uint", 1, 10, uint(5), errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"float", 1, 10, 5.0, errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"string", 1, 10, "5", errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"nil", 1, 10, nil, errorkit.ERR_VALIDATION_INVALID_FORMAT},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			checkCode(t, IsWithinRange(tc.lo, tc.hi)(tc.value), tc.want)
		})
	}
}

func TestValidateDateRangeTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		from, to string
		want     errorkit.ErrorCode
	}{
		{"ordered", "2024-01-01", "2024-12-31", ""},
		{"equal dates", "2024-05-05", "2024-05-05", ""},
		{"across years", "2023-12-31", "2024-01-01", ""},
		{"from after to", "2024-02-02", "2024-02-01", errorkit.ERR_VALIDATION_INCONSISTENT},
		{"bad from", "bad", "2024-01-01", errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"bad to", "2024-01-01", "2024-02-30", errorkit.ERR_VALIDATION_INVALID_FORMAT},
		{"empty from", "", "2024-01-01", errorkit.ERR_VALIDATION_INVALID_FORMAT},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			checkCode(t, ValidateDateRange(tc.from, tc.to), tc.want)
		})
	}
}
