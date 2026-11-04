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

// ToStatus converts an error into a gRPC status error suitable for returning
// from a handler.
//
// An errorkit.AppError is mapped to a gRPC code (core codes explicitly, any
// other code from its registered HTTPStatus and Retriable metadata), with its
// message preserved and its errorkit code surfaced as an errdetails.ErrorInfo
// (Reason = the code) so callers can read it programmatically. Any other error
// falls back to status.Convert, which preserves an existing gRPC status or
// yields codes.Unknown for a plain error. A nil error returns nil.
func ToStatus(err error) error {
	if err == nil {
		return nil
	}

	var appErr *errorkit.AppError
	if errors.As(err, &appErr) {
		st := status.New(codeFor(appErr), appErr.Error())
		if withDetail, derr := st.WithDetails(&errdetails.ErrorInfo{
			Reason: string(appErr.Code()),
			Domain: defaultErrorDomain,
		}); derr == nil {
			st = withDetail
		}
		return st.Err()
	}

	return status.Convert(err).Err()
}
