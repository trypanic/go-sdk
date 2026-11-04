package batch

// Chunk splits items into consecutive sub-slices of at most size elements;
// the last chunk may be shorter.
//
// Size contract: size must be > 0. Chunk returns nil when size <= 0 (unlike
// the standard library slices.Chunk, which panics), so a misconfigured size
// processes nothing; validate size before calling when that matters. Chunk
// also returns nil for empty or nil items.
//
// Chunks share elements with items, but each chunk's capacity is clipped to
// its length, so append on a chunk never overwrites the next chunk.
func Chunk[T any](items []T, size int) [][]T {
	if size <= 0 || len(items) == 0 {
		return nil
	}

	chunks := make([][]T, 0, (len(items)+size-1)/size)
	for i := 0; i < len(items); i += size {
		end := min(i+size, len(items))
		chunks = append(chunks, items[i:end:end])
	}
	return chunks
}
