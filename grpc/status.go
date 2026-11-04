package grpc

import (
	"errors"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/trypanic/go-sdk/errorkit"
)

// defaultErrorDomain is the errdetails.ErrorInfo.Domain set on statuses built
// by this package.
const defaultErrorDomain = "go-sdk"

// codeFor maps an errorkit error to the closest gRPC status code. The core
// errorkit codes have an explicit mapping; any other code (for example one a
// package registered with errorkit.MustRegister) falls back to its registered
// Metadata.HTTPStatus and Metadata.Retriable through codeForMetadata.
func codeFor(appErr *errorkit.AppError) codes.Code {
	switch appErr.Code() {
	case errorkit.ERR_VALIDATION,
		errorkit.ERR_VALIDATION_INVALID_FORMAT,
		errorkit.ERR_VALIDATION_MISSING_FIELD,
		errorkit.ERR_VALIDATION_INCONSISTENT,
		errorkit.ERR_VALIDATION_BUSINESS_RULE,
		errorkit.ERR_CLIENT_BAD_REQUEST:
		return codes.InvalidArgument
	case errorkit.ERR_VALIDATION_DUPLICATE:
		return codes.AlreadyExists
	case errorkit.ERR_CLIENT_NOT_FOUND:
		return codes.NotFound
	case errorkit.ERR_CLIENT_RATE_LIMIT:
		return codes.ResourceExhausted
	case errorkit.ERR_SYSTEM_TIMEOUT_INTERNAL:
		return codes.DeadlineExceeded
	case errorkit.ERR_SYSTEM_CONCURRENCY:
		return codes.Aborted
	case errorkit.ERR_SYSTEM_CONFIG_INVALID,
		errorkit.ERR_SYSTEM_UNEXPECTED,
		errorkit.ERR_INTERNAL,
		errorkit.ERR_UNKNOWN:
		return codes.Internal
	default:
		return codeForMetadata(appErr.Metadata)
	}
}

// codeForMetadata maps registered errorkit metadata to a gRPC code: the HTTP
// status picks the class and Retriable breaks ties, so a retriable server
// error becomes Unavailable (which gRPC retry policies act on) instead of
// Internal.
func codeForMetadata(meta errorkit.Metadata) codes.Code {
	switch meta.HTTPStatus {
	case 401:
		return codes.Unauthenticated
	case 403:
		return codes.PermissionDenied
	case 404:
		return codes.NotFound
	case 408, 504:
		return codes.DeadlineExceeded
	case 409:
		if meta.Retriable {
			return codes.Aborted
		}
		return codes.AlreadyExists
	case 429:
		return codes.ResourceExhausted
	case 499:
		return codes.Canceled
	case 501:
		return codes.Unimplemented
	case 503:
		return codes.Unavailable
	}
	switch {
	case meta.HTTPStatus >= 400 && meta.HTTPStatus < 500:
		return codes.InvalidArgument
	case meta.Retriable:
		return codes.Unavailable
	default:
		return codes.Internal
	}
}

// internalErrorMessage is the status message sent for a server-side error
// whose registered metadata has no description.
const internalErrorMessage = "internal error"

// StatusPolicy controls how ToStatusWithPolicy builds a gRPC status from an
// errorkit error. The zero value is the policy ToStatus applies.
type StatusPolicy struct {
	// ExposeInternalReasons sends the errorkit reason ("[CODE] reason") as the
	// status message for every code, server-side errors included. Reasons
	// often embed driver errors, hosts, SQL and file paths, so enable it only
	// for callers you trust (e.g. internal tooling), never on an endpoint
	// reachable by untrusted clients.
	ExposeInternalReasons bool
}

// ToStatus converts an error into a gRPC status error suitable for returning
// from a handler. It applies the zero StatusPolicy; see ToStatusWithPolicy.
//
// An errorkit.AppError is mapped to a gRPC code (core codes explicitly, any
// other code from its registered HTTPStatus and Retriable metadata). The
// status message depends on the class of the error:
//
//   - client errors (registered HTTPStatus below 500, and a gRPC code other
//     than Internal, Unknown or DataLoss) keep the errorkit message
//     ("[CODE] reason"), which tells the caller what to fix;
//   - server errors (HTTPStatus 500 or above, or a gRPC code of Internal,
//     Unknown or DataLoss) send only the registered Metadata.Description, or
//     "internal error" when it is empty, so internal details never reach the
//     caller.
//
// In both cases the errorkit code and error ID are surfaced as an
// errdetails.ErrorInfo (Reason = the code, Metadata["id"] = the error ID) so
// callers can read them programmatically and correlate with server logs.
//
// Any other error falls back to status.Convert, which preserves an existing
// gRPC status or yields codes.Unknown for a plain error. A nil error returns
// nil.
func ToStatus(err error) error {
	return ToStatusWithPolicy(err, StatusPolicy{})
}

// ToStatusWithPolicy is ToStatus with an explicit StatusPolicy.
func ToStatusWithPolicy(err error, policy StatusPolicy) error {
	if err == nil {
		return nil
	}

	var appErr *errorkit.AppError
	if errors.As(err, &appErr) {
		code := codeFor(appErr)
		st := status.New(code, statusMessage(appErr, code, policy))
		if withDetail, derr := st.WithDetails(&errdetails.ErrorInfo{
			Reason:   string(appErr.Code()),
			Domain:   defaultErrorDomain,
			Metadata: map[string]string{"id": appErr.ID},
		}); derr == nil {
			st = withDetail
		}
		return st.Err()
	}

	return status.Convert(err).Err()
}

// statusMessage picks the status message for appErr under policy: the errorkit
// message for client errors, a stable description for server errors.
func statusMessage(appErr *errorkit.AppError, code codes.Code, policy StatusPolicy) string {
	if policy.ExposeInternalReasons || !isServerError(appErr, code) {
		return appErr.Error()
	}
	if appErr.Metadata.Description != "" {
		return appErr.Metadata.Description
	}
	return internalErrorMessage
}

// isServerError reports whether appErr is a server-side failure whose reason
// must not be sent to the caller.
func isServerError(appErr *errorkit.AppError, code codes.Code) bool {
	switch code {
	case codes.Internal, codes.Unknown, codes.DataLoss:
		return true
	}
	return appErr.Metadata.HTTPStatus >= 500
}
