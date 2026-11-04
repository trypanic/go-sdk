package grpc

import (
	"context"
	"runtime/debug"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/trypanic/go-sdk/errorkit"
	"github.com/trypanic/go-sdk/logger"
)

// recoveryUnaryInterceptor recovers panics from unary handlers, logs them via
// the SDK logger, and returns a codes.Internal status so the server stays up.
func recoveryUnaryInterceptor(log *logger.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = recoverToStatus(ctx, log, info.FullMethod, r)
			}
		}()
		return handler(ctx, req)
	}
}

// recoveryStreamInterceptor recovers panics from streaming handlers (server-,
// client-, and bidi-streaming), logs them, and returns a codes.Internal status.
func recoveryStreamInterceptor(log *logger.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = recoverToStatus(ss.Context(), log, info.FullMethod, r)
			}
		}()
		return handler(srv, ss)
	}
}

// recoveredPanicMessage is the fixed status message sent to the caller for a
// recovered panic. The panic value and method stay in the server log.
const recoveredPanicMessage = "internal error"

// recoverToStatus builds the errorkit error for a recovered panic, logs it, and
// returns the gRPC status error to send back to the caller. The panic value is
// never sent to the caller: the status carries a fixed message plus an
// ErrorInfo with the errorkit code and the error ID that keys the log entry.
func recoverToStatus(ctx context.Context, log *logger.Logger, method string, r any) error {
	appErr := errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).With(
		errorkit.WithReason("grpc: panic recovered in %s: %v", method, r),
		errorkit.WithPayload(string(debug.Stack())),
	)
	if log != nil {
		log.ErrorCtx(ctx, appErr, "grpc: panic recovered")
	} else {
		logger.ErrorCtx(ctx, appErr, "grpc: panic recovered")
	}
	st := status.New(codes.Internal, recoveredPanicMessage)
	if withDetail, derr := st.WithDetails(&errdetails.ErrorInfo{
		Reason:   string(appErr.Code()),
		Domain:   defaultErrorDomain,
		Metadata: map[string]string{"id": appErr.ID},
	}); derr == nil {
		st = withDetail
	}
	return st.Err()
}
