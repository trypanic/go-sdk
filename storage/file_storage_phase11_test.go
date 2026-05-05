package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestFileStorageAppendReturnsErrorOnUnwritableDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fs, err := NewFileStorage[int](dir)
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skipf("chmod unsupported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	v := 1
	_, err = fs.Append("k", &v)
	if err == nil {
		t.Fatalf("Append must return error when target dir is read-only")
	}
	var appErr *errorkit.AppError
	if !errors.As(err, &appErr) || appErr.Code() != ERR_STORAGE_ERROR {
		t.Fatalf("expected ERR_STORAGE_ERROR, got %v", err)
	}
}

func TestFileStorageGetReturnsErrorOnCorruptJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fs, err := NewFileStorage[int](dir)
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileNameForKey("k")), []byte("not-json"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, _, err = fs.List("k")
	if err == nil {
		t.Fatalf("List must return error on corrupt JSON")
	}
	var appErr *errorkit.AppError
	if !errors.As(err, &appErr) || appErr.Code() != ERR_STORAGE_ERROR {
		t.Fatalf("expected ERR_STORAGE_ERROR, got %v", err)
	}
}

func TestFileStorageDeleteReturnsErrorWhenRemovalFails(t *testing.T) {
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
	// Make the parent directory read-only so unlink fails.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Skipf("chmod unsupported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err = fs.Delete("k")
	if err == nil {
		t.Fatalf("Delete must return error when unlink fails")
	}
	var appErr *errorkit.AppError
	if !errors.As(err, &appErr) || appErr.Code() != ERR_STORAGE_ERROR {
		t.Fatalf("expected ERR_STORAGE_ERROR, got %v", err)
	}
}

func TestFileStorageDeleteMissingKeyIsNotAnError(t *testing.T) {
	t.Parallel()

	fs, err := NewFileStorage[int](t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	if err := fs.Delete("never-existed"); err != nil {
		t.Fatalf("Delete on missing key must be a no-op, got %v", err)
	}
}

func TestFileStorageKeysWithDifferentSpecialCharsDoNotCollide(t *testing.T) {
	t.Parallel()

	fs, err := NewFileStorage[string](t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	a := "value-a"
	b := "value-b"
	if _, err := fs.Append("a/b", &a); err != nil {
		t.Fatalf("Append a/b: %v", err)
	}
	if _, err := fs.Append("a:b", &b); err != nil {
		t.Fatalf("Append a:b: %v", err)
	}

	gotA, ok, err := fs.List("a/b")
	if err != nil || !ok {
		t.Fatalf("List a/b: ok=%v err=%v", ok, err)
	}
	gotB, ok, err := fs.List("a:b")
	if err != nil || !ok {
		t.Fatalf("List a:b: ok=%v err=%v", ok, err)
	}
	if (*gotA)[0] != "value-a" || (*gotB)[0] != "value-b" {
		t.Fatalf("collision: got a/b=%v a:b=%v", gotA, gotB)
	}
}

func TestFileStorageGetAllRoundTripsKeys(t *testing.T) {
	t.Parallel()

	fs, err := NewFileStorage[int](t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	v := 1
	if _, err := fs.Append("a/b", &v); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := fs.Append("a:b", &v); err != nil {
		t.Fatalf("Append: %v", err)
	}

	all, err := fs.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if _, ok := all["a/b"]; !ok {
		t.Fatalf("GetAll lost original key a/b: %v", all)
	}
	if _, ok := all["a:b"]; !ok {
		t.Fatalf("GetAll lost original key a:b: %v", all)
	}
}

// Compile-time interface conformance: split interfaces.
var (
	_ KVStorager[int]        = (*MemoryStorage[int])(nil)
	_ AppendLogStorager[int] = (*MemoryStorage[int])(nil)
	_ AppendLogStorager[int] = (*FileStorage[int])(nil)
)
