package storage

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func storageFileMode(t *testing.T, dir, key string) fs.FileMode {
	t.Helper()
	info, err := os.Stat(filepath.Join(dir, fileNameForKey(key)))
	if err != nil {
		t.Fatalf("stat storage file: %v", err)
	}
	return info.Mode().Perm()
}

// TestFileStorageDefaultModesArePrivate covers C4-022: the directory was
// created 0o755 and data files 0o644, readable by every local account.
func TestFileStorageDefaultModesArePrivate(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}

	dir := filepath.Join(t.TempDir(), "store")
	store, err := NewFileStorage[int](dir)
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	v := 1
	if _, err := store.Append("k", &v); err != nil {
		t.Fatalf("Append: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("directory mode = %v, want no group/other access", perm)
	}
	if perm := storageFileMode(t, dir, "k"); perm != 0o600 {
		t.Fatalf("file mode = %v, want 0600", perm)
	}
}

func TestFileStorageModeOptions(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits")
	}

	dir := filepath.Join(t.TempDir(), "store")
	store, err := NewFileStorage[int](dir, WithDirMode(0o750), WithFileMode(0o640))
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	v := 1
	if _, err := store.Append("k", &v); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := store.Append("k", &v); err != nil {
		t.Fatalf("second Append: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	// MkdirAll applies the process umask, so only check that no bit beyond
	// the requested mode is set.
	if perm := info.Mode().Perm(); perm&^0o750 != 0 || perm&0o700 != 0o700 {
		t.Fatalf("directory mode = %v, want at most 0750 with full owner access", perm)
	}
	if perm := storageFileMode(t, dir, "k"); perm != 0o640 {
		t.Fatalf("file mode = %v, want 0640", perm)
	}
}
