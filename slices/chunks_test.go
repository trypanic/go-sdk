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
