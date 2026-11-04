// Package mongo exposes a small, driver-agnostic interface (ClientPort,
// Collection, Cursor and result interfaces) over the official MongoDB Go
// driver.
//
// Import it as github.com/trypanic/go-sdk/mongo; the package identifier is
// mongo.
//
// New connects, pings the primary and returns a concrete Client that opens one
// client span per collection operation; NewWithoutTracing and NewWithInstrumenter
// choose the tracing. Each option comes from the Config field, else the URI,
// else an SDK default.
//
// Errors are *errorkit.AppError values that wrap the driver error; see
// WrapOperationError and WrapDecodeError for the codes.
//
// A ClientPort is safe for concurrent use. Create one per process and call
// Close once at shutdown.
package mongo
