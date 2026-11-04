package storage

import (
	"errors"
	"sync"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

// appendLogFactory builds an empty AppendLog for one test.
type appendLogFactory func(t *testing.T) AppendLog[int]

// appendLogImplementations lists every AppendLog in the package so
// the conformance tests run against all of them.
func appendLogImplementations() map[string]appendLogFactory {
	return map[string]appendLogFactory{
		"MemoryStorage": func(*testing.T) AppendLog[int] {
			return NewMemory[int]()
		},
		"FileStorage": func(t *testing.T) AppendLog[int] {
			t.Helper()
			fs, err := NewFileStorage[int](t.TempDir())
			if err != nil {
				t.Fatalf("NewFileStorage: %v", err)
			}
			return fs
		},
	}
}

// TestAppendLogConformance_AppendNilReturnsError is the regression test for
// C4-015: FileStorage.Append(nil) panicked while MemoryStorage returned an
// error.
func TestAppendLogConformance_AppendNilReturnsError(t *testing.T) {
	t.Parallel()

	for name, newStore := range appendLogImplementations() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store := newStore(t)
			list, err := store.Append("k", nil)
			if err == nil {
				t.Fatal("Append(nil) expected error, got nil")
			}
			if list != nil {
				t.Fatalf("Append(nil) list = %v, want nil", *list)
			}
			if _, ok := errors.AsType[*errorkit.AppError](err); !ok {
				t.Fatalf("expected *errorkit.AppError, got %T", err)
			}
			if _, ok, err := store.List("k"); ok || err != nil {
				t.Fatalf("Append(nil) must not create the key: ok=%v err=%v", ok, err)
			}
		})
	}
}

// TestAppendLogConformance runs the AppendLog contract against every
// implementation (C4-023).
func TestAppendLogConformance(t *testing.T) {
	t.Parallel()

	for name, newStore := range appendLogImplementations() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			t.Run("missing key", func(t *testing.T) {
				t.Parallel()
				list, ok, err := newStore(t).List("absent")
				if list != nil || ok || err != nil {
					t.Fatalf("List(absent) = (%v, %v, %v), want (nil, false, nil)", list, ok, err)
				}
			})

			t.Run("append order and returned list", func(t *testing.T) {
				t.Parallel()
				store := newStore(t)
				for i := range 3 {
					got, err := store.Append("k", &i)
					if err != nil {
						t.Fatalf("Append(%d): %v", i, err)
					}
					if len(*got) != i+1 || (*got)[i] != i {
						t.Fatalf("Append(%d) returned %v", i, *got)
					}
				}
				list, ok, err := store.List("k")
				if err != nil || !ok {
					t.Fatalf("List: ok=%v err=%v", ok, err)
				}
				if len(*list) != 3 || (*list)[0] != 0 || (*list)[1] != 1 || (*list)[2] != 2 {
					t.Fatalf("List = %v, want [0 1 2]", *list)
				}
			})

			t.Run("returned lists are snapshots", func(t *testing.T) {
				t.Parallel()
				store := newStore(t)
				v := 1
				first, err := store.Append("k", &v)
				if err != nil {
					t.Fatalf("Append: %v", err)
				}
				(*first)[0] = 99
				list, _, err := store.List("k")
				if err != nil || (*list)[0] != 1 {
					t.Fatalf("mutating a returned list changed the store: %v (err %v)", *list, err)
				}
			})

			t.Run("keys are independent", func(t *testing.T) {
				t.Parallel()
				store := newStore(t)
				a, b := 1, 2
				if _, err := store.Append("a", &a); err != nil {
					t.Fatalf("Append a: %v", err)
				}
				if _, err := store.Append("b", &b); err != nil {
					t.Fatalf("Append b: %v", err)
				}
				if err := store.Delete("a"); err != nil {
					t.Fatalf("Delete a: %v", err)
				}
				if _, ok, err := store.List("a"); ok || err != nil {
					t.Fatalf("List(a) after Delete: ok=%v err=%v", ok, err)
				}
				if list, ok, err := store.List("b"); !ok || err != nil || len(*list) != 1 {
					t.Fatalf("Delete(a) affected b: ok=%v err=%v", ok, err)
				}
				if err := store.Delete("never-existed"); err != nil {
					t.Fatalf("Delete of a missing key must be a no-op, got %v", err)
				}
			})

			t.Run("clear", func(t *testing.T) {
				t.Parallel()
				store := newStore(t)
				for _, key := range []string{"a", "b"} {
					v := 1
					if _, err := store.Append(key, &v); err != nil {
						t.Fatalf("Append %s: %v", key, err)
					}
				}
				if err := store.Clear(); err != nil {
					t.Fatalf("Clear: %v", err)
				}
				for _, key := range []string{"a", "b"} {
					if _, ok, err := store.List(key); ok || err != nil {
						t.Fatalf("List(%s) after Clear: ok=%v err=%v", key, ok, err)
					}
				}
			})

			t.Run("concurrent appends land exactly once", func(t *testing.T) {
				t.Parallel()
				store := newStore(t)
				const writers = 50
				var wg sync.WaitGroup
				for i := range writers {
					wg.Go(func() {
						if _, err := store.Append("k", &i); err != nil {
							t.Errorf("Append(%d): %v", i, err)
						}
					})
				}
				wg.Wait()
				list, ok, err := store.List("k")
				if err != nil || !ok {
					t.Fatalf("List: ok=%v err=%v", ok, err)
				}
				assertEachOnce(t, *list, writers)
			})
		})
	}
}
