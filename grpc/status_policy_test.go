package grpc

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/trypanic/go-sdk/v2/errorkit"
	"github.com/trypanic/go-sdk/v2/grpc/internal/testsvc"
	"github.com/trypanic/go-sdk/v2/postgres"
)

const internalDetail = `pq: relation "users" does not exist (host=db-primary.internal)`

// What the remote caller sees, over the wire, for each class of errorkit
// code returned through ToStatus.
func TestToStatus_OverTheWireMessagePerClass(t *testing.T) {
	cases := []struct {
		name     string
		code     errorkit.ErrorCode
		wantCode codes.Code
		// wantReason: the errorkit reason reaches the caller (client-error
		// classes); otherwise only the registered description does.
		wantReason bool
	}{
		{"client error keeps reason", errorkit.ERR_VALIDATION_MISSING_FIELD, codes.InvalidArgument, true},
		{"not found keeps reason", errorkit.ERR_CLIENT_NOT_FOUND, codes.NotFound, true},
		{"conflict keeps reason", errorkit.ERR_SYSTEM_CONCURRENCY, codes.Aborted, true},
		{"internal hides reason", errorkit.ERR_INTERNAL, codes.Internal, false},
		{"unexpected hides reason", errorkit.ERR_SYSTEM_UNEXPECTED, codes.Internal, false},
		{"unknown hides reason", errorkit.ERR_UNKNOWN, codes.Internal, false},
		{"internal timeout hides reason", errorkit.ERR_SYSTEM_TIMEOUT_INTERNAL, codes.DeadlineExceeded, false},
		{"unavailable dependency hides reason", postgres.ERR_DB_POSTGRES_UNAVAILABLE, codes.Unavailable, false},
		{"unregistered code hides reason", errorkit.ErrorCode("ERR_TEST_GRPC_NOT_REGISTERED"), codes.Internal, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appErr := errorkit.NewError(tc.code).With(errorkit.WithReason("%s", internalDetail))
			impl := &echoImpl{
				unary: func(context.Context, *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
					return nil, ToStatus(appErr)
				},
			}
			cc := startTestServer(t, impl)

			err := cc.Invoke(testCtx(t), testsvc.UnaryMethod, wrapperspb.String("x"), new(wrapperspb.StringValue))
			st := status.Convert(err)
			if st.Code() != tc.wantCode {
				t.Fatalf("code = %s, want %s", st.Code(), tc.wantCode)
			}
			if tc.wantReason {
				if st.Message() != appErr.Error() {
					t.Fatalf("message = %q, want %q", st.Message(), appErr.Error())
				}
			} else {
				if strings.Contains(st.Message(), "db-primary") {
					t.Fatalf("internal reason leaked to the client: %q", st.Message())
				}
				if want := appErr.Metadata.Description; st.Message() != want {
					t.Fatalf("message = %q, want registered description %q", st.Message(), want)
				}
			}
			info := errorInfo(st)
			if info == nil {
				t.Fatal("no ErrorInfo detail")
			}
			if info.GetReason() != string(tc.code) {
				t.Fatalf("ErrorInfo.Reason = %q, want %q", info.GetReason(), tc.code)
			}
			if info.GetMetadata()["id"] != appErr.ID {
				t.Fatalf("ErrorInfo.Metadata[id] = %q, want %q", info.GetMetadata()["id"], appErr.ID)
			}
		})
	}
}

func TestToStatus_EmptyDescriptionFallsBackToFixedMessage(t *testing.T) {
	appErr := errorkit.NewError(errorkit.ERR_INTERNAL).With(errorkit.WithReason("%s", internalDetail))
	appErr.Metadata.Description = ""
	st := status.Convert(ToStatus(appErr))
	if st.Message() != "internal error" {
		t.Fatalf("message = %q, want %q", st.Message(), "internal error")
	}
}

// ToStatusWithPolicy can keep the pre-2.0 behavior (the reason on the wire for
// every class), for trusted internal callers only.
func TestToStatusWithPolicy_ExposeInternalReasons(t *testing.T) {
	appErr := errorkit.NewError(errorkit.ERR_INTERNAL).With(errorkit.WithReason("%s", internalDetail))
	st := status.Convert(ToStatusWithPolicy(appErr, StatusPolicy{ExposeInternalReasons: true}))
	if st.Code() != codes.Internal {
		t.Fatalf("code = %s, want %s", st.Code(), codes.Internal)
	}
	if st.Message() != appErr.Error() {
		t.Fatalf("message = %q, want %q", st.Message(), appErr.Error())
	}
}
