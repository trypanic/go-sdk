package validators_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/validators"
)

func TestValidationFailuresDoNotCaptureStacks(t *testing.T) {
	for _, err := range []error{
		validators.IsAlphanumericWithSpaces("bad!"), validators.IsValidDatePointer(nil),
		validators.IsValidDatePointer(new("bad")), validators.IsWithinRange(1, 3)(4),
		validators.ValidateDateRange("2026-02-01", "2026-01-01"),
	} {
		app, ok := errorkit.As(err)
		require.True(t, ok)
		require.Empty(t, app.Trace)
	}
	config, ok := errorkit.As(validators.IsWithinRange(3, 1)(2))
	require.True(t, ok)
	require.NotEmpty(t, config.Trace)
}
