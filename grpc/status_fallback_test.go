package grpc

import (
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/httpclient"
	"github.com/trypanic/go-sdk/httprequest"
	"github.com/trypanic/go-sdk/mongo"
	"github.com/trypanic/go-sdk/postgres"

	// Imported for their init(), which registers each package's codes in the
	// global errorkit registry swept below.
	_ "github.com/trypanic/go-sdk/messaging"
	_ "github.com/trypanic/go-sdk/storage"
)

// Codes named in finding C3-003, with the gRPC code their registered
// metadata calls for.
func TestToStatus_PackageCodesUseRegisteredMetadata(t *testing.T) {
	cases := []struct {
		code errorkit.ErrorCode
		want codes.Code
	}{
		{httprequest.ERR_AUTH_UNAUTHENTICATED, codes.Unauthenticated},
		{httprequest.ERR_AUTH_UNAUTHORIZED, codes.PermissionDenied},
		{httprequest.ERR_NETWORK_TIMEOUT, codes.DeadlineExceeded},
		{mongo.ERR_DB_MONGO_NOT_FOUND, codes.NotFound},
		{postgres.ERR_DB_POSTGRES_UNAVAILABLE, codes.Unavailable},
		{postgres.ERR_DB_POSTGRES_TIMEOUT, codes.DeadlineExceeded},
		{httpclient.ERR_EXTERNAL_SERVICE_UNAVAILABLE, codes.Unavailable},
	}
	for _, tc := range cases {
		t.Run(string(tc.code), func(t *testing.T) {
			st := status.Convert(ToStatus(errorkit.NewError(tc.code)))
			if st.Code() != tc.want {
				t.Fatalf("code = %s, want %s", st.Code(), tc.want)
			}
		})
	}
}

// Every code in the global registry (core codes plus every package's codes)
// maps consistently with its registered HTTPStatus and Retriable flag.
func TestToStatus_EveryRegisteredCodeFollowsItsMetadata(t *testing.T) {
	for _, code := range errorkit.GetAllCodes() {
		meta, _ := errorkit.GetMetadata(code)
		t.Run(string(code), func(t *testing.T) {
			got := status.Code(ToStatus(errorkit.NewError(code)))
			switch meta.HTTPStatus {
			case 401:
				assertGRPCCode(t, got, codes.Unauthenticated)
			case 403:
				assertGRPCCode(t, got, codes.PermissionDenied)
			case 404:
				assertGRPCCode(t, got, codes.NotFound)
			case 429:
				assertGRPCCode(t, got, codes.ResourceExhausted)
			case 503:
				assertGRPCCode(t, got, codes.Unavailable)
			case 504:
				assertGRPCCode(t, got, codes.DeadlineExceeded)
			}
			if meta.HTTPStatus < 500 && (got == codes.Internal || got == codes.Unknown) {
				t.Fatalf("client-error code (HTTP %d) mapped to %s", meta.HTTPStatus, got)
			}
			if meta.HTTPStatus >= 500 && meta.Retriable && got == codes.Internal {
				t.Fatalf("retriable server error (HTTP %d) mapped to Internal; clients will not retry it", meta.HTTPStatus)
			}
		})
	}
}

// The fallback for codes outside the core switch, driven by HTTPStatus with
// Retriable as the tie-breaker.
func TestToStatus_MetadataFallbackTable(t *testing.T) {
	cases := []struct {
		http      int
		retriable bool
		want      codes.Code
	}{
		{400, false, codes.InvalidArgument},
		{401, false, codes.Unauthenticated},
		{403, false, codes.PermissionDenied},
		{404, false, codes.NotFound},
		{408, true, codes.DeadlineExceeded},
		{409, false, codes.AlreadyExists},
		{409, true, codes.Aborted},
		{410, false, codes.InvalidArgument},
		{422, false, codes.InvalidArgument},
		{429, true, codes.ResourceExhausted},
		{499, false, codes.Canceled},
		{500, false, codes.Internal},
		{500, true, codes.Unavailable},
		{501, false, codes.Unimplemented},
		{502, false, codes.Internal},
		{502, true, codes.Unavailable},
		{503, true, codes.Unavailable},
		{504, true, codes.DeadlineExceeded},
	}
	for _, tc := range cases {
		code := errorkit.ErrorCode(fmt.Sprintf("ERR_TEST_GRPC_FALLBACK_%d_%t", tc.http, tc.retriable))
		errorkit.MustRegister(errorkit.Metadata{
			Code:        code,
			Type:        errorkit.ErrorTypeInternal,
			Group:       errorkit.GroupUnknown,
			Category:    "test",
			Description: "grpc fallback test code",
			HTTPStatus:  tc.http,
			Retriable:   tc.retriable,
		})
		t.Run(string(code), func(t *testing.T) {
			assertGRPCCode(t, status.Code(ToStatus(errorkit.NewError(code))), tc.want)
		})
	}
}

func assertGRPCCode(t *testing.T, got, want codes.Code) {
	t.Helper()
	if got != want {
		t.Fatalf("gRPC code = %s, want %s", got, want)
	}
}
