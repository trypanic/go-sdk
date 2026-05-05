package mongodb

import "github.com/trypanic/go-sdk/errorkit"

// ==============================
// MongoDB Error Codes
// ==============================

const (
	// ERR_DB_MONGO_UNAVAILABLE indicates MongoDB service is not available
	// Context: Cannot connect to MongoDB cluster, all replica set members down,
	// network partition, MongoDB authentication timeout, cluster unreachable
	ERR_DB_MONGO_UNAVAILABLE errorkit.ErrorCode = "ERR_DB_MONGO_UNAVAILABLE"

	// ERR_DB_MONGO_TIMEOUT indicates MongoDB operation timed out
	// Context: Query execution exceeded timeout, slow query on large collection,
	// write operation timeout, index building timeout, cursor timeout
	ERR_DB_MONGO_TIMEOUT errorkit.ErrorCode = "ERR_DB_MONGO_TIMEOUT"

	// ERR_DB_MONGO_AUTH_FAILED indicates MongoDB authentication failed
	// Context: Invalid database credentials, user doesn't have required privileges,
	// authentication mechanism not supported, SCRAM authentication failed, SSL/TLS certificate error
	ERR_DB_MONGO_AUTH_FAILED errorkit.ErrorCode = "ERR_DB_MONGO_AUTH_FAILED"

	// ERR_DB_MONGO_ERROR indicates a general MongoDB error
	// Context: MongoDB returned 5xx error, replica set election in progress,
	// database locked, storage engine error, out of disk space
	ERR_DB_MONGO_ERROR errorkit.ErrorCode = "ERR_DB_MONGO_ERROR"

	// ERR_DB_MONGO_NOT_FOUND indicates MongoDB document not found
	// Context: Document not found in collection, query returned no results, missing document
	ERR_DB_MONGO_NOT_FOUND errorkit.ErrorCode = "ERR_DB_MONGO_NOT_FOUND"

	// ERR_DB_MONGO_DECODE_FAILED indicates MongoDB document decode failed
	// Context: Document not found in collection, query returned no results, missing document
	ERR_DB_MONGO_DECODE_FAILED errorkit.ErrorCode = "ERR_DB_MONGO_DECODE_FAILED"
)

// ErrorMetadata is the canonical metadata for all mongo-owned error codes.
var ErrorMetadata = []errorkit.Metadata{
	{
		Code:        ERR_DB_MONGO_UNAVAILABLE,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupDatabase,
		Category:    "database",
		Description: "MongoDB service unavailable",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_DB_MONGO_TIMEOUT,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupDatabase,
		Category:    "database",
		Description: "MongoDB operation timeout",
		HTTPStatus:  504,
		Retriable:   true,
	},
	{
		Code:        ERR_DB_MONGO_AUTH_FAILED,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupDatabase,
		Category:    "database",
		Description: "MongoDB authentication failed",
		HTTPStatus:  502,
		Retriable:   false,
	},
	{
		Code:        ERR_DB_MONGO_ERROR,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupDatabase,
		Category:    "database",
		Description: "MongoDB error",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_DB_MONGO_NOT_FOUND,
		Type:        errorkit.ErrorTypeInternal,
		Group:       errorkit.GroupDatabase,
		Category:    "database",
		Description: "MongoDB document not found",
		HTTPStatus:  404,
		Retriable:   false,
	},
	{
		Code:        ERR_DB_MONGO_DECODE_FAILED,
		Type:        errorkit.ErrorTypeInternal,
		Group:       errorkit.GroupDatabase,
		Category:    "database",
		Description: "MongoDB document decode failed",
		HTTPStatus:  500,
		Retriable:   false,
	},
}

// RegisterErrors registers mongo-owned error metadata. When registry is nil it
// registers into the package-global registry. Calling more than once with
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
