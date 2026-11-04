package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/trypanic/go-sdk/v2/errorkit"
)

// TestFileStorageConcurrentInstancesKeepValidJSON is the regression test for
// C4-012: two FileStorage values on one directory used to share the fixed
// "<file>.tmp" path, interleave their writes and leave invalid JSON behind.
func TestFileStorageConcurrentInstancesKeepValidJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first, err := NewFileStorage[int](dir)
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	second, err := NewFileStorage[int](dir)
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}

	const appends = 200
	var (
		wg       sync.WaitGroup
		errMu    sync.Mutex
		failures []error
	)
	for i := range appends {
		store := first
		if i%2 == 1 {
			store = second
		}
		wg.Go(func() {
			if _, err := store.Append("k", &i); err != nil {
				errMu.Lock()
				failures = append(failures, err)
				errMu.Unlock()
			}
		})
	}
	wg.Wait()

	if len(failures) > 0 {
		t.Fatalf("%d of %d appends failed; first: %v", len(failures), appends, failures[0])
	}
	list, ok, err := first.List("k")
	if err != nil || !ok {
		t.Fatalf("List after concurrent appends: ok=%v err=%v", ok, err)
	}
	if len(*list) == 0 || len(*list) > appends {
		t.Fatalf("List returned %d items, want 1..%d", len(*list), appends)
	}
	assertNoTempFiles(t, dir)
}

// TestFileStorageAppendDoesNotFollowSymlinkAtTempPath is the regression test
// for C4-013: a symlink planted at the predictable temp path must not be
// followed.
func TestFileStorageAppendDoesNotFollowSymlinkAtTempPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim.txt")
	if err := os.WriteFile(victim, []byte("original"), 0o600); err != nil {
		t.Fatalf("write victim: %v", err)
	}
	fs, err := NewFileStorage[string](dir)
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	if err := os.Symlink(victim, filepath.Join(dir, fileNameForKey("k")+".tmp")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	item := "attacker-controlled"
	if _, err := fs.Append("k", &item); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("read victim: %v", err)
	}
	if string(got) != "original" {
		t.Fatalf("symlink target was overwritten: %q", got)
	}
}

// TestFileStorageEmptyFileIsCorruption covers C4-016: a zero-length file (for
// example after a crash) must be reported, not read as an empty list that
// the next Append silently overwrites.
func TestFileStorageEmptyFileIsCorruption(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fs, err := NewFileStorage[int](dir)
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileNameForKey("k")), nil, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, _, err := fs.List("k"); err == nil {
		t.Fatal("List on a zero-length file must return an error")
	}
	v := 1
	if _, err := fs.Append("k", &v); err == nil {
		t.Fatal("Append on a zero-length file must return an error")
	}
	_, _, err = fs.List("k")
	if _, ok := errors.AsType[*errorkit.AppError](err); !ok {
		t.Fatalf("expected *errorkit.AppError, got %T", err)
	}
}

func TestFileStorageAppendLeavesNoTempFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fs, err := NewFileStorage[int](dir)
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	for i := range 5 {
		if _, err := fs.Append("k", &i); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	assertNoTempFiles(t, dir)

	data, err := os.ReadFile(filepath.Join(dir, fileNameForKey("k")))
	if err != nil {
		t.Fatalf("read storage file: %v", err)
	}
	// JSON Lines: every line is one valid JSON value, and the file ends
	// with a newline.
	if !bytes.HasSuffix(data, []byte("\n")) {
		t.Fatalf("storage file does not end with a newline: %q", data)
	}
	lines := 0
	for line := range bytes.Lines(data) {
		lines++
		if !json.Valid(line) {
			t.Fatalf("storage file line %d is not valid JSON: %q", lines, line)
		}
	}
	if lines != 5 {
		t.Fatalf("storage file has %d lines, want 5: %q", lines, data)
	}
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "tmp") {
			t.Fatalf("temporary file left behind: %s", entry.Name())
		}
	}
}

// TestFileStorageClearKeepsForeignFiles covers C4-014: Clear removed every
// *.json file in baseDir, including files the store did not create. It now
// removes exactly the files GetAll reports.
func TestFileStorageClearKeepsForeignFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fs, err := NewFileStorage[int](dir)
	if err != nil {
		t.Fatalf("NewFileStorage: %v", err)
	}
	foreign := []string{"config.json", "notes.txt", "abc.json"}
	for _, name := range foreign {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{}`), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	for _, key := range []string{"a", "b/c"} {
		v := 1
		if _, err := fs.Append(key, &v); err != nil {
			t.Fatalf("Append %q: %v", key, err)
		}
	}

	if err := fs.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	all, err := fs.GetAll()
	if err != nil {
		t.Fatalf("GetAll after Clear: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("GetAll after Clear = %v, want empty", all)
	}
	for _, name := range foreign {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("foreign file %s removed by Clear: %v", name, err)
		}
	}
}
