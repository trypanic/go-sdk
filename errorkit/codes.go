package errorkit

// ==============================
// Error Code Definitions
// ==============================
//
// Naming Convention:
// - ERR_<GROUP>_<SPECIFIC>
// - Use descriptive names that explain the error
// - Group related errors with common prefix
//
// Guidelines:
// - Use specific codes when business logic differs
// - Use generic codes + payload for similar handling
// - Internal errors: our application's fault
// - External errors: third-party service's fault
//

// ==============================
// General Errors
// ==============================

const (
	// ERR_UNKNOWN represents an uncategorized error
	// Context: Use when error doesn't fit any other category, panic recovery, unhandled edge cases
	ERR_UNKNOWN ErrorCode = "ERR_UNKNOWN"

	// ERR_INTERNAL represents a generic internal server error
	// Context: Use for internal errors without specific category, configuration issues, unexpected nil pointers
	ERR_INTERNAL ErrorCode = "ERR_INTERNAL"
)

// ==============================
// Validation & Data Errors
// ==============================

const (
	// ERR_VALIDATION represents a generic validation failure
	// Context: Use when multiple validation errors occur simultaneously or for unspecified validation issues
	ERR_VALIDATION ErrorCode = "ERR_VALIDATION"

	// ERR_VALIDATION_INVALID_FORMAT indicates a field has wrong format
	// Context: Email format, phone format, date format, URL format, JSON structure, regex pattern mismatch
	ERR_VALIDATION_INVALID_FORMAT ErrorCode = "ERR_VALIDATION_INVALID_FORMAT"

	// ERR_VALIDATION_MISSING_FIELD indicates a required field is missing
	// Context: Required HTTP headers, required query parameters, required JSON fields, missing function arguments
	ERR_VALIDATION_MISSING_FIELD ErrorCode = "ERR_VALIDATION_MISSING_FIELD"

	// ERR_VALIDATION_INCONSISTENT indicates fields have conflicting values
	// Context: Start date > end date, password != confirm_password, conflicting boolean flags,
	// min_value > max_value, incompatible option combinations
	ERR_VALIDATION_INCONSISTENT ErrorCode = "ERR_VALIDATION_INCONSISTENT"

	// ERR_VALIDATION_BUSINESS_RULE indicates a business rule violation
	// Context: Cannot delete resource with dependencies, insufficient balance for transaction,
	// operation not allowed in current state, workflow step violations, quota exceeded
	ERR_VALIDATION_BUSINESS_RULE ErrorCode = "ERR_VALIDATION_BUSINESS_RULE"

	// ERR_VALIDATION_DUPLICATE indicates a resource already exists
	// Context: Unique constraint violation in database, duplicate email/username registration,
	// duplicate file upload, resource name already taken, idempotency key collision
	ERR_VALIDATION_DUPLICATE ErrorCode = "ERR_VALIDATION_DUPLICATE"
)

// ==============================
// Client Errors
// ==============================

const (
	// ERR_CLIENT_BAD_REQUEST indicates client sent malformed request
	// Context: Invalid JSON syntax, wrong content-type header, malformed multipart form data,
	// invalid base64 encoding, corrupted binary data, unparseable request body
	ERR_CLIENT_BAD_REQUEST ErrorCode = "ERR_CLIENT_BAD_REQUEST"

	ERR_CLIENT_NOT_FOUND ErrorCode = "ERR_CLIENT_NOT_FOUND"

	// ERR_CLIENT_RATE_LIMIT indicates client exceeded rate limit
	// Context: Too many requests per minute/hour, API quota exceeded, burst limit reached,
	// concurrent request limit exceeded, throttling applied
	ERR_CLIENT_RATE_LIMIT ErrorCode = "ERR_CLIENT_RATE_LIMIT"
)

// ==============================
// System Internal Errors
// ==============================

const (
	// ERR_SYSTEM_UNEXPECTED indicates an unexpected internal error
	// Context: Panic recovered, nil pointer dereference, array index out of bounds,
	// type assertion failure, unhandled switch case, impossible state reached
	ERR_SYSTEM_UNEXPECTED ErrorCode = "ERR_SYSTEM_UNEXPECTED"

	// ERR_SYSTEM_CONFIG_INVALID indicates invalid system configuration
	// Context: Missing environment variables, invalid config file format, wrong configuration values,
	// misconfigured connection strings, invalid feature flags, corrupt configuration data
	ERR_SYSTEM_CONFIG_INVALID ErrorCode = "ERR_SYSTEM_CONFIG_INVALID"

	// ERR_SYSTEM_TIMEOUT_INTERNAL indicates internal operation timeout
	// Context: Long-running query exceeded limit, background job timeout, batch process timeout,
	// internal task deadline exceeded, worker pool timeout
	ERR_SYSTEM_TIMEOUT_INTERNAL ErrorCode = "ERR_SYSTEM_TIMEOUT_INTERNAL"

	// ERR_SYSTEM_CONCURRENCY indicates concurrency conflict
	// Context: Optimistic locking failure, version mismatch, race condition detected,
	// concurrent modification conflict, distributed lock acquisition failed, CAS operation failed
	ERR_SYSTEM_CONCURRENCY ErrorCode = "ERR_SYSTEM_CONCURRENCY"
)
