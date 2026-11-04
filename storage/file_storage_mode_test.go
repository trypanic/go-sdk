package storage

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/trypanic/go-sdk/v2/errorkit"
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

// TestFileStorageRejectsModesWithoutOwnerAccess: a file mode without owner
// read/write, or a directory mode without owner rwx, would make the store
// unusable by its own process.
func TestFileStorageRejectsModesWithoutOwnerAccess(t *testing.T) {
	t.Parallel()

	tests := map[string]FileStorageOption{
		"file mode 0":    WithFileMode(0),
		"file mode 0400": WithFileMode(0o400),
		"file mode 0200": WithFileMode(0o200),
		"dir mode 0":     WithDirMode(0),
		"dir mode 0600":  WithDirMode(0o600),
	}
	for name, opt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "store")
			_, err := NewFileStorage[int](dir, opt)
			appErr := requireCode(t, err, errorkit.ERR_SYSTEM_CONFIG_INVALID)
			if appErr.Metadata.Retriable {
				t.Fatal("configuration errors must not be retriable")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("directory created despite the invalid option: %v", err)
			}
		})
	}
}
