// Package ioutils provides developer-only JSON dump helpers. None of these
// functions are intended for production data paths; reach for `storage` or
// `marshal` directly when persistence or wire encoding matters.
package ioutils

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/trypanic/go-sdk/errorkit"
)

// WriteJSON encodes v as JSON to w with HTML escaping disabled. The encoder
// writes a trailing newline, matching the standard library's
// json.Encoder.Encode behavior.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("failed to encode JSON"),
			errorkit.WithWrapped(err),
		)
	}
	return nil
}

// PrintJSON dumps v to stdout between two banner lines. Convenience wrapper
// around WriteJSON for ad-hoc REPL-style debugging. Errors are silently
// discarded; do not use in production.
func PrintJSON(v any) {
	// nosemgrep: no-fmt-print-in-library -- PrintJSON is a documented stdout debug helper.
	fmt.Println("================================")
	_ = WriteJSON(os.Stdout, v)
	// nosemgrep: no-fmt-print-in-library -- PrintJSON is a documented stdout debug helper.
	fmt.Println("================================")
}
