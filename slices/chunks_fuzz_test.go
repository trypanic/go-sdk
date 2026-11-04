package slices

import (
	"bytes"
	"testing"
)

// Invariants: for size > 0 no item is lost or reordered and every chunk has
// 1..size items (only the last may be shorter); for size <= 0 the result is
// nil.
func FuzzChunk(f *testing.F) {
	f.Add([]byte("abcdef"), 2)
	f.Add([]byte("abcde"), 3)
	f.Add([]byte{}, 1)
	f.Add([]byte("abc"), 0)
	f.Add([]byte("abc"), -4)
	f.Fuzz(func(t *testing.T, items []byte, size int) {
		chunks := Chunk(items, size)
		if size <= 0 || len(items) == 0 {
			if chunks != nil {
				t.Fatalf("Chunk(%q, %d) = %v, want nil", items, size, chunks)
			}
			return
		}
		var joined []byte
		for i, c := range chunks {
			if len(c) < 1 || len(c) > size {
				t.Fatalf("chunk %d has %d items, want 1..%d", i, len(c), size)
			}
			if i < len(chunks)-1 && len(c) != size {
				t.Fatalf("non-last chunk %d has %d items, want %d", i, len(c), size)
			}
			joined = append(joined, c...)
		}
		if !bytes.Equal(joined, items) {
			t.Fatalf("Chunk lost or reordered items: joined %q, want %q", joined, items)
		}
	})
}
