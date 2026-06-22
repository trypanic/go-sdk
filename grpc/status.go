package grpc

import (
	"errors"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/trypanic/go-sdk/errorkit"
)

// codeFor maps an errorkit error code to the closest gRPC status code.
func codeFor(c errorkit.ErrorCode) codes.Code {
	switch c {
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
	default:
		// ERR_SYSTEM_CONFIG_INVALID, ERR_SYSTEM_UNEXPECTED, ERR_INTERNAL,
		// ERR_UNKNOWN, and any unrecognized code.
		return codes.Internal
	}
}

// ToStatus converts an error into a gRPC status error suitable for returning
// from a handler.
//
// An errorkit.AppError is mapped to the gRPC code from codeFor, with its
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
		st := status.New(codeFor(appErr.Code()), appErr.Error())
		if withDetail, derr := st.WithDetails(&errdetails.ErrorInfo{
			Reason: string(appErr.Code()),
			Domain: "go-sdk",
		}); derr == nil {
			st = withDetail
		}
		return st.Err()
	}

	return status.Convert(err).Err()
}
