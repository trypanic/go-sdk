package storage

import (
	"errors"
	"testing"

	"github.com/trypanic/go-sdk/errorkit"
)

// appendLogFactory builds an empty AppendLogStorager for one test.
type appendLogFactory func(t *testing.T) AppendLogStorager[int]

// appendLogImplementations lists every AppendLogStorager in the package so
// the conformance tests run against all of them.
func appendLogImplementations() map[string]appendLogFactory {
	return map[string]appendLogFactory{
		"MemoryStorage": func(*testing.T) AppendLogStorager[int] {
			return NewMemory[int]()
		},
		"FileStorage": func(t *testing.T) AppendLogStorager[int] {
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
			var appErr *errorkit.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("expected *errorkit.AppError, got %T", err)
			}
			if _, ok, err := store.List("k"); ok || err != nil {
				t.Fatalf("Append(nil) must not create the key: ok=%v err=%v", ok, err)
			}
		})
	}
}
