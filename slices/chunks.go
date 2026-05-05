package slices

// Chunk splits items into consecutive sub-slices of at most size elements.
// Returns nil if size <= 0.
func Chunk[T any](items []T, size int) [][]T {
	if size <= 0 {
		return nil
	}

	var chunks [][]T
	for i := 0; i < len(items); i += size {
		end := i + size
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[i:end])
	}
	return chunks
}
