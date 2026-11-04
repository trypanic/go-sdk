package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestFileStorageNeverWritesThroughSymlinkAtFinalPath is the regression test
// for the R-134 review: the in-place append opened <hex>.jsonl with O_APPEND,
// which follows a symlink planted at the final path and appended to its
// target.
func TestFileStorageNeverWritesThroughSymlinkAtFinalPath(t *testing.T) {
	t.Parallel()

	store, dir := newIntFileStorage(t)
	victim := filepath.Join(t.TempDir(), "victim.jsonl")
	writeFixture(t, victim, "100\n200\n")
	link := filepath.Join(dir, fileNameForKey("k"))
	if err := os.Symlink(victim, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	// The link is not a storage file: its target is neither read nor written.
	if list, ok, err := store.List("k"); ok || err != nil {
		t.Fatalf("List through a symlink = (%v, %v, %v), want (nil, false, nil)", list, ok, err)
	}
	v := 1
	got, err := store.Append("k", &v)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if len(*got) != 1 || (*got)[0] != 1 {
		t.Fatalf("Append returned %v, want [1]", *got)
	}

	if content := readFile(t, victim); content != "100\n200\n" {
		t.Fatalf("symlink target was modified: %q", content)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("final path is still %v, want the link replaced by a regular file", info.Mode())
	}
	requireList(t, store, "k", []int{1})
}

// TestFileStorageNeverReadsThroughSymlinkAtLegacyPath covers the legacy
// array file: a symlink there is not read either.
func TestFileStorageNeverReadsThroughSymlinkAtLegacyPath(t *testing.T) {
	t.Parallel()

	store, dir := newIntFileStorage(t)
	victim := filepath.Join(t.TempDir(), "victim.json")
	writeFixture(t, victim, "[100, 200]")
	if err := os.Symlink(victim, filepath.Join(dir, legacyFileNameForKey("k"))); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	if list, ok, err := store.List("k"); ok || err != nil {
		t.Fatalf("List through a symlink = (%v, %v, %v), want (nil, false, nil)", list, ok, err)
	}
	all, err := store.GetAll()
	if err != nil || len(all) != 0 {
		t.Fatalf("GetAll through a symlink = (%v, %v), want empty", all, err)
	}
}

// TestAppendLineRefusesSymlink covers the window between readKey and the
// in-place append: if a symlink is swapped in, appendLine writes nothing and
// reports errNotRegular so Append falls back to the atomic rewrite.
func TestAppendLineRefusesSymlink(t *testing.T) {
	t.Parallel()

	victim := filepath.Join(t.TempDir(), "victim.jsonl")
	writeFixture(t, victim, "100\n")
	link := filepath.Join(t.TempDir(), "link.jsonl")
	if err := os.Symlink(victim, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	if err := appendLine(link, []byte("1")); !errors.Is(err, errNotRegular) {
		t.Fatalf("appendLine through a symlink = %v, want errNotRegular", err)
	}
	if content := readFile(t, victim); content != "100\n" {
		t.Fatalf("symlink target was modified: %q", content)
	}
}
