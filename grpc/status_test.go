package grpc

import (
	"errors"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestToStatus_ErrorkitCodeMapping(t *testing.T) {
	cases := []struct {
		code errorkit.ErrorCode
		want codes.Code
	}{
		{errorkit.ERR_VALIDATION, codes.InvalidArgument},
		{errorkit.ERR_VALIDATION_INVALID_FORMAT, codes.InvalidArgument},
		{errorkit.ERR_VALIDATION_MISSING_FIELD, codes.InvalidArgument},
		{errorkit.ERR_VALIDATION_INCONSISTENT, codes.InvalidArgument},
		{errorkit.ERR_VALIDATION_BUSINESS_RULE, codes.InvalidArgument},
		{errorkit.ERR_CLIENT_BAD_REQUEST, codes.InvalidArgument},
		{errorkit.ERR_VALIDATION_DUPLICATE, codes.AlreadyExists},
		{errorkit.ERR_CLIENT_NOT_FOUND, codes.NotFound},
		{errorkit.ERR_CLIENT_RATE_LIMIT, codes.ResourceExhausted},
		{errorkit.ERR_SYSTEM_TIMEOUT_INTERNAL, codes.DeadlineExceeded},
		{errorkit.ERR_SYSTEM_CONCURRENCY, codes.Aborted},
		{errorkit.ERR_SYSTEM_CONFIG_INVALID, codes.Internal},
		{errorkit.ERR_SYSTEM_UNEXPECTED, codes.Internal},
		{errorkit.ERR_INTERNAL, codes.Internal},
		{errorkit.ERR_UNKNOWN, codes.Internal},
	}

	for _, tc := range cases {
		t.Run(string(tc.code), func(t *testing.T) {
			appErr := errorkit.NewError(tc.code).With(errorkit.WithReason("boom"))
			st := status.Convert(ToStatus(appErr))

			if st.Code() != tc.want {
				t.Fatalf("code = %s, want %s", st.Code(), tc.want)
			}
			// message preserved (errorkit formats as "[CODE] reason")
			if st.Message() != appErr.Error() {
				t.Fatalf("message = %q, want %q", st.Message(), appErr.Error())
			}
			// errorkit code surfaced as ErrorInfo.Reason
			var reason string
			for _, d := range st.Details() {
				if info, ok := d.(*errdetails.ErrorInfo); ok {
					reason = info.GetReason()
				}
			}
			if reason != string(tc.code) {
				t.Fatalf("ErrorInfo.Reason = %q, want %q", reason, string(tc.code))
			}
		})
	}
}

func TestToStatus_NilReturnsNil(t *testing.T) {
	if got := ToStatus(nil); got != nil {
		t.Fatalf("ToStatus(nil) = %v, want nil", got)
	}
}

func TestToStatus_NonErrorkitFallsBackToUnknown(t *testing.T) {
	st := status.Convert(ToStatus(errors.New("plain")))
	if st.Code() != codes.Unknown {
		t.Fatalf("code = %s, want %s", st.Code(), codes.Unknown)
	}
	if st.Message() != "plain" {
		t.Fatalf("message = %q, want %q", st.Message(), "plain")
	}
}

func TestToStatus_ExistingStatusPassthrough(t *testing.T) {
	in := status.New(codes.PermissionDenied, "nope").Err()
	st := status.Convert(ToStatus(in))
	if st.Code() != codes.PermissionDenied {
		t.Fatalf("code = %s, want %s", st.Code(), codes.PermissionDenied)
	}
}
