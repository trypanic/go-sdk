package ioutils

import (
	"bytes"
	"os"

	"github.com/trypanic/go-sdk/v2/errorkit"
)

// SaveJSON writes v as JSON to filename. HTML escaping is disabled.
//
// v is encoded into memory first, so an encode error leaves an existing file
// untouched. On success an existing file is truncated and overwritten (its
// permissions are kept); a new file is created with mode 0600 because dumps
// may contain request bodies or credentials. filename is used as given, with
// no validation. The file is closed before SaveJSON returns.
//
// Convenience wrapper around WriteJSON; intended for development and debug
// dumps, not for production storage. Use the `storage` package for managed
// persistence.
func SaveJSON(filename string, v any) (retErr error) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, v); err != nil {
		return err
	}

	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("error creating file"),
			errorkit.WithWrapped(err),
		)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil && retErr == nil {
			retErr = errorkit.NewError(errorkit.ERR_INTERNAL).With(
				errorkit.WithReason("error closing file"),
				errorkit.WithWrapped(cerr),
			)
		}
	}()

	if _, err := file.Write(buf.Bytes()); err != nil {
		return errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("error writing file"),
			errorkit.WithWrapped(err),
		)
	}
	return nil
}

// SaveProductsToJSON is the legacy name for SaveJSON, retained for source
// compatibility within the SDK. Prefer SaveJSON.
//
// Deprecated: use SaveJSON.
func SaveProductsToJSON(filename string, products any) error {
	return SaveJSON(filename, products)
}
