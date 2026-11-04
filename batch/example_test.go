package batch_test

import (
	"fmt"

	"github.com/trypanic/go-sdk/v2/batch"
)

func ExampleChunk() {
	for _, batch := range batch.Chunk([]int{1, 2, 3, 4, 5}, 2) {
		fmt.Println(batch)
	}
	fmt.Println(batch.Chunk([]int{1, 2, 3}, 0) == nil)
	// Output:
	// [1 2]
	// [3 4]
	// [5]
	// true
}
