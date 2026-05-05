package httprequest

import "github.com/trypanic/go-sdk/errorkit"

// ==============================
// Network Error Codes
// ==============================

const (
	// ERR_NETWORK_ERROR indicates a network communication error
	// Context: Connection refused, DNS lookup failed, network unreachable,
	// socket closed unexpectedly, SSL/TLS handshake failed, proxy error
	ERR_NETWORK_ERROR errorkit.ErrorCode = "ERR_NETWORK_ERROR"

	// ERR_NETWORK_TIMEOUT indicates network operation timed out
	// Context: Connection timeout, read timeout, write timeout,
	// idle timeout, dial timeout, context deadline exceeded during network call
	ERR_NETWORK_TIMEOUT errorkit.ErrorCode = "ERR_NETWORK_TIMEOUT"
)

// ==============================
// Authentication & Authorization Error Codes
// ==============================

const (
	// ERR_AUTH_UNAUTHENTICATED indicates user is not authenticated
	// Context: Missing Authorization header, no session cookie, API key not provided,
	// OAuth token missing, bearer token absent
	ERR_AUTH_UNAUTHENTICATED errorkit.ErrorCode = "ERR_AUTH_UNAUTHENTICATED"

	// ERR_AUTH_UNAUTHORIZED indicates user lacks permission
	// Context: User authenticated but missing required role, insufficient permissions for action,
	// resource ownership violation, scope limitations, RBAC policy denial
	ERR_AUTH_UNAUTHORIZED errorkit.ErrorCode = "ERR_AUTH_UNAUTHORIZED"

	// ERR_AUTH_INVALID_TOKEN indicates authentication token is invalid
	// Context: JWT signature verification failed, token format malformed, corrupted token data,
	// tampered token detected, invalid encryption
	ERR_AUTH_INVALID_TOKEN errorkit.ErrorCode = "ERR_AUTH_INVALID_TOKEN"

	// ERR_AUTH_TOKEN_EXPIRED indicates authentication token has expired
	// Context: JWT exp claim exceeded, session timeout, refresh token expired,
	// access token lifetime exceeded, temporary token expired
	ERR_AUTH_TOKEN_EXPIRED errorkit.ErrorCode = "ERR_AUTH_TOKEN_EXPIRED"

	// ERR_AUTH_INVALID_CREDENTIALS indicates login credentials are wrong
	// Context: Incorrect username/password, wrong API key, invalid client secret,
	// failed biometric verification, incorrect PIN/OTP
	ERR_AUTH_INVALID_CREDENTIALS errorkit.ErrorCode = "ERR_AUTH_INVALID_CREDENTIALS"

	// ERR_AUTH_INTEGRATION_NOT_FOUND indicates OAuth integration not found
	// Context: No integration found for the given provider
	ERR_AUTH_INTEGRATION_NOT_FOUND errorkit.ErrorCode = "ERR_AUTH_INTEGRATION_NOT_FOUND"
)

// ErrorMetadata is the canonical metadata for httprequest-owned error codes.
var ErrorMetadata = []errorkit.Metadata{
	{
		Code:        ERR_NETWORK_ERROR,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupSystem,
		Category:    "network",
		Description: "Network error",
		HTTPStatus:  503,
		Retriable:   true,
	},
	{
		Code:        ERR_NETWORK_TIMEOUT,
		Type:        errorkit.ErrorTypeExternal,
		Group:       errorkit.GroupSystem,
		Category:    "network",
		Description: "Network timeout",
		HTTPStatus:  504,
		Retriable:   true,
	},
	{
		Code:        ERR_AUTH_UNAUTHENTICATED,
		Type:        errorkit.ErrorTypeInternal,
		Group:       errorkit.GroupAuth,
		Category:    "auth",
		Description: "Authentication required",
		HTTPStatus:  401,
		Retriable:   false,
	},
	{
		Code:        ERR_AUTH_UNAUTHORIZED,
		Type:        errorkit.ErrorTypeInternal,
		Group:       errorkit.GroupAuth,
		Category:    "auth",
		Description: "Not authorized to access resource",
		HTTPStatus:  403,
		Retriable:   false,
	},
	{
		Code:        ERR_AUTH_INVALID_TOKEN,
		Type:        errorkit.ErrorTypeInternal,
		Group:       errorkit.GroupAuth,
		Category:    "auth",
		Description: "Invalid authentication token",
		HTTPStatus:  401,
		Retriable:   false,
	},
	{
		Code:        ERR_AUTH_TOKEN_EXPIRED,
		Type:        errorkit.ErrorTypeInternal,
		Group:       errorkit.GroupAuth,
		Category:    "auth",
		Description: "Authentication token expired",
		HTTPStatus:  401,
		Retriable:   false,
	},
	{
		Code:        ERR_AUTH_INVALID_CREDENTIALS,
		Type:        errorkit.ErrorTypeInternal,
		Group:       errorkit.GroupAuth,
		Category:    "auth",
		Description: "Invalid credentials provided",
		HTTPStatus:  401,
		Retriable:   false,
	},
	{
		Code:        ERR_AUTH_INTEGRATION_NOT_FOUND,
		Type:        errorkit.ErrorTypeInternal,
		Group:       errorkit.GroupAuth,
		Category:    "auth",
		Description: "OAuth integration not found for provider",
		HTTPStatus:  404,
		Retriable:   false,
	},
}

// RegisterErrors registers httprequest-owned error metadata. When registry is
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
