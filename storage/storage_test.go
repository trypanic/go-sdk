package storage

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

func TestPutAndGetSingleValue(t *testing.T) {
	ms := NewMemory[int]()

	val := 42
	if err := ms.Put("answer", &val); err != nil {
		t.Fatalf("Put: %v", err)
	}

	result, ok, err := ms.Get("answer")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatalf("expected key to exist")
	}
	if *result != 42 {
		t.Fatalf("expected 42, got %d", *result)
	}
}

func TestPutOverwritesValue(t *testing.T) {
	ms := NewMemory[int]()

	val1 := 1
	val2 := 2
	_ = ms.Put("key", &val1)
	_ = ms.Put("key", &val2)

	result, _, _ := ms.Get("key")
	if *result != 2 {
		t.Fatalf("expected overwritten value 2, got %d", *result)
	}
}

func TestAppendAndListAccumulates(t *testing.T) {
	ms := NewMemory[int]()

	v1 := 10
	v2 := 20
	if _, err := ms.Append("numbers", &v1); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := ms.Append("numbers", &v2); err != nil {
		t.Fatalf("Append: %v", err)
	}

	list, ok, err := ms.List("numbers")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !ok {
		t.Fatalf("expected list to exist")
	}
	if len(*list) != 2 {
		t.Fatalf("expected slice length 2, got %d", len(*list))
	}
	if (*list)[0] != 10 || (*list)[1] != 20 {
		t.Fatalf("unexpected slice values: %+v", *list)
	}
}

func TestAppendCreatesNewSlice(t *testing.T) {
	ms := NewMemory[string]()

	v := "hello"
	if _, err := ms.Append("greetings", &v); err != nil {
		t.Fatalf("Append: %v", err)
	}

	list, ok, _ := ms.List("greetings")
	if !ok {
		t.Fatalf("expected list to exist")
	}
	if len(*list) != 1 || (*list)[0] != "hello" {
		t.Fatalf("unexpected stored value: %+v", *list)
	}
}

func TestGetMissingKey(t *testing.T) {
	ms := NewMemory[int]()

	if _, ok, _ := ms.Get("missing"); ok {
		t.Fatalf("expected Get to return false for missing keys")
	}
	if _, ok, _ := ms.List("missing"); ok {
		t.Fatalf("expected List to return false for missing keys")
	}
}

func TestDelete(t *testing.T) {
	ms := NewMemory[int]()

	v1 := 1
	v2 := 2
	_ = ms.Put("single", &v1)
	_, _ = ms.Append("list", &v2)

	if err := ms.Delete("single"); err != nil {
		t.Fatalf("Delete single: %v", err)
	}
	if err := ms.Delete("list"); err != nil {
		t.Fatalf("Delete list: %v", err)
	}

	if _, ok, _ := ms.Get("single"); ok {
		t.Fatalf("expected single key to be deleted")
	}
	if _, ok, _ := ms.List("list"); ok {
		t.Fatalf("expected list key to be deleted")
	}
}

func TestClear(t *testing.T) {
	ms := NewMemory[int]()

	v := 100
	_ = ms.Put("a", &v)
	_, _ = ms.Append("b", &v)

	if err := ms.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	if _, ok, _ := ms.Get("a"); ok {
		t.Fatalf("expected storage to be cleared for single values")
	}
	if _, ok, _ := ms.List("b"); ok {
		t.Fatalf("expected storage to be cleared for slices")
	}
}

// Thread-safety test: 100 concurrent writers, and every append must land
// exactly once (C4-023: the test used to accept any non-empty list).
func TestConcurrentAccess(t *testing.T) {
	ms := NewMemory[int]()

	const writers = 100
	var wg sync.WaitGroup
	for i := range writers {
		wg.Go(func() {
			if err := ms.Put("single", &i); err != nil {
				t.Errorf("Put: %v", err)
			}
			if _, err := ms.Append("list", &i); err != nil {
				t.Errorf("Append: %v", err)
			}
		})
	}
	wg.Wait()

	if _, ok, _ := ms.Get("single"); !ok {
		t.Fatalf("single value missing after concurrent writes")
	}
	list, ok, _ := ms.List("list")
	if !ok {
		t.Fatalf("list missing after concurrent writes")
	}
	assertEachOnce(t, *list, writers)
}

// assertEachOnce checks that items holds exactly the values 0..n-1, each once.
func assertEachOnce(t *testing.T, items []int, n int) {
	t.Helper()
	if len(items) != n {
		t.Fatalf("got %d items, want exactly %d", len(items), n)
	}
	seen := make(map[int]bool, n)
	for _, v := range items {
		if v < 0 || v >= n || seen[v] {
			t.Fatalf("unexpected or duplicate item %d in %v", v, items)
		}
		seen[v] = true
	}
}

func TestNewFileStorage_InvalidPath_ReturnsAppError(t *testing.T) {
	// A path below a regular file cannot be created on any OS.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := NewFileStorage[int](filepath.Join(file, "sub"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	appErr, ok := errors.AsType[*errorkit.AppError](err)
	if !ok {
		t.Fatalf("expected *errorkit.AppError, got %T", err)
	}
	if appErr.Code() != ERR_STORAGE_ERROR {
		t.Fatalf("expected code %s, got %s", ERR_STORAGE_ERROR, appErr.Code())
	}
}

// TestListDoesNotAliasInternalBuffer guards the defensive copy: a list returned
// by Append/List must not change when a later Append grows the internal slice.
func TestListDoesNotAliasInternalBuffer(t *testing.T) {
	ms := NewMemory[int]()

	a, b := 1, 2
	first, err := ms.Append("k", &a)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if len(*first) != 1 || (*first)[0] != 1 {
		t.Fatalf("first snapshot = %v, want [1]", *first)
	}

	if _, err := ms.Append("k", &b); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// The earlier snapshot must be untouched by the second Append.
	if len(*first) != 1 || (*first)[0] != 1 {
		t.Fatalf("snapshot aliased internal buffer: %v, want [1]", *first)
	}

	got, _, err := ms.List("k")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	(*got)[0] = 99 // mutating the returned copy must not corrupt storage
	again, _, _ := ms.List("k")
	if (*again)[0] == 99 {
		t.Fatalf("List returned an aliased slice; storage was corrupted by caller")
	}
}

func TestPutAndAppendRejectNil(t *testing.T) {
	ms := NewMemory[int]()

	if err := ms.Put("k", nil); err == nil {
		t.Fatal("Put(nil) expected error, got nil")
	}
	if _, err := ms.Append("k", nil); err == nil {
		t.Fatal("Append(nil) expected error, got nil")
	}
}
