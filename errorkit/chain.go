package errorkit

import "errors"

// As reports whether err's chain contains a non-nil *AppError and returns
// the first one. It follows fmt.Errorf("%w") wrappers, errors.Join and any
// Unwrap method, unlike a direct type assertion (err.(*AppError)), which
// fails as soon as a layer adds context:
//
//	if appErr, ok := errorkit.As(err); ok {
//		logger.ErrorCtx(ctx, appErr, "svc.Do failed")
//	}
func As(err error) (*AppError, bool) {
	appErr, ok := errors.AsType[*AppError](err)
	if !ok || appErr == nil {
		return nil, false
	}
	return appErr, true
}

// HasCode reports whether err's chain contains an *AppError with the given
// code. It checks the first *AppError found by As and then every *AppError
// reachable through its Wrapped error, so a code nested inside another
// AppError (for example ERR_INTERNAL wrapping a timeout) is found too.
//
// Prefer HasCode over comparing Error() strings or asserting on the dynamic
// type. AppError deliberately has no Is method: two errors with the same code
// are not errors.Is-equal.
func HasCode(err error, code ErrorCode) bool {
	for {
		appErr, ok := As(err)
		if !ok {
			return false
		}
		if appErr.ErrCode == code {
			return true
		}
		err = appErr.Wrapped
	}
}
