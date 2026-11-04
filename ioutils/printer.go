package ioutils

import (
	"fmt"
	"io"
	"os"

	"github.com/trypanic/go-sdk/v2/errorkit"
	"github.com/trypanic/go-sdk/v2/marshal"
)

// WriteJSON encodes v as JSON to w with HTML escaping disabled, using the
// same encoding as marshal.NoEscapeBytes. The output ends with a newline,
// matching the standard library's json.Encoder.Encode behavior. v is encoded
// in full before anything is written, so an encode error writes nothing.
func WriteJSON(w io.Writer, v any) error {
	b, err := marshal.NoEscapeBytes(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("failed to write JSON"),
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
