// Package errorkit provides structured application errors.
//
// An *AppError carries an ErrorCode, metadata from a registry (type, group,
// category, description, HTTP status, retriability), an optional reason,
// payload and wrapped cause, a unique ULID, an optional trace ID and the
// stack captured at creation:
//
//	err := errorkit.NewError(errorkit.ERR_VALIDATION_MISSING_FIELD).
//		With(errorkit.WithReason("field %q is required", "email"))
//
// # Inspecting errors
//
// Use As and HasCode to find an *AppError or a code anywhere in an error
// chain; a direct type assertion misses errors wrapped with %w.
//
// # Output
//
// PrettyJSON, Pretty and WriteTo include the stack trace, payload and
// wrapped error text: use them for logs only. Use PublicJSON (or Public)
// for HTTP and RPC responses; it exposes only the code, a client-safe
// message, the error ID and the trace ID.
//
// # Registry
//
// The package-global registry holds the built-in codes; register your own
// with MustRegister (in init) or RegisterMany. Registry and Factory give an
// isolated registry and configuration for code that must not touch package
// state.
//
// # Concurrency
//
// Registries, the configuration setters (SetMaxStackDepth, SetIDGenerator,
// DisableColors, EnableColors) and NewError are safe for concurrent use.
// An *AppError is not: With modifies it in place, so build it in one
// goroutine and share it only when it is complete.
package errorkit
