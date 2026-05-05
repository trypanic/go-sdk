package marshal

import (
	"bytes"
	"encoding/json"

	"github.com/trypanic/go-sdk/errorkit"
)

func NoEscape(v any) (string, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)

	err := encoder.Encode(v)
	if err != nil {
		return "", errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("failed to marshal JSON without escaping"),
			errorkit.WithWrapped(err),
		)
	}

	return buf.String(), nil
}

func NoEscapeBytes(v any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)

	err := encoder.Encode(v)
	if err != nil {
		return nil, errorkit.NewError(errorkit.ERR_INTERNAL).With(
			errorkit.WithReason("failed to marshal JSON without escaping"),
			errorkit.WithWrapped(err),
		)
	}

	return buf.Bytes(), nil
}

// NoEscapeNoNewline is NoEscape with the trailing newline stripped.
// Use it when the consumer requires the JSON document with no extra bytes
// (e.g. when concatenating onto another payload).
func NoEscapeNoNewline(v any) (string, error) {
	b, err := NoEscapeBytesNoNewline(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// NoEscapeBytesNoNewline is NoEscapeBytes with the trailing newline stripped.
func NoEscapeBytesNoNewline(v any) ([]byte, error) {
	b, err := NoEscapeBytes(v)
	if err != nil {
		return nil, err
	}
	if n := len(b); n > 0 && b[n-1] == '\n' {
		b = b[:n-1]
	}
	return b, nil
}
