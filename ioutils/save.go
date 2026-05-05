package ioutils

import (
	"os"

	"github.com/trypanic/go-sdk/errorkit"
)

// SaveJSON writes v as JSON to filename, truncating any existing file.
// The file is closed before SaveJSON returns. HTML escaping is disabled.
//
// Convenience wrapper around WriteJSON; intended for development and debug
// dumps, not for production storage. Use the `storage` package for managed
// persistence.
func SaveJSON(filename string, v any) (retErr error) {
	file, err := os.Create(filename)
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

	return WriteJSON(file, v)
}

// SaveProductsToJSON is the legacy name for SaveJSON, retained for source
// compatibility within the SDK. Prefer SaveJSON.
//
// Deprecated: use SaveJSON.
func SaveProductsToJSON(filename string, products any) error {
	return SaveJSON(filename, products)
}
