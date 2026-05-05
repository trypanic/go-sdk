package httpclient

import "github.com/trypanic/go-sdk/errorkit"

// ==============================
// External Service Error Codes (Generic)
// ==============================
//
// Used for HTTP-based external interactor or any third-party API. These errors
// indicate the fault is with the external service, not our application.

const (
	// ERR_EXTERNAL_SERVICE_UNAVAILABLE indicates external service is down or unreachable
	// Context: HTTP 503 response, service maintenance mode, load balancer returning errors,
	// circuit breaker open, health check failed, gRPC UNAVAILABLE status (14)
	ERR_EXTERNAL_SERVICE_UNAVAILABLE errorkit.ErrorCode = "ERR_EXTERNAL_SERVICE_UNAVAILABLE"

	// ERR_EXTERNAL_SERVICE_TIMEOUT indicates external service didn't respond in time
	// Context: HTTP 504 response, request deadline exceeded, read timeout from external API,
	// gRPC DEADLINE_EXCEEDED status (4), slow third-party response
	ERR_EXTERNAL_SERVICE_TIMEOUT errorkit.ErrorCode = "ERR_EXTERNAL_SERVICE_TIMEOUT"

	// ERR_EXTERNAL_SERVICE_ERROR indicates external service returned server error
	// Context: HTTP 500/502 response, internal error from third-party API,
	// gRPC INTERNAL status (13), external service degraded, unhandled exception in external service
	ERR_EXTERNAL_SERVICE_ERROR errorkit.ErrorCode = "ERR_EXTERNAL_SERVICE_ERROR"

	// ERR_EXTERNAL_INVALID_RESPONSE indicates external service response is invalid or unexpected
	// Context: HTTP 4xx responses (400, 401, 403, 404, 422), malformed JSON response,
	// unexpected response schema, missing required fields in response, response validation failed,
	// gRPC INVALID_ARGUMENT status (3), gRPC ABORTED status (10), protocol violation
	ERR_EXTERNAL_INVALID_RESPONSE errorkit.ErrorCode = "ERR_EXTERNAL_INVALID_RESPONSE"
)

// ErrorMetadata is the canonical metadata for httpclient-owned error codes.
var ErrorMetadata = []errorkit.Metadata{
	{
		Code:        ERR_EXTERNAL_SERVICE_UNAVAILABLE,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupSystem,
		Category:    "external",
		Description: "External service unavailable",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_EXTERNAL_SERVICE_TIMEOUT,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupSystem,
		Category:    "external",
		Description: "External service timeout",
		HTTPStatus:  504,
		Retriable:   true,
	},
	{
		Code:        ERR_EXTERNAL_SERVICE_ERROR,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupSystem,
		Category:    "external",
		Description: "External service error",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_EXTERNAL_INVALID_RESPONSE,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupSystem,
		Category:    "external",
		Description: "Invalid response from external service",
		HTTPStatus:  502,
		Retriable:   false,
	},
}

// RegisterErrors registers httpclient-owned error metadata. When registry is
// nil it registers into the package-global registry. Calling more than once
// with identical metadata is a no-op.
func RegisterErrors(registry *errorkit.Registry) error {
	if registry == nil {
		return errorkit.RegisterMany(ErrorMetadata...)
	}
	return registry.RegisterMany(ErrorMetadata...)
}

func init() {
	errorkit.MustRegister(ErrorMetadata...)
}
