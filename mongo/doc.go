// Package mongodb exposes a small, driver-agnostic interface (ClientPort,
// Collection, Cursor and result interfaces) over the official MongoDB Go
// driver.
//
// Import it as github.com/trypanic/go-sdk/mongo; the package identifier is
// mongodb.
//
// New connects, pings the primary and returns a ClientPort that opens one
// span per collection operation; NewWithoutTracing and NewWithInstrumenter
// choose the tracing. Each option comes from the Config field, else the URI,
// else an SDK default.
//
// Errors are *errorkit.AppError values that wrap the driver error; see
// WrapOperationError and WrapDecodeError for the codes.
//
// A ClientPort is safe for concurrent use. Create one per process and call
// Close once at shutdown.
package mongodb
