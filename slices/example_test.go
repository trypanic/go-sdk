package slices_test

import (
	"fmt"

	"github.com/trypanic/go-sdk/slices"
)

func ExampleChunk() {
	for _, batch := range slices.Chunk([]int{1, 2, 3, 4, 5}, 2) {
		fmt.Println(batch)
	}
	fmt.Println(slices.Chunk([]int{1, 2, 3}, 0) == nil)
	// Output:
	// [1 2]
	// [3 4]
	// [5]
	// true
}
