# batch

Prepare slices for batched processing: split bounded chunks and merge string
inputs while preserving order and removing duplicates. Formerly `slices`; the
new import path avoids a collision with the Go standard library.

## Overview

| Symbol | Kind | Purpose |
|---|---|---|
| `MergeUniqueStrings` | Function | Merge two string slices into one, preserving insertion order and discarding duplicates |
| `Chunk[T]` | Generic function | Split a slice into consecutive sub-slices of at most `size` elements |

---

## Quick Start

```go
import "github.com/trypanic/go-sdk/v2/batch"

a := []string{"apple", "banana", "cherry"}
b := []string{"banana", "date", "apple"}

result := batch.MergeUniqueStrings(a, b)
// result == []string{"apple", "banana", "cherry", "date"}
```

---

## Configuration / Settings

This package has no configuration struct or environment variables.

---

## API Reference

### `MergeUniqueStrings`

```go
func MergeUniqueStrings(a, b []string) []string
```

Returns a new slice containing every element from `a` followed by every element from `b` that did not already appear in `a`. Uniqueness is determined by exact string equality. The relative order of elements within each input slice is preserved.

**Behaviour:**
- Iterates `a` first, adding each unseen value to the result and to an internal `seen` map.
- Iterates `b` second, adding only values not already present in `seen`.
- The returned slice is always a new allocation; neither `a` nor `b` is modified.

**Edge cases:**
- If `a` itself contains duplicates, only the first occurrence is kept in the result.
- If both slices are empty or nil, returns an empty (non-nil) slice.
- Comparison is case-sensitive: `"Apple"` and `"apple"` are treated as distinct values.

**Error conditions:** None. The function never returns an error.

---

### `Chunk`

```go
func Chunk[T any](items []T, size int) [][]T
```

Splits `items` into consecutive sub-slices of at most `size` elements.
The final chunk may be shorter than `size`.

> **Size contract: `size` must be greater than zero.** With `size <= 0`
> `Chunk` returns `nil` and **no error** (the standard library `slices.Chunk`
> panics instead), so a batch loop with a misconfigured size silently
> processes nothing. Validate `size` before calling when that matters.

`Chunk` also returns `nil` when `items` is `nil` or empty. The outer slice is
allocated once with the exact number of chunks.

Sub-slices are views into the original backing array; mutating an element
through a chunk affects `items`. Each chunk's capacity is clipped to its length
(`items[i:end:end]`, as in the standard library `slices.Chunk`), so `append` on
a chunk allocates a new array instead of overwriting the next chunk or `items`.

```go
chunks := batch.Chunk([]int{1, 2, 3, 4, 5}, 2)
// [][]int{{1,2}, {3,4}, {5}}
```

---

## Real-World Usage

No known callers at the time of writing.

<!-- not used yet -->

---

## Lifecycle / Concurrency Notes

Both functions are pure and have no shared state; they are safe to call concurrently. The slice returned by `MergeUniqueStrings` is owned by the caller; the chunks returned by `Chunk` share elements with `items`.

---

## Dependencies

This package has no external dependencies beyond the Go standard library.
