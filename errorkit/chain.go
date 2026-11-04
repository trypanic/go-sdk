package errorkit

// As reports whether err's chain contains a non-nil *AppError and returns
// the first one. It follows fmt.Errorf("%w") wrappers, errors.Join and any
// Unwrap method, unlike a direct type assertion (err.(*AppError)), which
// fails as soon as a layer adds context. Custom As methods are consulted,
// but a typed-nil match does not stop traversal:
//
//	if appErr, ok := errorkit.As(err); ok {
//		logger.ErrorCtx(ctx, appErr, "svc.Do failed")
//	}
func As(err error) (*AppError, bool) {
	for err != nil {
		if appErr, ok := err.(*AppError); ok {
			if appErr != nil {
				return appErr, true
			}
			return nil, false
		}
		if x, ok := err.(interface{ As(any) bool }); ok {
			var appErr *AppError
			if x.As(&appErr) && appErr != nil {
				return appErr, true
			}
		}
		switch x := err.(type) {
		case interface{ Unwrap() error }:
			err = x.Unwrap()
		case interface{ Unwrap() []error }:
			for _, branch := range x.Unwrap() {
				if appErr, ok := As(branch); ok {
					return appErr, true
				}
			}
			return nil, false
		default:
			return nil, false
		}
	}
	return nil, false
}

// HasCode reports whether err's tree contains an *AppError with the given
// code. Like errors.Is it walks the whole tree: fmt.Errorf("%w") wrappers,
// errors.Join and any Unwrap() error / Unwrap() []error method, including
// the Wrapped error of every AppError, so a code nested inside another
// AppError (for example ERR_INTERNAL wrapping a timeout) or in any branch of
// a joined error is found. Custom As methods are not consulted.
//
// Prefer HasCode over comparing Error() strings or asserting on the dynamic
// type. AppError deliberately has no Is method: two errors with the same code
// are not errors.Is-equal.
func HasCode(err error, code ErrorCode) bool {
	for err != nil {
		if appErr, ok := err.(*AppError); ok && appErr != nil && appErr.ErrCode == code {
			return true
		}
		switch x := err.(type) {
		case interface{ Unwrap() error }:
			err = x.Unwrap()
		case interface{ Unwrap() []error }:
			for _, branch := range x.Unwrap() {
				if HasCode(branch, code) {
					return true
				}
			}
			return false
		default:
			return false
		}
	}
	return false
}
