package storage

import (
	"errors"
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

// Thread-safety smoke test.
func TestConcurrentAccess(t *testing.T) {
	ms := NewMemory[int]()

	var wg sync.WaitGroup
	wg.Add(100)

	for i := 0; i < 100; i++ {
		go func(i int) {
			defer wg.Done()
			_ = ms.Put("single", &i)
			_, _ = ms.Append("list", &i)
		}(i)
	}

	wg.Wait()

	if _, ok, _ := ms.Get("single"); !ok {
		t.Fatalf("single value missing after concurrent writes")
	}
	list, ok, _ := ms.List("list")
	if !ok {
		t.Fatalf("list missing after concurrent writes")
	}
	if len(*list) == 0 {
		t.Fatalf("expected appended list to have elements")
	}
}

func TestNewFileStorage_InvalidPath_ReturnsAppError(t *testing.T) {
	_, err := NewFileStorage[int]("/dev/null/invalid")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var appErr *errorkit.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *errorkit.AppError, got %T", err)
	}
	if appErr.Code() != ERR_STORAGE_ERROR {
		t.Fatalf("expected code %s, got %s", ERR_STORAGE_ERROR, appErr.Code())
	}
}
