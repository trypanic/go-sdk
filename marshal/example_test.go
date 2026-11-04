package marshal_test

import (
	"fmt"

	"github.com/trypanic/go-sdk/v2/marshal"
)

func ExampleNoEscapeNoNewline() {
	encoded, err := marshal.NoEscapeNoNewline(map[string]string{"text": "<hello>"})
	fmt.Println(encoded, err)
	// Output: {"text":"<hello>"} <nil>
}
