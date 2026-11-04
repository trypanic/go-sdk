// Package httprequest sends HTTP requests with retries and maps every
// failure to an *errorkit.AppError.
//
// Requester.Do builds the request, retries retriable failures that are
// safe to resend (idempotent methods, an Idempotency-Key header, or a
// connection that was never made) with jittered exponential backoff that
// honors Retry-After, and decodes the response into out by its type
// (string, numbers, bool, []byte, or JSON). Bodies never reach error
// payloads or logs unless WithBodyCapture or WithAuditLog is set, and even
// then they pass through a BodyRedactor ([REDACTED] by default); URLs are
// sanitized everywhere. Response bodies are capped (WithMaxResponseBytes).
// Do never logs the errors it returns.
//
// Constructors follow the SDK triplet: New (global OTel tracer),
// NewWithoutTracing and NewWithInstrumenter; NewWithOptions takes
// functional options. A requester is safe for concurrent use; build one
// per upstream and share it.
package httprequest
