package storage

import (
	"errors"
	"io/fs"

	"github.com/trypanic/go-sdk/errorkit"
)

// ioError classifies a filesystem failure: permission problems become the
// non-retriable ERR_STORAGE_ACCESS_DENIED, everything else ERR_STORAGE_ERROR.
// The cause is always wrapped so errors.Is/errors.As keep working.
func ioError(reason string, err error) *errorkit.AppError {
	code := ERR_STORAGE_ERROR
	if errors.Is(err, fs.ErrPermission) {
		code = ERR_STORAGE_ACCESS_DENIED
	}
	return errorkit.NewError(code).With(
		errorkit.WithReason("%s", reason),
		errorkit.WithWrapped(err),
	)
}

// corruptError reports stored data that cannot be decoded. err may be nil.
func corruptError(reason string, err error) *errorkit.AppError {
	appErr := errorkit.NewError(ERR_STORAGE_CORRUPTED).With(errorkit.WithReason("%s", reason))
	if err != nil {
		appErr = appErr.With(errorkit.WithWrapped(err))
	}
	return appErr
}

// nilInputError reports a nil value or item passed by the caller.
func nilInputError(reason string) *errorkit.AppError {
	return errorkit.NewError(errorkit.ERR_VALIDATION_MISSING_FIELD).With(
		errorkit.WithReason("%s", reason),
	)
}

// validateFileKey rejects keys that Append cannot write: the encoded
// ".jsonl" name would exceed the 255-byte file-name limit of common
// filesystems.
func validateFileKey(key string) error {
	switch {
	case len(key) <= MaxFileKeyLength:
		return nil
	case len(key) <= legacyMaxFileKeyLength:
		return errorkit.NewError(errorkit.ERR_VALIDATION).With(
			errorkit.WithReason("key is %d bytes long; FileStorage writes keys of at most %d bytes "+
				"(keys of %d bytes written by earlier versions can still be listed and deleted)",
				len(key), MaxFileKeyLength, legacyMaxFileKeyLength),
		)
	default:
		return errorkit.NewError(errorkit.ERR_VALIDATION).With(
			errorkit.WithReason("key is %d bytes long; FileStorage accepts at most %d", len(key), MaxFileKeyLength),
		)
	}
}

// validateReadKey rejects keys that no version of FileStorage could have
// written. List and Delete accept the legacy limit so that keys written by
// earlier versions stay reachable.
func validateReadKey(key string) error {
	if len(key) > legacyMaxFileKeyLength {
		return errorkit.NewError(errorkit.ERR_VALIDATION).With(
			errorkit.WithReason("key is %d bytes long; FileStorage accepts at most %d (%d for keys written by earlier versions)",
				len(key), MaxFileKeyLength, legacyMaxFileKeyLength),
		)
	}
	return nil
}
