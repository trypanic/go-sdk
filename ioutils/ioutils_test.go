package ioutils

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestWriteJSONEncodesWithoutHTMLEscape(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := WriteJSON(&buf, map[string]string{"q": "<a>&"}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`<a>&`)) {
		t.Fatalf("expected raw <a>& in output, got %s", buf.String())
	}
}

func TestWriteJSONReturnsErrorOnUnsupportedType(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := WriteJSON(&buf, make(chan int))
	if err == nil {
		t.Fatalf("expected error encoding chan")
	}
	var appErr *errorkit.AppError
	if !errors.As(err, &appErr) || appErr.Code() != errorkit.ERR_INTERNAL {
		t.Fatalf("expected ERR_INTERNAL, got %v", err)
	}
}

func TestSaveJSONWritesFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	if err := SaveJSON(path, map[string]int{"n": 1}); err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}
}

func TestSaveJSONReturnsErrorOnInvalidPath(t *testing.T) {
	t.Parallel()

	err := SaveJSON("/dev/null/does-not-exist/out.json", map[string]int{"n": 1})
	if err == nil {
		t.Fatalf("expected error on invalid path")
	}
	var appErr *errorkit.AppError
	if !errors.As(err, &appErr) || appErr.Code() != errorkit.ERR_INTERNAL {
		t.Fatalf("expected ERR_INTERNAL, got %v", err)
	}
}
