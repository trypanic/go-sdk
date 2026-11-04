package grpc

import (
	"context"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/grpc/internal/testsvc"
)

// mustErrorInfo returns the ErrorInfo detail of err, failing the test if
// there is none.
func mustErrorInfo(t *testing.T, err error) *errdetails.ErrorInfo {
	t.Helper()
	info := errorInfo(status.Convert(err))
	if info == nil {
		t.Fatalf("status %v has no ErrorInfo detail", err)
	}
	return info
}

// ErrorInfo.Metadata carries the errorkit ID and the registered Retriable
// flag, so callers can decide on retries without parsing messages.
func TestToStatus_ErrorInfoMetadataCarriesIDAndRetriable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		code errorkit.ErrorCode
		want string
	}{
		{errorkit.ERR_SYSTEM_TIMEOUT_INTERNAL, "true"},
		{errorkit.ERR_VALIDATION_MISSING_FIELD, "false"},
	}
	for _, tc := range cases {
		t.Run(string(tc.code), func(t *testing.T) {
			appErr := errorkit.NewError(tc.code)
			info := mustErrorInfo(t, ToStatus(appErr))
			if got := info.GetMetadata()["id"]; got != appErr.ID {
				t.Fatalf("Metadata[id] = %q, want %q", got, appErr.ID)
			}
			if got := info.GetMetadata()["retriable"]; got != tc.want {
				t.Fatalf("Metadata[retriable] = %q, want %q", got, tc.want)
			}
		})
	}
}

// StatusPolicy.Domain sets ErrorInfo.Domain; the zero value keeps "go-sdk".
func TestToStatusWithPolicy_Domain(t *testing.T) {
	t.Parallel()
	appErr := errorkit.NewError(errorkit.ERR_CLIENT_NOT_FOUND)
	if got := mustErrorInfo(t, ToStatus(appErr)).GetDomain(); got != "go-sdk" {
		t.Fatalf("default Domain = %q, want %q", got, "go-sdk")
	}
	got := mustErrorInfo(t, ToStatusWithPolicy(appErr, StatusPolicy{Domain: "payments.example.com"})).GetDomain()
	if got != "payments.example.com" {
		t.Fatalf("Domain = %q, want %q", got, "payments.example.com")
	}
}

// panickingImpl panics in the unary and server-stream handlers.
func panickingImpl() *echoImpl {
	return &echoImpl{
		unary: func(context.Context, *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
			panic(panicSecret)
		},
		serverStream: func(grpc.ServerStream) error { panic(panicSecret) },
	}
}

// recoveredErrors makes one unary and one server-stream call against a
// panicking handler and returns the status errors the client received.
func recoveredErrors(t *testing.T, opts ...ServerOption) map[string]error {
	t.Helper()
	cc := startTestServer(t, panickingImpl(), opts...)
	unaryErr := cc.Invoke(testCtx(t), testsvc.UnaryMethod, wrapperspb.String("x"), new(wrapperspb.StringValue))

	cs, err := cc.NewStream(testCtx(t), testsvc.ServerStreamDesc, testsvc.ServerStreamMethod)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := cs.SendMsg(wrapperspb.String("x")); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	if err := cs.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	streamErr := cs.RecvMsg(new(wrapperspb.StringValue))
	return map[string]error{"unary": unaryErr, "stream": streamErr}
}

// The recovery status is built by ToStatus: the same code, message and
// ErrorInfo shape a handler returning an ERR_SYSTEM_UNEXPECTED gets.
func TestRecovery_StatusBuiltLikeToStatus(t *testing.T) {
	t.Parallel()
	want := status.Convert(ToStatus(errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED)))
	for mode, err := range recoveredErrors(t) {
		t.Run(mode, func(t *testing.T) {
			st := status.Convert(err)
			if st.Code() != codes.Internal || st.Message() != want.Message() {
				t.Fatalf("status = %s %q, want %s %q", st.Code(), st.Message(), codes.Internal, want.Message())
			}
			info := mustErrorInfo(t, err)
			if info.GetDomain() != "go-sdk" {
				t.Fatalf("Domain = %q, want %q", info.GetDomain(), "go-sdk")
			}
			if info.GetMetadata()["retriable"] != "false" {
				t.Fatalf("Metadata[retriable] = %q, want %q", info.GetMetadata()["retriable"], "false")
			}
		})
	}
}

// WithServerErrorDomain sets ErrorInfo.Domain on recovered panics.
func TestRecovery_ServerErrorDomain(t *testing.T) {
	t.Parallel()
	for mode, err := range recoveredErrors(t, WithServerErrorDomain("payments.example.com")) {
		t.Run(mode, func(t *testing.T) {
			if got := mustErrorInfo(t, err).GetDomain(); got != "payments.example.com" {
				t.Fatalf("Domain = %q, want %q", got, "payments.example.com")
			}
		})
	}
}
