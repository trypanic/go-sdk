package storage

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

// TestFileStorageLegacyKeyOf125Bytes covers the R-134 review: earlier
// versions accepted 125-byte keys (hex + ".json" = 255 bytes). The ".jsonl"
// name for such a key is 256 bytes, the probe failed with ENAMETOOLONG, and
// GetAll failed for the whole directory with a retriable ERR_STORAGE_ERROR.
func TestFileStorageLegacyKeyOf125Bytes(t *testing.T) {
	t.Parallel()

	key := strings.Repeat("a", legacyMaxFileKeyLength)
	if len(legacyFileNameForKey(key)) != 255 {
		t.Fatalf("fixture name is %d bytes, want 255", len(legacyFileNameForKey(key)))
	}
	newStoreWithLegacyKey := func(t *testing.T) (*FileStorage[int], string) {
		t.Helper()
		store, dir := newIntFileStorage(t)
		writeFixture(t, filepath.Join(dir, legacyFileNameForKey(key)), "[1,2]")
		v := 9
		if _, err := store.Append("short", &v); err != nil {
			t.Fatalf("Append short: %v", err)
		}
		return store, dir
	}

	t.Run("GetAll returns it", func(t *testing.T) {
		t.Parallel()
		store, _ := newStoreWithLegacyKey(t)
		all, err := store.GetAll()
		if err != nil {
			t.Fatalf("GetAll: %v", err)
		}
		if len(all) != 2 || !slices.Equal(all[key], []int{1, 2}) || !slices.Equal(all["short"], []int{9}) {
			t.Fatalf("GetAll = %v", all)
		}
	})

	t.Run("List reads it", func(t *testing.T) {
		t.Parallel()
		store, _ := newStoreWithLegacyKey(t)
		requireList(t, store, key, []int{1, 2})
	})

	t.Run("Delete removes it", func(t *testing.T) {
		t.Parallel()
		store, dir := newStoreWithLegacyKey(t)
		if err := store.Delete(key); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, legacyFileNameForKey(key))); !os.IsNotExist(err) {
			t.Fatalf("legacy file still present: %v", err)
		}
	})

	t.Run("Clear removes it", func(t *testing.T) {
		t.Parallel()
		store, dir := newStoreWithLegacyKey(t)
		if err := store.Clear(); err != nil {
			t.Fatalf("Clear: %v", err)
		}
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Fatalf("Clear left %d files", len(entries))
		}
	})

	t.Run("Append explains the limit", func(t *testing.T) {
		t.Parallel()
		store, _ := newStoreWithLegacyKey(t)
		v := 3
		_, err := store.Append(key, &v)
		appErr := requireCode(t, err, errorkit.ERR_VALIDATION)
		if appErr.Metadata.Retriable {
			t.Fatal("over-long key must not be retriable")
		}
		if !strings.Contains(appErr.Error(), "124") {
			t.Fatalf("error does not name the limit: %v", appErr)
		}
		requireList(t, store, key, []int{1, 2})
	})
}
