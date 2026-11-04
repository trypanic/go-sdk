package grpc

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trypanic/go-sdk/v2/errorkit"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestFromStatusRoundTripAndLocalMetadata(t *testing.T) {
	original := errorkit.NewError(errorkit.ERR_CLIENT_NOT_FOUND)
	converted := FromStatus(ToStatus(original))
	app, ok := errorkit.As(converted)
	require.True(t, ok)
	require.Equal(t, original.Code(), app.Code())
	require.Equal(t, original.Metadata, app.Metadata)
	st, err := status.New(codes.Unavailable, "private server details").WithDetails(&errdetails.ErrorInfo{
		Reason: string(errorkit.ERR_EXTERNAL_SERVICE_UNAVAILABLE), Metadata: map[string]string{"retriable": "false"},
	})
	require.NoError(t, err)
	remote := st.Err()
	converted = FromStatus(remote)
	app, ok = errorkit.As(converted)
	require.True(t, ok)
	require.True(t, app.Metadata.Retriable)
	require.ErrorIs(t, converted, remote)
	require.Empty(t, app.Reason)
	require.Nil(t, FromStatus(nil))
	plain := errors.New("local")
	require.Same(t, plain, FromStatus(plain))
	require.Same(t, original, FromStatus(original))
}

func TestFromStatusFallbacks(t *testing.T) {
	for _, tt := range []struct {
		code codes.Code
		want errorkit.ErrorCode
	}{
		{codes.InvalidArgument, errorkit.ERR_VALIDATION}, {codes.NotFound, errorkit.ERR_CLIENT_NOT_FOUND},
		{codes.AlreadyExists, errorkit.ERR_VALIDATION_DUPLICATE}, {codes.ResourceExhausted, errorkit.ERR_CLIENT_RATE_LIMIT},
		{codes.Aborted, errorkit.ERR_SYSTEM_CONCURRENCY}, {codes.Unavailable, errorkit.ERR_EXTERNAL_SERVICE_UNAVAILABLE},
		{codes.DeadlineExceeded, errorkit.ERR_EXTERNAL_SERVICE_TIMEOUT}, {codes.Canceled, errorkit.ERR_EXTERNAL_INVALID_RESPONSE},
		{codes.Internal, errorkit.ERR_EXTERNAL_SERVICE_ERROR}, {codes.PermissionDenied, errorkit.ERR_EXTERNAL_INVALID_RESPONSE},
	} {
		t.Run(tt.code.String(), func(t *testing.T) {
			st, err := status.New(tt.code, "secret").WithDetails(&errdetails.ErrorInfo{Reason: "UNKNOWN_REMOTE_CODE"})
			require.NoError(t, err)
			converted := FromStatus(st.Err())
			require.True(t, errorkit.HasCode(converted, tt.want))
		})
	}
}

func TestWrappedContextStatusMessageIsCanonical(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		got := status.Convert(ToStatus(fmt.Errorf("password=secret: %w", cause)))
		require.Equal(t, cause.Error(), got.Message())
	}
}

func TestFromStatusRejectsMismatchedErrorInfo(t *testing.T) {
	st, err := status.New(codes.Unavailable, "remote").WithDetails(&errdetails.ErrorInfo{Reason: string(errorkit.ERR_CLIENT_NOT_FOUND)})
	require.NoError(t, err)
	require.True(t, errorkit.HasCode(FromStatus(st.Err()), errorkit.ERR_EXTERNAL_SERVICE_UNAVAILABLE))
}

func TestToStatusSkipsTypedNilInJoinedErrors(t *testing.T) {
	var absent *errorkit.AppError
	expected := errorkit.NewError(errorkit.ERR_CLIENT_NOT_FOUND)
	joined := errors.Join(absent, expected)
	require.NotPanics(t, func() { require.Equal(t, codes.NotFound, status.Code(ToStatus(joined))) })
	require.NotPanics(t, func() { require.Equal(t, codes.Unknown, status.Code(ToStatus(absent))) })
}
