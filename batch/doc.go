// Package batch prepares slices for batched processing with bounded chunks
// and stable string deduplication. Chunks share backing elements with the input
// but have clipped capacity; merged strings are a new slice. Functions have no
// shared state. Chunk returns nil for non-positive sizes and empty input.
package batch
