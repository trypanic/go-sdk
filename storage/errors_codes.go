package storage

import "github.com/trypanic/go-sdk/errorkit"

// ==============================
// Storage Error Codes
// ==============================

const (
	// ERR_STORAGE_UNAVAILABLE indicates storage service is not available
	ERR_STORAGE_UNAVAILABLE errorkit.ErrorCode = "ERR_STORAGE_UNAVAILABLE"

	// ERR_STORAGE_TIMEOUT indicates storage operation timed out
	ERR_STORAGE_TIMEOUT errorkit.ErrorCode = "ERR_STORAGE_TIMEOUT"

	// ERR_STORAGE_ACCESS_DENIED indicates access to storage was denied
	ERR_STORAGE_ACCESS_DENIED errorkit.ErrorCode = "ERR_STORAGE_ACCESS_DENIED"

	// ERR_STORAGE_INVALID_CREDENTIALS indicates storage credentials are wrong
	ERR_STORAGE_INVALID_CREDENTIALS errorkit.ErrorCode = "ERR_STORAGE_INVALID_CREDENTIALS"

	// ERR_STORAGE_ERROR indicates a general storage error
	ERR_STORAGE_ERROR errorkit.ErrorCode = "ERR_STORAGE_ERROR"
)

// ErrorMetadata is the canonical metadata for all storage-owned error codes.
var ErrorMetadata = []errorkit.Metadata{
	{
		Code:        ERR_STORAGE_UNAVAILABLE,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupStorage,
		Category:    "storage",
		Description: "Storage service unavailable",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_STORAGE_TIMEOUT,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupStorage,
		Category:    "storage",
		Description: "Storage operation timeout",
		HTTPStatus:  504,
		Retriable:   true,
	},
	{
		Code:        ERR_STORAGE_ACCESS_DENIED,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupStorage,
		Category:    "storage",
		Description: "Storage access denied",
		HTTPStatus:  502,
		Retriable:   false,
	},
	{
		Code:        ERR_STORAGE_INVALID_CREDENTIALS,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupStorage,
		Category:    "storage",
		Description: "Invalid storage credentials",
		HTTPStatus:  502,
		Retriable:   false,
	},
	{
		Code:        ERR_STORAGE_ERROR,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupStorage,
		Category:    "storage",
		Description: "Storage error",
		HTTPStatus:  503,
		Retriable:   true,
	},
}

// RegisterErrors registers storage-owned error metadata. When registry is nil
// it registers into the package-global registry. Calling more than once with
// identical metadata is a no-op.
func RegisterErrors(registry *errorkit.Registry) error {
	if registry == nil {
		return errorkit.RegisterMany(ErrorMetadata...)
	}
	return registry.RegisterMany(ErrorMetadata...)
}

func init() {
	errorkit.MustRegister(ErrorMetadata...)
}
