package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trypanic/go-sdk/v2/errorkit"
)

func requireCode(t *testing.T, err error, want errorkit.ErrorCode) *errorkit.AppError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s, got nil", want)
	}
	appErr, ok := errors.AsType[*errorkit.AppError](err)
	if !ok {
		t.Fatalf("expected *errorkit.AppError, got %T: %v", err, err)
	}
	if appErr.Code() != want {
		t.Fatalf("code = %s, want %s (err: %v)", appErr.Code(), want, err)
	}
	return appErr
}

// TestStorageErrorCodes is the regression test for C4-018: every storage
// failure used to be the retriable ERR_STORAGE_ERROR (503).
func TestStorageErrorCodes(t *testing.T) {
	t.Parallel()

	t.Run("MemoryStorage.Put(nil) is a validation error", func(t *testing.T) {
		t.Parallel()
		appErr := requireCode(t, NewMemory[int]().Put("k", nil), errorkit.ERR_VALIDATION_MISSING_FIELD)
		if appErr.Metadata.Retriable {
			t.Fatal("nil input must not be retriable")
		}
	})

	for name, newStore := range appendLogImplementations() {
		t.Run(name+".Append(nil) is a validation error", func(t *testing.T) {
			t.Parallel()
			_, err := newStore(t).Append("k", nil)
			_ = requireCode(t, err, errorkit.ERR_VALIDATION_MISSING_FIELD)
		})
	}

	t.Run("over-long FileStorage key is a validation error", func(t *testing.T) {
		t.Parallel()
		fs, err := NewFileStorage[int](t.TempDir())
		if err != nil {
			t.Fatalf("NewFileStorage: %v", err)
		}
		key := strings.Repeat("k", MaxFileKeyLength+1)
		v := 1
		_, err = fs.Append(key, &v)
		appErr := requireCode(t, err, errorkit.ERR_VALIDATION)
		if appErr.Metadata.Retriable {
			t.Fatal("over-long key must not be retriable")
		}
		// List and Delete also accept the 125-byte keys that earlier
		// versions wrote (see TestFileStorageLegacyKeyOf125Bytes); one byte
		// more is rejected everywhere.
		tooLong := strings.Repeat("k", legacyMaxFileKeyLength+1)
		_, err = fs.Append(tooLong, &v)
		_ = requireCode(t, err, errorkit.ERR_VALIDATION)
		_, _, err = fs.List(tooLong)
		_ = requireCode(t, err, errorkit.ERR_VALIDATION)
		_ = requireCode(t, fs.Delete(tooLong), errorkit.ERR_VALIDATION)
	})

	t.Run("longest accepted FileStorage key works", func(t *testing.T) {
		t.Parallel()
		fs, err := NewFileStorage[int](t.TempDir())
		if err != nil {
			t.Fatalf("NewFileStorage: %v", err)
		}
		key := strings.Repeat("k", MaxFileKeyLength)
		v := 1
		if _, err := fs.Append(key, &v); err != nil {
			t.Fatalf("Append with a %d-byte key: %v", MaxFileKeyLength, err)
		}
		if _, ok, err := fs.List(key); err != nil || !ok {
			t.Fatalf("List with a %d-byte key: ok=%v err=%v", MaxFileKeyLength, ok, err)
		}
	})

	t.Run("corrupt file is non-retriable", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		fs, err := NewFileStorage[int](dir)
		if err != nil {
			t.Fatalf("NewFileStorage: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, fileNameForKey("k")), []byte("not-json"), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		_, _, err = fs.List("k")
		appErr := requireCode(t, err, ERR_STORAGE_CORRUPTED)
		if appErr.Metadata.Retriable {
			t.Fatal("corruption must not be retriable")
		}
		_, err = fs.GetAll()
		_ = requireCode(t, err, ERR_STORAGE_CORRUPTED)
	})

	t.Run("permission denied is ERR_STORAGE_ACCESS_DENIED", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		fs, err := NewFileStorage[int](dir)
		if err != nil {
			t.Fatalf("NewFileStorage: %v", err)
		}
		v := 1
		if _, err := fs.Append("k", &v); err != nil {
			t.Fatalf("seed Append: %v", err)
		}
		// Append writes the key's file in place (O_APPEND) and Delete
		// unlinks it from the directory: make both read-only.
		if err := os.Chmod(filepath.Join(dir, fileNameForKey("k")), 0o400); err != nil {
			t.Skipf("chmod unsupported: %v", err)
		}
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Skipf("chmod unsupported: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
		if probe, err := os.CreateTemp(dir, "probe"); err == nil {
			_ = probe.Close()
			t.Skip("directory is still writable (running as root?)")
		}

		_, err = fs.Append("k", &v)
		_ = requireCode(t, err, ERR_STORAGE_ACCESS_DENIED)
		_ = requireCode(t, fs.Delete("k"), ERR_STORAGE_ACCESS_DENIED)
	})
}
