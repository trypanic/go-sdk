package slices

import (
	"reflect"
	"testing"
)

func TestChunkSplitsEvenly(t *testing.T) {
	t.Parallel()

	got := Chunk([]int{1, 2, 3, 4}, 2)
	want := [][]int{{1, 2}, {3, 4}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestChunkRemainderInLastChunk(t *testing.T) {
	t.Parallel()

	got := Chunk([]int{1, 2, 3, 4, 5}, 2)
	want := [][]int{{1, 2}, {3, 4}, {5}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestChunkSizeLargerThanSlice(t *testing.T) {
	t.Parallel()

	got := Chunk([]int{1, 2}, 10)
	want := [][]int{{1, 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestChunkNilInput(t *testing.T) {
	t.Parallel()

	var nothing []int
	if got := Chunk(nothing, 3); got != nil {
		t.Fatalf("Chunk(nil, 3) = %v, want nil", got)
	}
}

func TestChunkInvalidSizeReturnsNil(t *testing.T) {
	t.Parallel()

	if got := Chunk([]int{1, 2, 3}, 0); got != nil {
		t.Fatalf("Chunk(_, 0) = %v, want nil", got)
	}
	if got := Chunk([]int{1, 2, 3}, -1); got != nil {
		t.Fatalf("Chunk(_, -1) = %v, want nil", got)
	}
}

func TestMergeUniqueStringsPreservesOrderAndDedupes(t *testing.T) {
	t.Parallel()

	got := MergeUniqueStrings(
		[]string{"apple", "banana", "cherry"},
		[]string{"banana", "date", "apple"},
	)
	want := []string{"apple", "banana", "cherry", "date"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Each chunk is clipped to its length, so appending to one chunk
// reallocates instead of overwriting the next chunk and the caller's
// backing array (stdlib slices.Chunk does the same).
func TestChunkAppendDoesNotOverwriteNextChunk(t *testing.T) {
	t.Parallel()

	items := []int{1, 2, 3, 4}
	c := Chunk(items, 2)
	if cap(c[0]) != len(c[0]) {
		t.Fatalf("cap(c[0]) = %d, want %d", cap(c[0]), len(c[0]))
	}
	c[0] = append(c[0], 99)

	if want := []int{1, 2, 3, 4}; !reflect.DeepEqual(items, want) {
		t.Fatalf("items after append = %v, want %v", items, want)
	}
	if want := []int{3, 4}; !reflect.DeepEqual(c[1], want) {
		t.Fatalf("c[1] after append = %v, want %v", c[1], want)
	}
}
