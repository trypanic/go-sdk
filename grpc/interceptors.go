package grpc

import (
	"context"
	"runtime/debug"

	"google.golang.org/grpc"

	"github.com/trypanic/go-sdk/v2/errorkit"
	"github.com/trypanic/go-sdk/v2/logger"
)

// recoveryUnaryInterceptor recovers panics from unary handlers, logs them via
// the SDK logger, and returns a codes.Internal status so the server stays up.
func recoveryUnaryInterceptor(log *logger.Logger, domain string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = recoverToStatus(ctx, log, domain, info.FullMethod, r)
			}
		}()
		return handler(ctx, req)
	}
}

// recoveryStreamInterceptor recovers panics from streaming handlers (server-,
// client-, and bidi-streaming), logs them, and returns a codes.Internal status.
func recoveryStreamInterceptor(log *logger.Logger, domain string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = recoverToStatus(ss.Context(), log, domain, info.FullMethod, r)
			}
		}()
		return handler(srv, ss)
	}
}

// recoverToStatus builds the errorkit error for a recovered panic, logs it, and
// returns the gRPC status error to send back to the caller, built by
// ToStatusWithPolicy with domain as the ErrorInfo.Domain. The panic value is
// never sent to the caller: ERR_SYSTEM_UNEXPECTED is a server error, so the
// status carries only its registered description, plus an ErrorInfo with the
// errorkit code and the error ID that keys the log entry.
func recoverToStatus(ctx context.Context, log *logger.Logger, domain, method string, r any) error {
	appErr := errorkit.NewError(errorkit.ERR_SYSTEM_UNEXPECTED).With(
		errorkit.WithReason("grpc: panic recovered in %s: %v", method, r),
		errorkit.WithPayload(string(debug.Stack())),
	)
	if log != nil {
		log.ErrorCtx(ctx, appErr, "grpc: panic recovered")
	} else {
		logger.ErrorCtx(ctx, appErr, "grpc: panic recovered")
	}
	return ToStatusWithPolicy(appErr, StatusPolicy{Domain: domain})
}
