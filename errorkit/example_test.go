package errorkit_test

import (
	"fmt"

	"github.com/trypanic/go-sdk/errorkit"
)

func ExampleHasCode() {
	notFound := errorkit.NewError(errorkit.ERR_CLIENT_NOT_FOUND).
		With(errorkit.WithReason("user %q not found", "42"))
	err := fmt.Errorf("load profile: %w", notFound)

	if appErr, ok := errorkit.As(err); ok {
		fmt.Println(appErr.Code(), appErr.Metadata.HTTPStatus)
	}
	fmt.Println(errorkit.HasCode(err, errorkit.ERR_CLIENT_NOT_FOUND))
	// Output:
	// ERR_CLIENT_NOT_FOUND 404
	// true
}

func ExampleAppError_Public() {
	internal := errorkit.NewError(errorkit.ERR_INTERNAL).
		With(errorkit.WithReason("dial tcp 10.0.0.7:5432: connection refused"))
	validation := errorkit.NewError(errorkit.ERR_VALIDATION_MISSING_FIELD).
		With(errorkit.WithReason("field %q is required", "email"))

	// Server-side reasons are replaced by the description; client-error
	// reasons describe the caller's input and are kept.
	fmt.Println(internal.Public().Message)
	fmt.Println(validation.Public().Message)
	// Output:
	// Internal server error
	// field "email" is required
}
