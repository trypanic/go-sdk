package storage

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func newIntFileStorage(t *testing.T) (*FileStorage[int], string) {
	t.Helper()
	dir := t.TempDir()
	store, err := NewFileStorage[int](dir)
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	return store, dir
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func requireList(t *testing.T, store *FileStorage[int], key string, want []int) {
	t.Helper()
	got, ok, err := store.List(key)
	if err != nil || !ok {
		t.Fatalf("List(%q): ok=%v err=%v", key, ok, err)
	}
	if !slices.Equal(*got, want) {
		t.Fatalf("List(%q) = %v, want %v", key, *got, want)
	}
}

// TestFileStorageAppendWritesInPlace covers C4-017: every Append rewrote and
// re-indented the whole file. Appends to an existing file now add one line
// to the same file instead of replacing it.
func TestFileStorageAppendWritesInPlace(t *testing.T) {
	t.Parallel()

	store, dir := newIntFileStorage(t)
	path := filepath.Join(dir, fileNameForKey("k"))
	one, two := 1, 2
	if _, err := store.Append("k", &one); err != nil {
		t.Fatalf("Append: %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	list, err := store.Append("k", &two)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	if !os.SameFile(before, after) {
		t.Fatal("Append replaced the storage file instead of appending to it")
	}
	if got := readFile(t, path); got != "1\n2\n" {
		t.Fatalf("file content = %q, want JSON Lines %q", got, "1\n2\n")
	}
	if !slices.Equal(*list, []int{1, 2}) {
		t.Fatalf("Append returned %v, want [1 2]", *list)
	}
}

// TestFileStorageReadsLegacyArrayFormat checks that files written by earlier
// versions (one indented JSON array per key) stay readable and are converted
// on the next Append.
func TestFileStorageReadsLegacyArrayFormat(t *testing.T) {
	t.Parallel()

	store, dir := newIntFileStorage(t)
	legacy := filepath.Join(dir, legacyFileNameForKey("k"))
	writeFixture(t, legacy, "[\n  1,\n  2\n]")

	requireList(t, store, "k", []int{1, 2})
	all, err := store.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 || !slices.Equal(all["k"], []int{1, 2}) {
		t.Fatalf("GetAll = %v, want map[k:[1 2]]", all)
	}

	three := 3
	list, err := store.Append("k", &three)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if !slices.Equal(*list, []int{1, 2, 3}) {
		t.Fatalf("Append returned %v, want [1 2 3]", *list)
	}
	if got := readFile(t, filepath.Join(dir, fileNameForKey("k"))); got != "1\n2\n3\n" {
		t.Fatalf("converted file = %q, want %q", got, "1\n2\n3\n")
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy file still present after conversion: %v", err)
	}
	requireList(t, store, "k", []int{1, 2, 3})
}

func TestFileStorageLegacyCorruptAndEmptyFiles(t *testing.T) {
	t.Parallel()

	for name, content := range map[string]string{"corrupt": "[1,", "empty": ""} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store, dir := newIntFileStorage(t)
			writeFixture(t, filepath.Join(dir, legacyFileNameForKey("k")), content)
			_, _, err := store.List("k")
			requireCode(t, err, ERR_STORAGE_CORRUPTED)
		})
	}
}

// TestFileStorageCurrentFormatWinsOverLegacy covers a crash between writing
// the converted file and removing the legacy one.
func TestFileStorageCurrentFormatWinsOverLegacy(t *testing.T) {
	t.Parallel()

	store, dir := newIntFileStorage(t)
	writeFixture(t, filepath.Join(dir, legacyFileNameForKey("k")), "[1]")
	writeFixture(t, filepath.Join(dir, fileNameForKey("k")), "1\n2\n")

	requireList(t, store, "k", []int{1, 2})
	all, err := store.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all) != 1 || !slices.Equal(all["k"], []int{1, 2}) {
		t.Fatalf("GetAll = %v, want map[k:[1 2]]", all)
	}

	if err := store.Delete("k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, err := store.List("k"); ok || err != nil {
		t.Fatalf("List after Delete: ok=%v err=%v; Delete must remove both formats", ok, err)
	}
}

func TestFileStorageClearRemovesLegacyFiles(t *testing.T) {
	t.Parallel()

	store, dir := newIntFileStorage(t)
	writeFixture(t, filepath.Join(dir, legacyFileNameForKey("old")), "[1]")
	v := 1
	if _, err := store.Append("new", &v); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("Clear left %d files, want 0", len(entries))
	}
}

// TestFileStorageTornLastLine covers a crash in the middle of an append: the
// unterminated last line was never acknowledged, so it is ignored by reads
// and dropped by the next Append, which rewrites the file atomically.
func TestFileStorageTornLastLine(t *testing.T) {
	t.Parallel()

	store, dir := newIntFileStorage(t)
	path := filepath.Join(dir, fileNameForKey("k"))
	writeFixture(t, path, "1\n2\n{\"tr")

	requireList(t, store, "k", []int{1, 2})
	three := 3
	if _, err := store.Append("k", &three); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if got := readFile(t, path); got != "1\n2\n3\n" {
		t.Fatalf("file after Append = %q, want %q", got, "1\n2\n3\n")
	}
}

func TestFileStorageCorruptLines(t *testing.T) {
	t.Parallel()

	for name, content := range map[string]string{
		"bad line in the middle": "1\nnot-json\n3\n",
		"no complete line":       "1",
		"empty":                  "",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store, dir := newIntFileStorage(t)
			writeFixture(t, filepath.Join(dir, fileNameForKey("k")), content)
			_, _, err := store.List("k")
			requireCode(t, err, ERR_STORAGE_CORRUPTED)
			v := 4
			_, err = store.Append("k", &v)
			requireCode(t, err, ERR_STORAGE_CORRUPTED)
		})
	}
}
