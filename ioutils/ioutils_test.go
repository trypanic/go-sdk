package ioutils

import (
	"bytes"
	"errors"
	"os"
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

// SaveJSON used to truncate the target with os.Create before encoding, so a
// failed encode destroyed the previous dump.
func TestSaveJSONKeepsExistingFileOnEncodeError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "out.json")
	if err := os.WriteFile(path, []byte(`{"keep":"me"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	err := SaveJSON(path, make(chan int))
	if appErr, ok := errors.AsType[*errorkit.AppError](err); !ok || appErr.Code() != errorkit.ERR_INTERNAL {
		t.Fatalf("expected ERR_INTERNAL, got %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"keep":"me"}` {
		t.Fatalf("existing file content = %q, want it unchanged", got)
	}
}

// Dumps may contain request bodies or credentials; new files are created
// owner-only instead of 0666 &^ umask.
func TestSaveJSONCreatesOwnerOnlyFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "out.json")
	if err := SaveJSON(path, map[string]int{"n": 1}); err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file mode = %v, want 0600", perm)
	}
}
