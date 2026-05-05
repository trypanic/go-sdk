package database

import "github.com/trypanic/go-sdk/errorkit"

// ==============================
// PostgreSQL Error Codes
// ==============================

const (
	// ERR_DB_POSTGRES_UNAVAILABLE indicates PostgreSQL service is not available
	// Context: Cannot connect to PostgreSQL server, all replicas down,
	// network partition, connection pool exhausted, server unreachable
	ERR_DB_POSTGRES_UNAVAILABLE errorkit.ErrorCode = "ERR_DB_POSTGRES_UNAVAILABLE"

	// ERR_DB_POSTGRES_TIMEOUT indicates PostgreSQL operation timed out
	// Context: Query execution exceeded timeout, slow query on large table,
	// write operation timeout, transaction timeout, lock wait timeout
	ERR_DB_POSTGRES_TIMEOUT errorkit.ErrorCode = "ERR_DB_POSTGRES_TIMEOUT"

	// ERR_DB_POSTGRES_AUTH_FAILED indicates PostgreSQL authentication failed
	// Context: Invalid database credentials, user doesn't have required privileges,
	// authentication method not supported, SSL/TLS certificate error, role doesn't exist
	ERR_DB_POSTGRES_AUTH_FAILED errorkit.ErrorCode = "ERR_DB_POSTGRES_AUTH_FAILED"

	// ERR_DB_POSTGRES_CONNECTION_FAILED indicates PostgreSQL connection failed
	// Context: Max connections reached, connection refused, server starting up,
	// connection string invalid, network issues, firewall blocking
	ERR_DB_POSTGRES_CONNECTION_FAILED errorkit.ErrorCode = "ERR_DB_POSTGRES_CONNECTION_FAILED"

	// ERR_DB_POSTGRES_DEADLOCK indicates PostgreSQL deadlock detected
	// Context: Deadlock detected during transaction, circular lock dependency,
	// concurrent transactions conflict, serialization failure
	ERR_DB_POSTGRES_DEADLOCK errorkit.ErrorCode = "ERR_DB_POSTGRES_DEADLOCK"

	// ERR_DB_POSTGRES_ERROR indicates a general PostgreSQL error
	// Context: PostgreSQL returned error, disk full, WAL corruption,
	// replication lag, vacuum failure, index corruption
	ERR_DB_POSTGRES_ERROR errorkit.ErrorCode = "ERR_DB_POSTGRES_ERROR"
)

// ErrorMetadata is the canonical metadata for all postgres-owned error codes.
var ErrorMetadata = []errorkit.Metadata{
	{
		Code:        ERR_DB_POSTGRES_UNAVAILABLE,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupDatabase,
		Category:    "database",
		Description: "PostgreSQL service unavailable",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_DB_POSTGRES_TIMEOUT,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupDatabase,
		Category:    "database",
		Description: "PostgreSQL operation timeout",
		HTTPStatus:  504,
		Retriable:   true,
	},
	{
		Code:        ERR_DB_POSTGRES_AUTH_FAILED,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupDatabase,
		Category:    "database",
		Description: "PostgreSQL authentication failed",
		HTTPStatus:  502,
		Retriable:   false,
	},
	{
		Code:        ERR_DB_POSTGRES_CONNECTION_FAILED,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupDatabase,
		Category:    "database",
		Description: "PostgreSQL connection failed",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_DB_POSTGRES_DEADLOCK,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupDatabase,
		Category:    "database",
		Description: "PostgreSQL deadlock detected",
		HTTPStatus:  409,
		Retriable:   true,
	},
	{
		Code:        ERR_DB_POSTGRES_ERROR,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupDatabase,
		Category:    "database",
		Description: "PostgreSQL error",
		HTTPStatus:  503,
		Retriable:   true,
	},
}

// RegisterErrors registers postgres-owned error metadata. When registry is nil
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
